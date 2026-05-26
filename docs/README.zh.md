# Unifabric 文档

## Get Started

根据集群的物理网络类型，选择对应的安装文档：

- [通用 SONiC RoCE](./getting-started-sonic-roce.zh.md)：适用于 SONiC 交换机承载 RoCE 网络的场景。
- [Spectrum-X fabric](./getting-started-spectrum-x.zh.md)：适用于 Spectrum-X 交换机的场景。
- [InfiniBand fabric](./getting-started-infiniband.zh.md)：适用于 NVIDIA InfiniBand 网络场景。

## Usage Guides

- [RDMA 可观测性使用指南](./usage/rdma-metrics.zh.md)：开启并验证 RDMA metrics、Prometheus 抓取和 Grafana dashboard。
- [switch-agent hostProc LLDP 采集方式](./usage/switch-agent-host-proc.zh.md)：在交换机无法挂载 `lldpd` socket 时，通过宿主机 `/proc` 命名空间采集 LLDP。
- [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)：将 Unifabric scale-out leaf Node label 用于 Kueue Topology Aware Scheduling。

## Design Docs

- [Scale-Out 网络拓扑发现设计](./design/scaleout-topology.zh.md)：基于 Switch 和 SwitchGroup 的 scale-out 拓扑发现与 Node label 写回设计。
- [FabricNode CRD 设计](./design/fabricnode.md)：节点本地 RDMA 拓扑状态资源设计。
- [ScaleOutLeafGroup CRD 设计](./design/scaleoutleafgroup.md)：scale-out leaf 分组和 Node label 写回设计。
- [RDMA 可观测性设计](./design/rdma-metrics.md)：RDMA 指标模型、Pod 归因和采集设计。

## Development

- [NVAIR 开发环境指南](./development/dev-with-nvair.md)：使用 NVAIR 搭建 e2e 拓扑、部署监控组件和安装本地 Unifabric。

## Reference

- [Helm values 参考](../chart/README.md)：chart 参数和默认值。
