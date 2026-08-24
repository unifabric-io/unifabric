# 拓扑可视化使用指南

English version: [topology-visualization.md](./topology-visualization.md)

本文介绍如何启用和使用 Unifabric 拓扑可视化。该功能通过 Grafana datasource 和 panel
插件，在同一张图中展示集群的 Scale-out、Scale-up 和 Storage 网络拓扑。

## 使用前提

1. 集群已经安装 Unifabric。
2. 集群已经安装 Grafana Operator 及其 CRD。
3. 执行以下命令可以获取至少一个 Topology CR：

```bash
kubectl get topologies.unifabric.io
NAME       AGE
scaleout   4h27m
scaleup    4h27m
storage    4h27m
```

## 开启拓扑可视化

### 使用内置 Grafana 实例

执行以下 Helm upgrade，在保留现有 values 的同时启用 Topology API 和附带的 Grafana 实例：

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topologyAPI.enabled=true \
  --set grafanaInstance.enabled=true \
  --wait
```

Chart 会创建 `Grafana`、`GrafanaDatasource` 和 `GrafanaDashboard`。Topology datasource 会自动
连接当前 release 的 Controller Topology API。设置生效后访问 Grafana，搜索并打开
**Unifabric Topology** dashboard。

内置 Grafana 使用 `ghcr.io/unifabric-io/unifabric-grafana:<version>` 镜像。镜像已包含拓扑
datasource 和 panel 插件；默认 tag 跟随 Chart 的 `appVersion`。

### 使用外部 Grafana 实例

使用由其他 Chart 或组件管理的 Grafana 实例时，只需开启 Topology API：

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topologyAPI.enabled=true \
  --wait
```

`grafanaInstance.enabled` 默认为 `false`。使用默认的 Grafana dashboard 配置时，Chart 会创建
`GrafanaDatasource` 和 `GrafanaDashboard`，并通过 `grafanaDashboard.instanceSelector` 选择外部
Grafana 实例。默认 selector 为 `{}`；集群中存在多个 Grafana 实例时，请配置 selector 以匹配
目标实例。

例如，外部 `Grafana` CR 包含 `dashboards: grafana` label 时，可以设置：

```yaml
grafanaDashboard:
  instanceSelector:
    matchLabels:
      dashboards: grafana
```

如果外部 Grafana 可以使用 Unifabric Grafana 镜像，请在对应的 `Grafana` CR 中修改
`spec.version`：

```yaml
spec:
  version: ghcr.io/unifabric-io/unifabric-grafana:<version>
```

如果外部 Grafana 必须保留当前镜像，可以通过 init container 安装插件。下面的配置假设
`grafana-plugins` 共享卷已经挂载到 Grafana container 的 `/var/lib/grafana/plugins`。同时需要
允许 Grafana 加载这两个未签名插件：

```yaml
spec:
  config:
    plugins:
      allow_loading_unsigned_plugins: unifabric-unifabrictopology-datasource,unifabric-unifabrictopology-panel
  deployment:
    spec:
      template:
        spec:
          initContainers:
            - name: unifabric-plugins-init
              image: ghcr.io/unifabric-io/unifabric-grafana:<version>
              command:
                - /bin/sh
                - -c
              args:
                - |
                  set -eu
                  for plugin in \
                    unifabric-unifabrictopology-datasource \
                    unifabric-unifabrictopology-panel
                  do
                    test -f "/var/lib/grafana/plugins/${plugin}/plugin.json"
                    rm -rf "/opt/plugins/${plugin}"
                    cp -a "/var/lib/grafana/plugins/${plugin}" /opt/plugins/
                  done
              volumeMounts:
                - name: grafana-plugins
                  mountPath: /opt/plugins
```

如果共享卷名称不是 `grafana-plugins`，请同步修改 init container 中的 `volumeMounts[].name`。
应用 CR 后，等待 Grafana Operator 完成 reconcile，并确认 Grafana Pod 中存在两个插件目录。

## 拓扑可视化看板介绍

![Unifabric Topology dashboard](../images/topology-visualization.png)

拓扑图从上到下分为四个区域：

1. **Scale-Out** domain 位于 Node 上方。Tier 1 离 Node 行最近，更高的 tier 依次向上排列。
2. **Scale-Up** domain 位于 Scale-Out 和 Node 行之间。
3. **Node** 卡片组成三类拓扑共用的节点行。
4. **Storage** domain 位于 Node 行下方。

每张 domain 卡片显示拓扑类型、tier、domain 名称和成员交换机。连线表示 domain 的父子关系及
Node 归属。选择 domain、交换机或 Node 后，面板会突出显示与其直接相连的边。再次选择该卡片
或选择画布空白处，可以清除选择状态。

卡片可以拖动，便于临时调整观察位置。左下角控件分别用于放大、缩小和让整张图重新适应面板。

### 查看资源详情

将鼠标移到卡片上，可以查看对应资源：

- Node 卡片加载对应的 `FabricNode` 资源。
- Switch View 中的交换机卡片加载对应的 `Switch` 资源。
- domain 卡片或交换机组显示对应 `Topology` domain 中的字段。

详情浮层提供 **View raw JSON** 操作。出现 `Not found` 表示该名称存在于拓扑中，但当前没有找到
对应的 FabricNode 或 Switch 资源。

## 排障

### Dashboard 没有被导入

- 确认 `topologyAPI.enabled=true`。
- `grafanaDashboard.enabled` 默认为 `true`；如果安装时显式关闭了该设置，需要重新开启。
- 确认 `grafanaDashboard.kind=GrafanaDashboard`，并且实例选择条件与
  `grafanaDashboard.instanceSelector` 一致。

### 拓扑图中没有 Domain

- 执行 `kubectl get topologies`，检查各资源的 `status.domains` 和 `status.nodes`。
- 确认拓扑发现已经完成，并且 Controller cache 已同步。
- 确认 dashboard 的 Cluster 选择与 API 返回的集群名称一致。

### 节点出现在 Outside topology 中

面板会比较 `FabricNode.metadata.name` 与所有 `Topology.status.nodes` 中的 Node 名称，并据此生成
该分组。请检查相关 Node 的拓扑标签和当前 Topology 资源。拓扑发现更新 status 后，刷新
dashboard。

### 交换机或 Node 详情显示 Not found

即使引用的 `Switch` 或 `FabricNode` 资源不存在，Topology 资源中的 domain 仍然可以显示。
可以通过 `kubectl get switch <name>` 或 `kubectl get fabricnode <name>` 检查对应资源。

API 和插件架构说明见[拓扑可视化设计](../design/topology-visualization.zh.md)。