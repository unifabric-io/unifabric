# 拓扑可视化

English version: [topology-visualization.md](./topology-visualization.md)

## 简介

拓扑可视化把集群已发现的 Scale-out、Scale-up 和 Storage 拓扑渲染为 Grafana 中的一张
交互式拓扑图。本功能只读取 `Topology`、`FabricNode` 和 `Switch` 资源,展示拓扑发现
已经产出的数据。它本身不做拓扑发现,也不展示实时流量或链路利用率。如果集群中没有
至少一个已填充数据的 `Topology` 资源,拓扑图为空。

本功能由三部分组成:Controller 提供 Topology HTTP API;Grafana 数据源插件查询该 API;
Grafana 面板插件渲染拓扑图。

> **注意:** 使用 NVIDIA Topograph 数据源(`nv-topograph` 模式)时,交换机成员卡片
> 为空,除非管理员创建仅含标签的 `Switch` 资源。
> 参见[拓扑 CRD 设计](../design/topology-crd.zh.md)。

## 前提条件

- 已按[基础安装](../getting-started.zh.md)安装 Unifabric,并完成某个场景指南,
  例如 [SONiC RoCE](../getting-started-sonic-roce.zh.md)。
- 集群中已安装 Grafana Operator 及其 CRD。
- 以下命令返回至少一个 `Topology` 资源:

  ```bash
  kubectl get topologies.unifabric.io
  ```

  ```text
  NAME       AGE
  scaleout   4h27m
  scaleup    4h27m
  storage    4h27m
  ```

## 配置与使用

本功能有两种 case。集群自身没有 Grafana 时,使用内置 Grafana 实例;已有团队运维
Grafana 时,使用外部实例。

### Case 1:使用内置 Grafana 实例

1. 启用 Topology API 和内置 Grafana 实例。在一台有集群 Helm 访问权限的主机上执行:

   ```bash
   helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     --set topologyAPI.enabled=true \
     --set grafanaInstance.enabled=true \
     --wait
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   Chart 会创建 `Grafana`、`GrafanaDatasource` 和 `GrafanaDashboard`。数据源连接
   同一 release 的 Controller Topology API。内置镜像
   `ghcr.io/unifabric-io/unifabric-grafana:<version>` 已包含两个插件。

   > **警告:** Topology API 自身没有认证。生产环境中,启用
   > `topologyAPI.enabled=true` 时必须配合 NetworkPolicy 或 ingress 规则,把访问
   > 限制在 Grafana 实例内。否则集群中任意 Pod 都能读取拓扑数据。

2. 获取 NodePort 和登录凭据。Service 类型默认是 NodePort,端口随机分配:

   ```bash
   kubectl get service -n unifabric-system unifabric-grafana-service
   kubectl get secret -n unifabric-system unifabric-grafana-admin-credentials \
     -o go-template='{{range $k, $v := .data}}{{$k}}: {{$v | base64decode}}{{"\n"}}{{end}}'
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

3. 在浏览器中打开 `http://<节点 IP>:<NodePort>`,登录后搜索并打开
   `Unifabric Topology` 仪表盘。

### Case 2:使用外部 Grafana 实例

1. 只启用 Topology API:

   ```bash
   helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     --set topologyAPI.enabled=true \
     --wait
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   Chart 会创建 `GrafanaDatasource` 和 `GrafanaDashboard`,并通过
   `grafanaDashboard.instanceSelector` 选择外部实例。默认选择器是 `{}`。

   > **注意:** 如果集群中有多个 Grafana 实例,必须把选择器设置为匹配目标实例,
   > 否则仪表盘会落到错误的 Grafana 上。例如,当外部 `Grafana` 资源带有
   > `dashboards: grafana` 标签时,设置:
   >
   > ```yaml
   > grafanaDashboard:
   >   instanceSelector:
   >     matchLabels:
   >       dashboards: grafana
   > ```

2. 为外部实例安装插件。如果实例可以直接使用 Unifabric Grafana 镜像,更新其
   `Grafana` 资源的 `spec.version`:

   ```yaml
   spec:
     version: ghcr.io/unifabric-io/unifabric-grafana:<version>
   ```

   如果实例必须保留当前镜像,用 init 容器安装插件。以下配置假设 `grafana-plugins`
   共享卷在 Grafana 容器内挂载于 `/var/lib/grafana/plugins`,并允许 Grafana 加载
   两个未签名插件:

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

   > **注意:** 如果共享卷名称不同,把 init 容器中的 `volumeMounts[].name` 改成
   > 实际名称。

3. 应用资源后,等待 Grafana Operator 完成调谐,确认 Grafana Pod 中两个插件目录
   都存在。

### 查看仪表盘

![Unifabric Topology 仪表盘](../images/topology-visualization.png)

拓扑图分为四个纵向区域:Scale-out 域位于 Node 行上方,tier 1 最靠近 Node;
Scale-up 域位于 Scale-out 与 Node 行之间;Node 卡片构成所有拓扑类型共用的一行;
Storage 域位于 Node 行下方。

每张域卡片展示其拓扑类型、tier、域名称和成员交换机。连线表示父子关系和 Node 归属。
点击某个域、交换机或 Node,会高亮它的直接连接;再次点击它或点击空白画布,取消选中。
使用左下角控件可以缩放,或让拓扑图适配面板大小。

悬停在卡片上可以查看它背后的资源:Node 卡片加载对应的 `FabricNode`,交换机卡片
加载对应的 `Switch`。详情弹窗提供 `View raw JSON` 操作。

## 验证

在 Grafana 中打开 `Unifabric Topology` 仪表盘。当拓扑图展示的域卡片、Node 行和
连线与物理网络一致时,验证通过。

![预期的仪表盘效果](../images/topology-visualization.png)
<!-- TODO: 待真实环境采集截图 -->

## 故障排查

### 仪表盘未导入

先确认前提条件成立:

1. 确认 `topologyAPI.enabled=true`。
2. 确认 `grafanaDashboard.enabled=true`。默认值是 `true`;如果安装时被关闭,重新启用。
3. 确认 `grafanaDashboard.kind=GrafanaDashboard`,且目标 Grafana 实例匹配
   `grafanaDashboard.instanceSelector`。

### 拓扑图没有任何拓扑域

1. 确认 `kubectl get topologies` 返回资源,且每个资源都有 `status.domains` 和
   `status.nodes`。
2. 确认拓扑发现已完成。参见 [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md)
   的验证步骤。
3. 确认仪表盘的 Cluster 选项与 API 的集群名一致。

### Node 出现在 Outside Topology 分组

面板通过比较 `FabricNode.metadata.name` 与 `Topology.status.nodes` 中的每个 Node
名称来构建该分组。检查受影响 Node 的拓扑标签和当前的 `Topology` 资源。拓扑发现
更新 status 后,刷新仪表盘。

### 交换机或 Node 详情显示 Not found

即使被引用的 `Switch` 或 `FabricNode` 资源不存在,域仍会基于 `Topology` 资源保持
可见。用 `kubectl get switch <name>` 或 `kubectl get fabricnode <name>` 确认被引用
的资源存在。

## 参考

- [FabricNode 参考](../reference/fabricnode.zh.md):Node 详情弹窗展示的字段。
- [拓扑 CRD 设计](../design/topology-crd.zh.md):拓扑数据模型的工作方式。
- [拓扑可视化设计](../design/topology-visualization.zh.md):API 与插件架构。
