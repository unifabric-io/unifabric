# Topology Aware Scheduling

中文版: [workload-tas.zh.md](./workload-tas.zh.md)

Data centers are usually divided into organizational units such as racks and
blocks. Nodes in the same organizational unit usually have shorter network
distance and better bandwidth, while nodes across racks or blocks are farther
apart. Topology aware scheduling expresses these hierarchy relationships as
Kubernetes Node labels, so each node declares the topology domain it belongs to.
Unifabric or NVIDIA Topograph discovers the topology and writes it back as Node
labels; Kueue, Volcano, or KAI Scheduler ultimately consumes these labels to
place workloads.

Unifabric uses Helm values `topologyLabels.*` to define the label keys consumed
by schedulers:

| Scheduling level | Default Node label | Usage |
| --- | --- | --- |
| scale-up | `unifabric.io/scale-up` | GPU high-speed interconnect domain, such as an NVLink domain |
| scale-out leaf | `unifabric.io/scale-out-leaf` | Leaf-level scale-out topology domain, directly connected to hosts |
| scale-out spine | `unifabric.io/scale-out-spine` | Spine-level scale-out topology domain, upstream of leaf switches |
| scale-out core | `unifabric.io/scale-out-core` | Core-level scale-out topology domain, upstream of spine switches |
| node | `kubernetes.io/hostname` | Kubernetes default node label, the finest-grained node-level topology domain |

Schedulers can only use labels that are actually written to Nodes. When
configuring Kueue `Topology`, Volcano, or KAI Scheduler, topology levels should
be ordered from coarse to fine. If a label at a given level does not cover all
nodes participating in scheduling, do not add it to the topology configuration.

## Install Topology Discovery

Before using a TAS queue, complete the Unifabric installation for your fabric
scenario. Then validate the topology results below so you know the expected
Node labels are in place. Use the scenario links below to jump to the
corresponding document.

- [General SONiC RoCE](./getting-started-sonic-roce.md): For scenarios where SONiC switches carry the RoCE network.
- [Spectrum-X fabric](./getting-started-spectrum-x.md): For Spectrum-X switch scenarios.
- [InfiniBand fabric](./getting-started-infiniband.md): For NVIDIA InfiniBand network scenarios.

## Validate Topology Results

Validate against the labels that actually exist on Nodes: confirm that nodes
have the topology labels mentioned above with the `unifabric.io/` prefix.
Different fabric scenarios may write different label levels.

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

Check:

- Nodes participating in scheduling have the expected `unifabric.io/` topology
  labels.

`FabricNode` and `ScaleOutLeafGroup` CRs are generated only in the SONiC RoCE
scenario. Spectrum-X and InfiniBand scenarios write Node labels through NVIDIA
topograph and do not create or update these CRs.

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
kubectl get scaleoutleafgroups -o wide
kubectl get scaleoutleafgroup <group-name> -o yaml
```

`FabricNode` output example:

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node-gpu-1
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: All FabricNode conditions are ready
  - type: LLDPNeighborsReady
    status: "True"
    reason: LLDPNeighborsReady
    message: All selected RDMA interfaces have LLDP neighbors
  totalNics: 2
  nodeRole: GPU
  nodeIP: 192.168.200.6
  scaleOutNics:
  - name: eth1
    rdma: true
    rdmaDeviceName: mlx5_0
    state: up
    ipv4: 172.17.1.11/24
    ipv6: ""
    lldpNeighbor:
      hostname: gpu-leaf-1
      mgmtIP: 10.10.10.1
      mac: 00:11:22:33:44:55
      port: Ethernet1/1
      description: GPU leaf switch
```

`ScaleOutLeafGroup` output example:

```yaml
apiVersion: unifabric.io/v1beta1
kind: ScaleOutLeafGroup
metadata:
  name: 6f8a91c2
status:
  healthy: true
  healthyNodes: 2
  totalNodes: 2
  nodes:
  - name: node-gpu-1
    healthy: true
  - name: node-gpu-2
    healthy: true
  switches:
  - name: gpu-leaf-1
    mgmtIP: 10.10.10.1
  - name: gpu-leaf-2
    mgmtIP: 10.10.10.2
```

