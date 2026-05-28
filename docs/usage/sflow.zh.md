# 应用间流量观测指南

本文说明 Unifabric 如何提供应用间流量观测能力。启用后，Unifabric 可以接收多台交换机上报的采样流量，把采样报文解析成可查询的流记录，并用 Kubernetes Pod、Node 和 workload owner 信息补充源端与目的端。运维人员可以从交换机或 workload 视角查看哪些应用在通信、流量经过哪些交换机、发送和接收流量主要来自哪些 Pod。

sFlow 在这条链路里是交换机侧的采样输入方式。使用者需要关注的是写入 `flows_raw` 后形成的应用间流记录，以及基于这些记录的 switch / workload dashboard。sFlow 解码、字段补充和过载处理的实现细节见 [实现设计](../design/sflow.zh.md)。

## 观测模型

这项能力的输入来自交换机推送的 sFlow v5 datagram。collector 会从 datagram 中提取源地址、目的地址、协议、端口、采样字节数、采样包数、采样率和 sampler address，并把每条有效 sample 转换为一条规范化流记录。

写入 ClickHouse 前，collector 会用源端和目的端 IP 匹配当前 Kubernetes Pod cache。命中的一侧会补充 Pod namespace、Pod name、Node name，以及顶层 workload owner。这里的「应用」按 Kubernetes 顶层 workload owner 表达，例如 Deployment、StatefulSet、DaemonSet、Job，以及当前 RBAC 允许读取的训练或计算任务对象。

没有命中 Pod IP 的端点仍会写入基础流字段，Kubernetes 字段保持为空。这类记录可以继续用于按交换机、IP、协议和时间范围排查；涉及集群内 Pod 的一侧仍然会保留应用归因信息。

## 前置条件

- 一个可安装 Unifabric chart 的 Kubernetes 集群。
- 如果使用 chart 内置 ClickHouse 并采用 PVC 存储，集群需要有可用的默认 StorageClass，或在 values 中指定 `sflow.clickhouse.managed.persistence.storageClassName`。如果采用宿主机目录存储，需要为 ClickHouse Pod 选择固定节点，并确保该节点目录有足够空间和写入权限。
- 交换机能够把 sFlow v5 datagram 发送到 Kubernetes Service 地址。
- 如果 `grafanaDashboard.enabled=true`，集群需要具备 Grafana dashboard 导入能力。

## 安装路径

根据 ClickHouse 来源和数据保存方式，可以选择以下三种安装路径。三种路径都会让 collector 在启动时检查并准备所需的 `flows_raw` 表，不需要手工执行 `clickhouse-client` 初始化 SQL。

### 1. 使用内置 ClickHouse 安装（PVC）

适用于集群已有默认 StorageClass，或可以为内置 ClickHouse 指定 StorageClass 的环境。ClickHouse 数据保存在 PVC 中，适合作为默认安装方式。

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    # 可选。不设置或设置为 0 时，由 Kubernetes 自动分配 nodePort。
    nodePort: 0
  clickhouse:
    database: default
    username: default
    table: flows_raw
    managed:
      enabled: true
      persistence:
        enabled: true
        type: pvc
        size: 20Gi
        # 可选。为空时使用集群默认 StorageClass。
        storageClassName: ""
    schema:
      retentionDays: 3
```

这份配置会让 chart 渲染 sFlow collector、UDP Service、ClickHouse `StatefulSet` 和 ClickHouse Service。collector 的 ClickHouse 地址会自动指向 chart 内置的 ClickHouse Service，并在启动时准备写入所需的 `flows_raw` 表。`retentionDays` 用于控制流记录保留时间，最小值为 1；未配置时默认保留 3 天。

如果 `sflow.replicaCount` 大于 1，不需要为表初始化增加额外步骤；chart 会给 collector 配好所需权限。

将上面的内容保存为 `sflow-pvc-values.yaml`，执行下面的命令。命令可同时用于初次安装和已有 release 更新；已有 release 时，`--reuse-values` 会保留既有 Helm values，并叠加当前文件中的 sFlow 配置。

```bash
helm upgrade --install unifabric ./chart \
  --namespace unifabric-system \
  --create-namespace \
  --reuse-values \
  --values sflow-pvc-values.yaml \
  --wait
```

### 2. 使用内置 ClickHouse 安装（hostPath 测试）

适用于测试集群、单节点环境，或暂时没有可用 StorageClass 的环境。ClickHouse 数据会写入宿主机目录；Pod 调度到其它节点后，不会自动带有原节点上的数据。生产环境需要跨节点迁移或更高可用时，仍建议使用 PVC、local PV，或外部 ClickHouse。

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    nodePort: 0
  clickhouse:
    database: default
    username: default
    table: flows_raw
    managed:
      enabled: true
      # 把 ClickHouse Pod 固定到保存 hostPath 数据的节点。
      # 先通过 `kubectl get nodes` 确认目标节点名，再替换下面的值。
      nodeSelector:
        kubernetes.io/hostname: worker-1
      persistence:
        enabled: true
        type: hostPath
        hostPath:
          path: /var/lib/unifabric/clickhouse
          type: DirectoryOrCreate
    schema:
      retentionDays: 3
```

