# 为 Topograph 场景补充 Switch 资源

English version: [topograph-switch-resources.md](./topograph-switch-resources.md)

## 简介

在 NVIDIA Spectrum-X 和 InfiniBand 场景中,Node label 和 `Topology` 资源由 NVIDIA
Topograph 产生。Unifabric 的 LLDP 发现不参与,所以集群中没有 `Switch` 资源。在
`Topology` 资源中,每个性能域的 `members` 列表为空。在拓扑可视化中,每个域的交换机
成员卡片也为空。调度和 label 不受影响。

如果需要拓扑可视化显示交换机,为每台交换机创建一个仅含标签的 `Switch` 资源。仅含
标签的 `Switch` 资源没有 `mgmtIP`、没有 tier、也不需要 switch-agent。Controller 只
读取它的 `unifabric.io/domain` label,把交换机加入对应的性能域。本步骤是可选的。

## 前提条件

- 已按[基础安装](../../getting-started.zh.md)完成 Spectrum-X 或 InfiniBand 场景的
  部署,并且 `kubectl get nodes -L scale-out.unifabric.io/tier-1` 中已出现拓扑 label。
- 知道每台交换机属于哪个性能域。例如,NetQ 的输出或 `ibnetdiscover` 的输出显示
  `leaf1` 属于哪个 leaf 域。

## 配置与使用

1. 列出 `Topology` 资源中的域名:

   ```bash
   kubectl get topologies.unifabric.io scaleout -o jsonpath='{range .status.domains[*]}{.tier}{"\t"}{.name}{"\n"}{end}'
   ```

   ```text
   2	tier2-group1
   1	tier1-group1
   1	tier1-group2
   ```

2. 为每台交换机创建一个 `Switch` 资源。把 `unifabric.io/domain` 设为该交换机所属的
   域名。把 `role` 设为与 `Topology` 资源一致的值,`ScaleOut` 或 `Storage`:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: leaf1
     labels:
       unifabric.io/domain: tier1-group1
   spec:
     role: ScaleOut
   ---
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine1
     labels:
       unifabric.io/domain: tier2-group1
   spec:
     role: ScaleOut
   EOF
   ```

> **注意:** Topograph 场景下,只有 Topograph 写入 Node label。仅含标签的 `Switch`
> 资源不参与拓扑计算,也不改变任何 Node label。

## 验证

```bash
kubectl get topologies.unifabric.io scaleout -o jsonpath='{range .status.domains[*]}{.name}{"\t"}{.members}{"\n"}{end}'
```

```text
tier2-group1	["spine1"]
tier1-group1	["leaf1"]
tier1-group2	[]
```

对应域的 `members` 中出现交换机名称后,刷新 Grafana 拓扑仪表盘。交换机成员卡片随之
显示该交换机。参见[拓扑可视化](./topology-visualization.zh.md)。

## 参考

- [网络拓扑发现](./README.zh.md):NetQ 导入和 InfiniBand 方式。
- [Switch 参考](../../reference/switch.zh.md):`Switch` 资源的字段。
- [Topology CRD 设计](../../design/topology-crd.zh.md):`switchMember` 的填充规则。
