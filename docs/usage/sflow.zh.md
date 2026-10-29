# 应用流量观测指南

本指南介绍 Unifabric 如何提供应用流量观测能力。启用后，Unifabric 接收多台交换机上报的采样流量，把样本解码为可查询的流记录，并为源端和目的端补充 Kubernetes Pod、Node 和工作负载属主信息。运维人员可以从交换机视角或工作负载视角查看哪些应用在通信、哪些交换机观测到了流量，以及哪些 Pod 贡献了主要的发送和接收流量。

sFlow 是这条链路的交换机侧采样输入。采集器把补全后的流记录以 OTLP 日志形式导出到随 Chart 部署的 OpenTelemetry Collector，由 Collector 写入 Elasticsearch 数据流 `unifabric-switch-sflow`。使用者主要面向这个数据流以及基于它构建的交换机和工作负载仪表盘。sFlow 解码、补全和过载处理的细节见 [sFlow 处理设计](../design/sflow.zh.md)。

## 观测模型

该能力的输入是交换机推送的 sFlow v5 数据报。采集器从数据报中提取源地址、目的地址、协议、端口、采样字节数、采样包数、采样率和采样器地址，并把每个有效样本转换为一条规范化流记录。

导出前，采集器把源 IP 和目的 IP 与当前 Kubernetes Pod 缓存进行匹配。匹配成功的端点会补充 Pod 命名空间、Pod 名称、Node 名称和顶层工作负载属主。本指南中的「应用」以 Kubernetes 顶层工作负载属主表示，例如 Deployment、StatefulSet、DaemonSet、Job，以及当前 RBAC 权限允许采集器读取的训练或计算作业对象。

未匹配到 Pod IP 的端点仍会导出基础流字段，Kubernetes 字段保持为空。这些记录仍可用于按交换机、IP、协议和时间范围排查问题。如果流的一侧涉及集群内 Pod，该侧保留应用归属字段。

以下派生字段在导出时计算，仪表盘和查询可以直接使用：

- `scaled_bytes` 和 `scaled_packets`：采样值乘以采样率。
- `sampler_ip`、`src_ip`、`dst_ip`：字符串形式的 IP 地址。
- `protocol`：IP 协议名称，例如 `TCP` 或 `UDP`。
- `src_workload`、`dst_workload`：顶层属主，格式为 `namespace/kind/name`。

## 前置条件

- 一个可以安装 Unifabric Chart 的 Kubernetes 集群。
- 一个 Elasticsearch 目标。Chart 可以部署一个随 Chart 管理的单节点测试实例，也可以通过 `ebpfFlowEventCollector.elasticsearch.endpoints` 指向外部集群。
- 交换机能够向 Kubernetes Service 地址发送 sFlow v5 数据报。
- 如果设置 `grafanaDashboard.enabled=true`，需要支持 Grafana 仪表盘导入。

## 安装路径

根据数据存储位置选择以下安装路径之一。所有路径中，Chart 会在写入前自动应用所需的 `unifabric-switch-sflow` 索引模板和 ILM 策略，不需要手动创建数据流。

### 1. 使用随 Chart 部署的测试 Elasticsearch

该路径适用于评估和测试集群。当 `ebpfFlowEventCollector.elasticsearch.endpoints` 为空时，Chart 会部署一个使用固定凭据、未启用 TLS 的单节点 Elasticsearch。默认数据存放在 emptyDir 中，Pod 重建后数据丢失；设置 `ebpfFlowEventCollector.elasticsearch.bundled.persistence.enabled=true` 可以改为 PVC 存储。

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    # 可选。省略或设置为 0 时由 Kubernetes 自动分配 nodePort。
    nodePort: 0

ebpfFlowEventCollector:
  elasticsearch:
    enabled: true
```

该配置会渲染 sFlow 采集器、UDP Service、带独立 sFlow OTLP 端口的共享 OpenTelemetry Collector、随 Chart 部署的 Elasticsearch，以及 sFlow 的 Grafana 数据源。采集器的 OTLP 端点自动指向该 Collector。

把以上内容保存为 `sflow-values.yaml`，然后执行以下命令。首次安装和更新已有 Release 使用同一条命令。对已有 Release，`--reuse-values` 会保留现有 Helm values，并叠加本文件中的 sFlow 配置。

```bash
helm upgrade --install unifabric ./chart \
  --namespace unifabric-system \
  --create-namespace \
  --reuse-values \
  --values sflow-values.yaml \
  --wait
```

### 2. 使用外部 Elasticsearch

该路径适用于生产环境已有 Elasticsearch，或 Elasticsearch 的容量、备份和高可用由外部系统管理的场景。Collector 会把 sFlow 文档写入外部集群的 `unifabric-switch-sflow` 数据流。

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    nodePort: 0

ebpfFlowEventCollector:
  elasticsearch:
    enabled: true
    endpoints:
      - http://elasticsearch.data.svc:9200
    user: elastic
    # 已存在的 Secret，包含 password 键。也可以直接设置 password。
    existingSecret: elasticsearch-auth
```