`worker-1` 只是示例，需要替换成实际承载 `/var/lib/unifabric/clickhouse` 的节点名。后续如果要迁移到其他节点，需要先迁移或重新准备该目录下的数据。

将上面的内容保存为 `sflow-hostpath-values.yaml`，执行下面的命令。命令可同时用于初次安装和已有 release 更新；已有 release 时，`--reuse-values` 会保留既有 Helm values，并叠加当前文件中的 sFlow 配置。

```bash
helm upgrade --install unifabric ./chart \
  --namespace unifabric-system \
  --create-namespace \
  --reuse-values \
  --values sflow-hostpath-values.yaml \
  --wait
```

### 3. 使用外部 ClickHouse

适用于生产环境已经有 ClickHouse，或希望 ClickHouse 的容量、备份和高可用由外部系统管理的场景。此路径不会安装内置 ClickHouse，只把连接地址和认证信息交给 collector。

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    nodePort: 0
  clickhouse:
    address: clickhouse-clickhouse.clickhouse.svc.cluster.local:9000
    database: default
    username: default
    passwordSecret:
      name: clickhouse-auth
      key: password
    table: flows_raw
    managed:
      enabled: false
    schema:
      retentionDays: 3
```

目标 ClickHouse 用户需要具备 `CREATE DATABASE`、`CREATE TABLE`、`ALTER TABLE` 和写入权限。已有表如果结构不一致，需要先由管理员确认并处理，collector 不会自动改写已有表结构。

将上面的内容保存为 `sflow-external-clickhouse-values.yaml`，执行下面的命令。命令可同时用于初次安装和已有 release 更新；已有 release 时，`--reuse-values` 会保留既有 Helm values，并叠加当前文件中的 sFlow 配置。

```bash
helm upgrade --install unifabric ./chart \
  --namespace unifabric-system \
  --create-namespace \
  --reuse-values \
  --values sflow-external-clickhouse-values.yaml \
  --wait
```

默认使用 `NodePort`，因为交换机 sFlow 流量通常来自集群外部。交换机需要固定 UDP node port 时，可以设置 `sflow.service.nodePort`；保持为 `0` 时，由 Kubernetes 自动分配。环境支持 UDP 外部负载均衡时，也可以把 `sflow.service.type` 改成 `LoadBalancer`。

## 接入交换机采样数据

在交换机上配置 sFlow collector 的地址和端口。下面示例在 SONiC 上对所有端口启用 sFlow，并把采样数据发送到 Unifabric sFlow Service：

```bash
sudo config sflow collector add collector1 192.168.122.172 --port 6343
sudo config sflow interface enable all
sudo config sflow enable
sudo show sflow
```

示例中的 `192.168.122.172` 和 `6343` 需要替换成 sFlow Service 的实际地址和端口。可以先查看 Service：

```bash
kubectl get svc -n unifabric-system unifabric-sflow
```

如果 Service 是 `NodePort`，地址使用交换机可访问的 Kubernetes Node IP，端口使用输出中的 nodePort。如果 Service 是 `LoadBalancer`，地址使用输出中的 LoadBalancer IP，端口使用输出中的 Service port。

## 验证采集与写入链路

检查渲染出的资源：

```bash
kubectl -n unifabric-system get deploy,statefulset,svc,cm \
  -l 'app.kubernetes.io/component in (unifabric-sflow,unifabric-sflow-clickhouse)'
kubectl -n unifabric-system get clusterrole,clusterrolebinding -l app.kubernetes.io/component=unifabric-sflow
```

内置 ClickHouse 启用时，`unifabric-sflow-clickhouse` StatefulSet 应处于 Ready 状态。collector 会在启动阶段准备 ClickHouse 写入环境；如果 ClickHouse 权限不足或连接失败，collector 无法进入 Ready，并会在日志中输出相关错误。

检查 Pod 状态和日志：

```bash
kubectl -n unifabric-system get pods \
  -l 'app.kubernetes.io/component in (unifabric-sflow,unifabric-sflow-clickhouse)'
kubectl -n unifabric-system logs deploy/unifabric-sflow -c sflow
```

从集群内检查 metrics endpoint：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-sflow -o jsonpath='{.items[0].status.podIP}')
kubectl -n unifabric-system run sflow-metrics-check --rm -i --restart=Never --image=curlimages/curl -- \
  "http://${POD_IP}:8084/metrics"
```

常用指标：

- `unifabric_sflow_datagrams_accepted_total`：collector 接收到的 datagram 数量。
- `unifabric_sflow_decode_errors_total`：格式错误或不支持的 datagram 数量。
- `unifabric_sflow_records_decoded_total`：生成的规范化记录数量。
- `unifabric_sflow_records_written_total`：写入 ClickHouse 的记录数量。
- `unifabric_sflow_records_dropped_total`：writer queue 满后丢弃的记录数量。
- `unifabric_sflow_write_errors_total`：ClickHouse 写入失败次数。
- `unifabric_sflow_queue_depth`：当前排队等待写入的记录数量。

