# Topology-Aware Scheduling

中文版: [workload-tas.zh.md](./workload-tas.zh.md)

## Introduction

Topology-aware scheduling places the Pods of one workload into the same network
topology domain, so that cross-node RDMA traffic stays on short paths. Unifabric or
NVIDIA Topograph discovers the topology and writes it to Kubernetes Nodes as labels.
A scheduler such as Kueue, Volcano, or KAI Scheduler consumes these labels. A
scheduler can only use labels that exist on the Nodes. This guide shows the full path
with Kueue. Volcano and KAI Scheduler consume the same labels through their own
configuration.

The Helm values `topoDiscovery.*.nodeLabel.keyTemplate` define the label keys. The
defaults are:

| Scheduling level | Default Node label | Meaning |
| --- | --- | --- |
| scale-up tier 1 | `scale-up.unifabric.io/tier-1` | GPU high-speed interconnect domain, such as an NVLink domain |
| scale-out tier 1 | `scale-out.unifabric.io/tier-1` | First scale-out tier, directly connected to hosts |
| scale-out tier 2 | `scale-out.unifabric.io/tier-2` | Second scale-out tier, upstream of tier 1 |
| scale-out tier 3 | `scale-out.unifabric.io/tier-3` | Third scale-out tier, upstream of tier 2 |
| node | `kubernetes.io/hostname` | The default Kubernetes node label, the finest topology domain |

> **Note:** If the `topoDiscovery.*.nodeLabel.keyTemplate` values are customized, use
> the customized keys in every scheduler configuration and workload annotation in this
> guide.

## Prerequisites

- Unifabric is installed through the [basic installation](../getting-started.md), and
  the scenario guide for your fabric is complete:
  [SONiC RoCE](../getting-started-sonic-roce.md),
  [Spectrum-X](../getting-started-spectrum-x.md), or
  [InfiniBand](../getting-started-infiniband.md).
- Kueue is installed with the `TopologyAwareScheduling` feature gate enabled.
- The GPU examples need Nodes that report allocatable `nvidia.com/gpu` resources.

## Configuration and Usage

### Step 1: Make Sure That the Topology Labels Exist on the Nodes

The labels on the Nodes are the source of truth for the scheduler. Different fabric
scenarios write different label levels:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

```text
<!-- TODO: capture output from a real environment -->
```

Every Node that takes part in topology-aware scheduling must have the labels that the
Kueue `Topology` will reference. If a label level does not cover all participating
Nodes, do not add that level to the Kueue `Topology`.

### Step 2: Create the Kueue Queues

The example below creates four resources. A Kueue `Topology` lists the topology
levels from coarse to fine and ends with `kubernetes.io/hostname`. A `ResourceFlavor`
binds GPU Nodes to that `Topology` and tolerates the `nvidia.com/gpu:NoSchedule`
taint. A `ClusterQueue` sets the resource quota. A `LocalQueue` exposes the
`ClusterQueue` to the `training` namespace.

> **Note:** The example uses the Kueue `kueue.x-k8s.io/v1beta2` API. If the cluster
> runs an older Kueue, set the API version that its CRDs support.

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
  - nodeLabel: scale-out.unifabric.io/tier-1
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

> **Note:** To admit only a subset of Nodes into the flavor, set
> `ResourceFlavor.spec.nodeLabels` to labels that exist on those Nodes, for example
> `node-role.kubernetes.io/gpu: "true"`.

Apply the file, then make sure that the queues exist:

```bash
kubectl apply -f tas-queues.yaml
kubectl get topology,resourceflavor,clusterqueue
kubectl -n training get localqueue
```

```text
<!-- TODO: capture output from a real environment -->
```

> **Note:** When all participating Nodes also carry `scale-out.unifabric.io/tier-2`,
> add that level above tier 1 for a deeper hierarchy:
>
> ```yaml
> spec:
>   levels:
>   - nodeLabel: scale-out.unifabric.io/tier-2
>   - nodeLabel: scale-out.unifabric.io/tier-1
>   - nodeLabel: kubernetes.io/hostname
> ```
>
> Before you add a level, make sure that every Node in the flavor has that label.

### Step 3: Annotate the Workload

A Job selects its placement rule with one annotation on the Pod template. Two cases
exist:

- Case 1, prefer the same tier 1 domain: throughput and queue time matter more than
  strict locality, and the job tolerates some cross-domain placement.
- Case 2, require the same tier 1 domain: the job is sensitive to cross-domain
  bandwidth, for example AllReduce or AllGather training, and waiting is better than
  running across domains.