配置的 Elasticsearch 用户需要具备创建 ILM 策略、索引模板和数据流的权限，以及写入文档的权限。Elasticsearch 连接与 RDMA 流事件链路共用；sFlow 数据写入独立的数据流，不会与 RDMA 文档混合。

把以上内容保存为 `sflow-external-es-values.yaml`，然后使用与路径 1 相同的 `helm upgrade --install` 命令并指定该文件。

### 3. 使用外部 OpenTelemetry Collector

该路径适用于已经运营 OpenTelemetry Collector，并希望自行管理 Elasticsearch exporter 和 Schema 的场景。把 `sflow.otel.endpoint` 设置为 OTLP gRPC 地址后，Chart 不再要求启用随 Chart 部署的 Collector，索引模板、ILM 策略和 Grafana 数据源由使用者自行负责。

```yaml
sflow:
  enabled: true
  otel:
    endpoint: otel-collector.observability.svc:4317
    insecure: true
    timeout: 10s
```

由于交换机的 sFlow 流量通常来自集群外部，Service 默认使用 `NodePort` 类型。当交换机需要固定 UDP 节点端口时设置 `sflow.service.nodePort`；保持为 `0` 则由 Kubernetes 自动分配。如果环境支持外部 UDP 负载均衡，可以把 `sflow.service.type` 设置为 `LoadBalancer`。

## 配置交换机采样数据

在交换机上配置 sFlow 采集器地址和端口。以下示例在 SONiC 上对所有端口启用 sFlow，并把采样数据发送到 Unifabric 的 sFlow Service：

```bash
sudo config sflow collector add collector1 192.168.122.172 --port 6343
sudo config sflow interface enable all
sudo config sflow enable
sudo show sflow
```

把 `192.168.122.172` 和 `6343` 替换为实际的 sFlow Service 地址和端口。先检查 Service：

```bash
kubectl get svc -n unifabric-system unifabric-sflow
```

如果 Service 是 `NodePort` 类型，使用交换机可达的 Kubernetes Node IP 和输出中显示的 nodePort。如果 Service 是 `LoadBalancer` 类型，使用输出中显示的 LoadBalancer IP 和 Service 端口。

## 验证接收与导出链路

检查渲染出的资源：

```bash
kubectl -n unifabric-system get deploy,svc,cm \
  -l 'app.kubernetes.io/component in (unifabric-sflow,unifabric-ebpf-flow-event-collector)'
kubectl -n unifabric-system get clusterrole,clusterrolebinding -l app.kubernetes.io/component=unifabric-sflow
```

`ebpf-flow-event-collector` Deployment 应处于就绪状态，并暴露 sFlow 的 OTLP 端口 4319。使用随 Chart 部署的 Elasticsearch 时，`elasticsearch` Deployment 也应处于就绪状态；其 Schema sidecar 会在每次启动后应用索引模板和 ILM 策略。

检查 Pod 状态和日志：

```bash
kubectl -n unifabric-system get pods \
  -l 'app.kubernetes.io/component in (unifabric-sflow,unifabric-ebpf-flow-event-collector,unifabric-elasticsearch)'
kubectl -n unifabric-system logs deploy/unifabric-sflow -c sflow
```

在集群内检查指标端点：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-sflow -o jsonpath='{.items[0].status.podIP}')
kubectl -n unifabric-system run sflow-metrics-check --rm -i --restart=Never --image=curlimages/curl -- \
  "http://${POD_IP}:8084/metrics"
```

常用指标：

- `unifabric_sflow_datagrams_accepted_total`：采集器接收的数据报数量。
- `unifabric_sflow_decode_errors_total`：格式错误或不支持的数据报数量。
- `unifabric_sflow_records_decoded_total`：生成的规范化记录数量。
- `unifabric_sflow_records_written_total`：交给 OTLP 导出器的记录数量。
- `unifabric_sflow_records_dropped_total`：因写入队列已满而丢弃的记录数量。
- `unifabric_sflow_write_errors_total`：OTLP 导出失败次数。
- `unifabric_sflow_queue_depth`：当前等待导出的记录数量。

## 验证应用流量记录

按源工作负载、目的工作负载和交换机查询近期流量。示例使用随 Chart 部署的 Elasticsearch 凭据（`elastic` / `rdma-flow`），外部集群请替换为实际凭据。

```bash
ES_POD=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-elasticsearch -o jsonpath='{.items[0].metadata.name}')
kubectl -n unifabric-system exec "$ES_POD" -c elasticsearch -- \
  curl -fsS -u elastic:rdma-flow -H 'Content-Type: application/json' \
  'http://localhost:9200/unifabric-switch-sflow/_search' -d '{
    "size": 0,
    "query": {
      "bool": {
        "filter": [{"range": {"@timestamp": {"gte": "now-10m"}}}],
        "should": [
          {"exists": {"field": "src_workload"}},
          {"exists": {"field": "dst_workload"}}
        ],
        "minimum_should_match": 1
      }
    },
    "aggs": {
      "by_pair": {
        "multi_terms": {
          "terms": [
            {"field": "src_workload", "missing": "-"},
            {"field": "dst_workload", "missing": "-"},
            {"field": "sampler_ip"}
          ],
          "size": 20,
          "order": {"observed_bytes": "desc"}
        },
        "aggs": {
          "observed_bytes": {"sum": {"field": "scaled_bytes"}},
          "observed_packets": {"sum": {"field": "scaled_packets"}}
        }
      }
    }
  }'