## 验证应用间流记录

按源 workload、目的 workload 和交换机查询最近流量：

```sql
SELECT
  src_k8s_top_owner_namespace AS src_namespace,
  src_k8s_top_owner_kind AS src_kind,
  src_k8s_top_owner_name AS src_workload,
  dst_k8s_top_owner_namespace AS dst_namespace,
  dst_k8s_top_owner_kind AS dst_kind,
  dst_k8s_top_owner_name AS dst_workload,
  IPv4NumToString(reinterpretAsUInt32(reverse(substring(sampler_address, 13, 4)))) AS switch,
  count() AS samples,
  sum(bytes * if(sampling_rate = 0, toUInt64(1), sampling_rate)) AS observed_bytes,
  sum(packets * if(sampling_rate = 0, toUInt64(1), sampling_rate)) AS observed_packets
FROM flows_raw
WHERE time_flow_start > now() - INTERVAL 10 MINUTE
  AND (src_k8s_top_owner_name != '' OR dst_k8s_top_owner_name != '')
GROUP BY
  src_namespace,
  src_kind,
  src_workload,
  dst_namespace,
  dst_kind,
  dst_workload,
  switch
ORDER BY observed_bytes DESC
LIMIT 20;
```

预期结果：涉及当前集群 Pod 的记录，会在命中的源端或目的端包含 workload owner 字段。两个端点都命中 Pod IP 时，可以看到 workload 到 workload 的流量；只有一侧命中时，可以看到集群内应用与外部端点之间的流量。

需要检查具体 Pod 时，可以查询最近原始记录：

```sql
SELECT
  time_flow_start,
  IPv4NumToString(reinterpretAsUInt32(reverse(substring(sampler_address, 13, 4)))) AS switch,
  src_k8s_pod_namespace,
  src_k8s_pod_name,
  src_k8s_node_name,
  dst_k8s_pod_namespace,
  dst_k8s_pod_name,
  dst_k8s_node_name,
  bytes,
  packets,
  sampling_rate
FROM flows_raw
WHERE time_flow_start > now() - INTERVAL 10 MINUTE
ORDER BY time_flow_start DESC
LIMIT 20;
```

## 验证 Dashboard

当 `grafanaDashboard.enabled=true` 且 `sflow.enabled=true` 时，chart 会渲染：

- `switch-sflow.json`
- `workload-sflow.json`

Switch dashboard 从交换机 sampler address 进入，展示该交换机观测到的 workload、Pod、sample、protocol 和 top talker。

![Unifabric switch sFlow 看板效果，展示选中交换机的流量面板](../images/switch-sflow.png)

Workload dashboard 从 workload owner 进入，展示选中应用的发送与接收流量、经过的交换机、对应的 Pod 分布，以及选中应用访问其他 workload / 其他 workload 访问选中应用的流量。

![Unifabric workload sFlow 看板效果，展示选中 workload 的流量面板](../images/workload-sflow.png)

如果 Dashboard 没有数据，先确认所选时间范围内 ClickHouse 有行，再检查 dashboard data source 和变量。

## 排障

如果安装后 collector 没有进入 Ready：

- 查看 `unifabric-sflow` 日志，确认能连接 ClickHouse，并且配置的用户有 `CREATE DATABASE`、`CREATE TABLE`、`ALTER TABLE` 和 `INSERT` 权限。
- 使用内置 ClickHouse 时，确认 PVC 已绑定，`unifabric-sflow-clickhouse` Pod 处于 Ready 状态。
- 使用外部 ClickHouse 时，确认 `sflow.clickhouse.address` 是 collector Pod 可以访问的 `host:port`，并且 Secret 中的密码 key 与 `sflow.clickhouse.passwordSecret.key` 一致。

如果 collector 没有收到 datagram：

- 确认 Service type 和地址能被交换机访问。
- 确认交换机 exporter 指向 UDP `6343`，或配置的 `sflow.service.port`。
- 确认 Kubernetes NetworkPolicy 或外部防火墙允许 UDP 流量。

如果行已写入，但没有 Pod 字段：

- 确认 sampled record 中的端点 IP 是 Pod IP，而不只是 Node 地址或 NAT 后地址。
- 确认目标 Pod 处于 Running 状态，并已有 Pod IP。
- 确认 collector RBAC 可以 list Pods，并读取 owner 对象。
- Pod 创建后，等待一个 Pod cache refresh interval 再检查。

如果 ClickHouse 写入失败：

- 确认 `sflow.clickhouse.address` 包含 host 和 port。
- 确认 collector 已完成 ClickHouse 初始化，且表结构与项目内置的 `flows_raw` 表结构匹配。
- 确认配置的 username 和 Secret password 有效。
- 检查 `unifabric_sflow_write_errors_total` 和 collector 日志。

如果记录被丢弃：

- 检查 `unifabric_sflow_queue_depth` 和 `unifabric_sflow_records_dropped_total`。
- 提高交换机 sampling rate，降低输入量。
- 增大 `sflow.writer.queueSize` 或 `sflow.writer.batchSize`。
- 检查 ClickHouse insert latency 和资源使用情况。