- `Ready` and `LLDPNeighborsReady` in `FabricNode.status.conditions` are `True`.
- `ScaleOutLeafGroup.status.nodes` contains the expected nodes.
- Nodes have `unifabric.io/scale-out-leaf=<group-name>`.

Once these checks pass, the topology discovery result is exposed through
Kubernetes Node labels and you can continue configuring scheduling queues.

## Kueue Example

The examples use the current Kueue `kueue.x-k8s.io/v1beta2` API. If your
cluster runs an older Kueue version, adjust the API version to match the API
version supported by that version's CRDs.

### Prerequisites

- Unifabric has been installed for the corresponding fabric scenario in the
  [documentation index](../README.md), and you have validated the topology
  results in the section above.
- Kueue is installed with the `TopologyAwareScheduling` feature gate enabled.
- Nodes participating in topology aware scheduling have the `kubernetes.io/hostname` label.
- Nodes participating in topology aware scheduling already have the topology labels used by this example, such as `unifabric.io/scale-out-leaf`.
- GPU examples require nodes to report allocatable `nvidia.com/gpu` resources.
- If GPU nodes have the `nvidia.com/gpu:NoSchedule` taint, declare the corresponding toleration in `ResourceFlavor.spec.tolerations`.

Reconfirm the Node labels that the Kueue Topology will consume:

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

If a GPU node does not have the expected topology label, return to the
previous section or the corresponding installation guide to troubleshoot
topology discovery components.

### Create Kueue Topology Aware Scheduling Queues

The following example creates:

- A Kueue `Topology` that uses the `unifabric.io/scale-out-leaf` Node label.
- A GPU `ResourceFlavor` that references the Topology.
- A `LocalQueue` for the `training` namespace.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: training
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: Topology
metadata:
  name: unifabric-scaleout
spec:
  levels:
  - nodeLabel: unifabric.io/scale-out-leaf
  - nodeLabel: kubernetes.io/hostname
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ResourceFlavor
metadata:
  name: unifabric-gpu-tas
spec:
  topologyName: unifabric-scaleout
  tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule
  # If only a subset of nodes should enter this flavor, enable this
  # configuration with labels that match your cluster.
  # nodeLabels:
  #   node-role.kubernetes.io/gpu: "true"
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: unifabric-gpu-tas
spec:
  namespaceSelector: {}
  resourceGroups:
  - coveredResources:
    - cpu
    - memory
    - nvidia.com/gpu
    flavors:
    - name: unifabric-gpu-tas
      resources:
      - name: cpu
        nominalQuota: 160
      - name: memory
        nominalQuota: 2Ti
      - name: nvidia.com/gpu
        nominalQuota: 16
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: LocalQueue
metadata:
  namespace: training
  name: tas
spec:
  clusterQueue: unifabric-gpu-tas
```

After applying it, confirm that the queues are available:

```bash
kubectl get topology,resourceflavor,clusterqueue
kubectl -n training get localqueue
```

If NVIDIA topograph or another component has already written spine/core labels
in your cluster, you can configure a fuller Kueue `Topology`. For example:

```yaml
apiVersion: kueue.x-k8s.io/v1beta2
kind: Topology
metadata:
  name: unifabric-scaleout
spec:
  levels:
  - nodeLabel: unifabric.io/scale-out-spine
  - nodeLabel: unifabric.io/scale-out-leaf
  - nodeLabel: kubernetes.io/hostname