```

预期结果：涉及当前集群 Pod 的记录在匹配成功的源端或目的端带有工作负载属主字段。当两端都匹配到 Pod IP 时，可以看到工作负载到工作负载的流量。当只有一端匹配时，可以看到集群内应用与外部端点之间的流量。

需要检查特定 Pod 时，查询近期原始文档：

```bash
kubectl -n unifabric-system exec "$ES_POD" -c elasticsearch -- \
  curl -fsS -u elastic:rdma-flow -H 'Content-Type: application/json' \
  'http://localhost:9200/unifabric-switch-sflow/_search' -d '{
    "size": 20,
    "sort": [{"@timestamp": "desc"}],
    "query": {"range": {"@timestamp": {"gte": "now-10m"}}},
    "_source": ["@timestamp", "sampler_ip", "src_pod_namespace", "src_pod_name",
                "src_node_name", "dst_pod_namespace", "dst_pod_name", "dst_node_name",
                "bytes", "packets", "sampling_rate"]
  }'
```

## 验证仪表盘

当 `grafanaDashboard.enabled=true` 且 `sflow.enabled=true` 时，Chart 会渲染：

- `switch-sflow.json`
- `workload-sflow.json`
- `workload-pair-sflow.json`

三个仪表盘都通过 `unifabric-sflow-elasticsearch` 数据源查询 Elasticsearch。当 `sflow.elasticsearch.grafanaDatasource.enabled=true` 时，该数据源由 Chart 自动创建。

交换机仪表盘以交换机采样器地址为入口，展示该交换机观测到的工作负载、Pod、样本、协议和流量排名。

![Unifabric 交换机 sFlow 仪表盘，展示所选交换机的流量面板](../images/switch-sflow.png)

工作负载仪表盘以工作负载属主为入口，展示所选应用的发送和接收流量、观测到流量的交换机、相关 Pod 分布，以及所选应用与其他工作负载之间的双向流量。

![Unifabric 工作负载 sFlow 仪表盘，展示所选工作负载的流量面板](../images/workload-sflow.png)

工作负载对仪表盘以源工作负载和目的工作负载为入口，按观测到各方向流量的交换机分组，展示源到目的和目的到源的流量。

Pod 数量、工作负载数量等去重统计使用 Elasticsearch cardinality 聚合，结果为近似值。源端和目的端的数量分开展示。

如果仪表盘没有数据，先确认所选时间范围内 `unifabric-switch-sflow` 数据流有文档，再检查仪表盘的数据源和变量。

## 故障排查

安装后采集器未就绪：

- 检查 `unifabric-sflow` 日志，确认它可以访问配置的 OTLP 端点。
- 确认 `ebpf-flow-event-collector` Deployment 就绪并监听端口 4319。
- 使用随 Chart 部署的 Elasticsearch 时，确认 `elasticsearch` Pod 就绪，且 Schema sidecar 应用索引模板时没有报错。

采集器未收到数据报：

- 确认 Service 类型和地址对交换机可达。
- 确认交换机导出目标是 UDP `6343` 或配置的 `sflow.service.port`。
- 确认 Kubernetes NetworkPolicy 或外部防火墙允许 UDP 流量。

写入的文档缺少 Pod 字段：

- 确认采样记录中的端点 IP 是 Pod IP，而不只是 Node 地址或 NAT 后的地址。
- 确认目标 Pod 正在运行且已分配 Pod IP。
- 确认采集器的 RBAC 可以列出 Pod 并读取属主对象。
- Pod 创建后等待一个 Pod 缓存刷新周期再检查。

OTLP 导出失败：

- 如果设置了 `sflow.otel.endpoint`，确认它是采集器可达的 `host:port`。
- 检查 `unifabric_sflow_write_errors_total` 和采集器日志。
- 检查 OpenTelemetry Collector 日志中的 Elasticsearch 导出错误，并确认 Elasticsearch 凭据有效。

文档缺少预期字段：

- `unifabric-switch-sflow` 索引模板禁用了动态映射。新增属性需要先在模板中增加字段，再对数据流执行一次 rollover。

出现记录丢弃：

- 检查 `unifabric_sflow_queue_depth` 和 `unifabric_sflow_records_dropped_total`。
- 调大交换机采样率以降低输入量。
- 调大 `sflow.writer.queueSize` 或 `sflow.writer.batchSize`。
- 检查 OpenTelemetry Collector 和 Elasticsearch 的延迟与资源使用情况。