#### Case 1: Prefer the Same Tier 1 Domain

The `kueue.x-k8s.io/podset-preferred-topology` annotation tells Kueue to try to place
the whole PodSet into one domain at the given level. If one domain does not have
enough capacity, Kueue places the PodSet at a higher level or across domains, which
avoids long pending times.

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
        kueue.x-k8s.io/podset-preferred-topology: scale-out.unifabric.io/tier-1
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

#### Case 2: Require the Same Tier 1 Domain

The `kueue.x-k8s.io/podset-required-topology` annotation tells Kueue to place the
whole PodSet into one domain at the given level. If no domain can fit all Pods at the
same time, the Workload stays not admitted until resources or configuration change.

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
        kueue.x-k8s.io/podset-required-topology: scale-out.unifabric.io/tier-1
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

## Verification

Submit a Job from Case 1 or Case 2, then make sure that Kueue admitted the Workload:

```bash
kubectl create -f tas-job.yaml
kubectl -n training get workloads.kueue.x-k8s.io
kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
```

```text
<!-- TODO: capture output from a real environment -->
```

Make sure that the Pods landed on Nodes that share one topology domain:

```bash
kubectl -n training get pods -o wide
kubectl get nodes -L scale-out.unifabric.io/tier-1
```

```text
<!-- TODO: capture output from a real environment -->
```

The verification passes under these conditions:

- For Case 2, all Pod Nodes have the same `scale-out.unifabric.io/tier-1` value.
- For Case 1, Kueue first tries one domain. When capacity is insufficient, Pods can
  land across domains, and the Workload is still admitted.
- For an annotation at a higher level, such as `scale-out.unifabric.io/tier-2`, all
  Pod Nodes share the label value at that level.

## Troubleshooting

### The Workload Stays Not Admitted

First make sure that the preconditions hold: the participating Nodes have every label
that `Topology.spec.levels[*].nodeLabel` declares, and the Nodes are Ready and not
cordoned.

1. Read the admission events:

   ```bash
   kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
   ```

   A message about insufficient topology domain capacity means that no single domain
   fits the PodSet.

2. Make sure that the `ClusterQueue` `nominalQuota` covers the `cpu`, `memory`, and
   `nvidia.com/gpu` requests of the Job.
3. Make sure that the target Nodes have enough resources in `status.allocatable`.

### The Workload Is Admitted but the Pods Stay Pending

1. Make sure that `ResourceFlavor.spec.tolerations` covers the GPU taints.
2. If `ResourceFlavor.spec.nodeLabels` is set, make sure that the Nodes have those
   labels.
3. Read the Pod events:

   ```bash
   kubectl -n training describe pod <pod-name>
   ```

### The Topology Labels Are Missing or Unstable

1. List the labels that exist on the Nodes:

   ```bash
   kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
   ```

2. In the SONiC RoCE scenario, inspect the discovery inputs. The Spectrum-X and
   InfiniBand scenarios write labels through NVIDIA Topograph and do not create these
   resources. For these two scenarios, go to the scenario guide directly.

   ```bash
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

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

   Make sure that `Ready` and `LLDPNeighborsReady` in `status.conditions` are `True`.

   ```bash
   kubectl get switches -o wide
   kubectl get switch <switch-name> -o yaml
   ```

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: gpu-leaf-1
   spec:
     mgmtIP: 10.10.10.1
     role: ScaleOut
   status:
     hostname: gpu-leaf-1
     healthy: true
     conditions:
     - type: Connected
       status: "True"
       reason: StreamReady
     lldpNeighborCount: 2
     lldpNeighbors:
     - remoteSystemType: KubernetesNode
       remoteSystemName: node-gpu-1
     - remoteSystemType: Switch
       remoteSystemName: gpu-spine-1
   ```

   Make sure that `status.healthy` is `true` and that `status.lldpNeighbors` contains
   the expected node and switch peers.

3. If the labels are still missing, go to the troubleshooting section of the scenario
   guide: [SONiC RoCE](../getting-started-sonic-roce.md),
   [Spectrum-X](../getting-started-spectrum-x.md), or
   [InfiniBand](../getting-started-infiniband.md).

## References

- [Topology reference](../reference/topology.md): the aggregated topology resource.
- [FabricNode reference](../reference/fabricnode.md): the node-side discovery input.
- [Switch reference](../reference/switch.md): the switch-side discovery input.
- [Scale-out topology design](../design/scaleout-topology.md): how discovery builds
  the tiers and the labels.