```

Before configuring this, confirm that all nodes participating in this
`ResourceFlavor` actually have the `unifabric.io/scale-out-spine` label. In
topograph scenarios, do not use `ScaleOutLeafGroup` to determine whether these
labels have been generated. Use the actual labels on Nodes.

### Example 1: Prefer the Same Leaf Group

`kueue.x-k8s.io/podset-preferred-topology` means Kueue should try to place the
same PodSet into the same domain at the specified topology level. If a single
leaf group does not have enough capacity, Kueue can place the workload at a
higher level or across domains, avoiding long pending times.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: tas-preferred-
  namespace: training
  labels:
    kueue.x-k8s.io/queue-name: tas
spec:
  parallelism: 4
  completions: 4
  completionMode: Indexed
  template:
    metadata:
      annotations:
        kueue.x-k8s.io/podset-preferred-topology: unifabric.io/scale-out-leaf
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: busybox:1.36
        command: ["sh", "-c", "sleep 3600"]
        resources:
          requests:
            cpu: "8"
            memory: 32Gi
            nvidia.com/gpu: "1"
          limits:
            nvidia.com/gpu: "1"
```

Use this when:

- You want to reduce cross-leaf RDMA traffic, but throughput and queue time matter more.
- A single training job can tolerate some cross-leaf placement.

### Example 2: Require the Same Leaf Group

`kueue.x-k8s.io/podset-required-topology` means the same PodSet must be placed
entirely within one domain at the specified topology level. If no leaf group
can fit all Pods at the same time, the Workload remains not admitted until
resources or configuration change.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: tas-required-
  namespace: training
  labels:
    kueue.x-k8s.io/queue-name: tas
spec:
  parallelism: 8
  completions: 8
  completionMode: Indexed
  template:
    metadata:
      annotations:
        kueue.x-k8s.io/podset-required-topology: unifabric.io/scale-out-leaf
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: busybox:1.36
        command: ["sh", "-c", "sleep 3600"]
        resources:
          requests:
            cpu: "8"
            memory: 32Gi
            nvidia.com/gpu: "1"
          limits:
            nvidia.com/gpu: "1"
```

Use this when:

- Training workloads such as AllReduce and AllGather are sensitive to cross-leaf bandwidth.
- You prefer waiting when topology requirements cannot be met instead of running across leaf groups.

### Verify Scheduling Results

Check Kueue Workload admission:

```bash
kubectl -n training get workloads.kueue.x-k8s.io
kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
```

Check actual Pod placement:

```bash
kubectl -n training get pods -o wide
```

Check the topology labels on the Nodes where Pods landed:

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

If the Job annotation uses `unifabric.io/scale-out-leaf`:

- For the `required` example, all Pod nodes should have the same `unifabric.io/scale-out-leaf` value.
- For the `preferred` example, Kueue first tries same-leaf placement; when resources are insufficient, it may place Pods across leaf groups.

If the Job annotation uses `unifabric.io/scale-out-spine` or another
higher-level label, check whether all Pod nodes have the same label value at
that level.

### Troubleshooting

Workload remains not admitted:

- Check whether `kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>` reports insufficient topology domain capacity.
- Confirm that `ClusterQueue` `nominalQuota` covers the Job's requested `cpu`, `memory`, and `nvidia.com/gpu`.
- Confirm that target nodes are Ready, not cordoned, and have enough resources in `status.allocatable`.
- Confirm that participating nodes have all labels declared in Kueue `Topology.spec.levels[*].nodeLabel`.
- If `Topology.spec.levels` includes `unifabric.io/scale-up`, `unifabric.io/scale-out-spine`, or `unifabric.io/scale-out-core`, confirm that these labels have been written to Nodes.

Pod is admitted but cannot be scheduled:

- Check whether GPU taints are covered by `ResourceFlavor.spec.tolerations`.
- If `ResourceFlavor.spec.nodeLabels` is enabled, confirm that nodes actually have those labels.
- Check Pod events:

  ```bash
  kubectl -n training describe pod <pod-name>
  ```

Topology labels are missing or unstable:

- Return to the corresponding installation guide to troubleshoot topology discovery components:
  [General SONiC RoCE](../getting-started-sonic-roce.md),
  [Spectrum-X fabric](../getting-started-spectrum-x.md), or
  [InfiniBand fabric](../getting-started-infiniband.md).
- Confirm the topology labels that actually exist on Nodes:

  ```bash
  kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
  ```

- If Helm values `topologyLabels.*` are customized, the Kueue
  `Topology.spec.levels[*].nodeLabel` and Job annotation values must be updated
  accordingly.
