# Deploy Unifabric

中文版: [getting-started.zh.md](./getting-started.zh.md)

## Introduction and Deployment Goals

This guide installs the Unifabric base components with Helm: the Controller, the
Agent, and the observability components. The default installation works on a
SONiC RoCE fabric. If the cluster uses InfiniBand or Spectrum-X, the reader
changes a small set of Helm options.

After you finish this guide, the cluster will show the following results:

- A `FabricNode` resource exists for every node and lists its RDMA NICs by
  purpose: scale-out, storage, or scale-up.
- The topology-discovery and RDMA-metrics components are ready. Node topology
  labels and RDMA metrics are feature results; see the
  [topology discovery guide](./usage/topology-discovery.md) and the
  [host and Pod RDMA NIC metrics guide](./usage/traffic-observability/rdma-metrics.md).

## Prerequisites

Make sure that each item below is true before you continue:

- [ ] You have access to the target Kubernetes cluster.
- [ ] The cluster nodes have RDMA NICs that connect to a SONiC RoCE,
      InfiniBand, or NVIDIA Spectrum-X fabric, and the RDMA driver (for
      example NVIDIA DOCA-OFED) is installed on the nodes.
- [ ] `kubectl` and Helm (v3.13 or later) are installed on the host that you
      use for the installation.
- [ ] Prometheus Operator and Grafana Operator (v5 or later, which provides
      the `grafana.integreatly.org/v1beta1` API) are installed in the cluster
      to collect RDMA metrics and load the Grafana dashboards. Topology
      visualization also requires Grafana 12.1 or later; see the
      [topology visualization guide](./usage/topology-visualization.md).

## Installation

The command below installs the latest Unifabric release. Helm resolves the
latest chart version automatically. To install a specific version, add
`--version <x.y.z>`; the available versions are listed on the
[releases](https://github.com/unifabric-io/unifabric/releases) page. Run it on
the host with `kubectl` and Helm access to the cluster:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --create-namespace \
  --wait
```

1. The chart sets `topoDiscovery.scaleOut.mode` to `unifabric-roce` by default.
This mode is for clusters where SONiC switches carry the RoCE network. RoCE is
RDMA over Converged Ethernet. Unifabric discovers the scale-out topology from
LLDP neighbors. LLDP is a protocol that devices use to announce themselves to
their link peers. Make sure that LLDP is enabled on every switch. If the
cluster uses an InfiniBand or Spectrum-X fabric instead, change the
topology-discovery options as described in
[Other Network Environments](#other-network-environments).

> With node-side LLDP alone, discovery produces only tier 1: the leaf switches
that the nodes connect to. To discover the spine and core tiers above the
leaves, declare the upper switches or run the switch-agent on the switches after
the installation. The [topology discovery guide](./usage/topology-discovery.md)
describes both options.

2. By default, Unifabric classifies all RDMA NICs on a node as scale-out. If a node
has RDMA NICs for different purposes, add the matching selector with
`helm upgrade --install` after the base installation.

> For example, a node has six RDMA NICs named `eth1` through `eth6`. The NICs
`eth1` through `eth4` carry scale-out traffic, and `eth5` and `eth6` carry
storage traffic. Reference architectures of standard GPU servers, such as
DGX H100/B200 and GB200/GB300 NVL72, commonly provide two dedicated storage
NICs. Run this command, which also enables topology discovery for the storage
network:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set fabricNode.storageInterfaceSelector="interface=eth5\,eth6" \
  --set topoDiscovery.storage.mode=unifabric-roce \
  --wait
```

> Unifabric then classifies `eth5` and `eth6` as storage. The other four RDMA
NICs remain scale-out by the default rule. The selector also supports wildcards
and CIDRs, for example `interface=eth*,!eth9` or `cidr=172.17.0.0/16`. The
[Helm values reference](../chart/README.md) describes the complete selector
syntax.

3. In mainland China, run this command to accelerate image pulls:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set global.registry=m.daocloud.io \
  --set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
  --set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
  --set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph \
  --wait
```

## Other Network Environments

The default installation is for a SONiC RoCE fabric. If the cluster uses
InfiniBand or Spectrum-X, keep the same installation flow and change only the
topology-discovery options in the matching subsection below.

### InfiniBand Fabric

Use this section when the cluster uses an NVIDIA InfiniBand fabric, for example
Mellanox NICs in IB mode connected to IB switches. Make sure that RDMA devices
in IB mode exist on the nodes.

After the base installation, run this command to select NVIDIA Topograph as
the topology writer for the scale-up and scale-out networks:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --wait
```

### Spectrum-X Fabric

Use this section when the cluster uses Spectrum-X switches and NetQ manages the
network. NetQ is the NVIDIA network management platform for Spectrum-X fabrics,
and NVIDIA Topograph reads the fabric topology through the NetQ API. Make sure
that each item below is true:

- [ ] NetQ manages the Spectrum-X fabric, and NetQ holds the fabric topology
      data.
- [ ] The cluster can reach the NetQ API.
- [ ] You have the NetQ API URL, the username, and the password.

First create the namespace and the NetQ credentials Secret. The commands below
do not write the credentials to local files, Helm values, or ConfigMaps. The
Secret must contain a key named `credentials.yaml`:

```bash
kubectl create namespace unifabric-system --dry-run=client -o yaml | kubectl apply -f -

read -r -p "NetQ username: " NETQ_USERNAME
read -r -s -p "NetQ password: " NETQ_PASSWORD
printf '\n'
printf 'username: %s\npassword: %s\n' "${NETQ_USERNAME}" "${NETQ_PASSWORD}" |
  kubectl -n unifabric-system create secret generic netq-credentials \
    --from-file=credentials.yaml=/dev/stdin
unset NETQ_USERNAME NETQ_PASSWORD
```

```text
namespace/unifabric-system created
NetQ username: admin
NetQ password:
secret/netq-credentials created
```

A production cluster must also enable encryption at rest for Kubernetes Secrets
and restrict the RBAC permissions that read this Secret.

Then run:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
  --wait
```

Option description:

| Helm value | Purpose |
| --- | --- |
| `topoDiscovery.scaleOut.mode` | Set to `nv-topograph` so that NVIDIA Topograph writes the scale-out labels. |
| `nvidiaTopograph.provider.name` | Set to `netq` to discover the topology through the NetQ API. |
| `nvidiaTopograph.provider.params.apiUrl` | The NetQ API URL. |
| `nvidiaTopograph.credentialsSecretName` | The name of the existing NetQ credentials Secret. The Secret must contain the `credentials.yaml` key. The credentials are mounted read-only into the Topograph Pod. |

## Verification

### Inspect FabricNode NIC Information

Make sure that the Controller and Agent Pods are running and that a `FabricNode`
exists for every node:

```bash
kubectl -n unifabric-system get pods
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

```text
NAME                                    READY   STATUS    RESTARTS   AGE
unifabric-agent-7tkfm                   2/2     Running   0          3m12s
unifabric-agent-x5d9z                   2/2     Running   0          3m12s
unifabric-controller-6f9c7b5d44-l2wqv   1/1     Running   0          3m12s

NAME    TOTALNICS   READY   ROLE   NODEIP        TOPOLOGIES
node1   6           True    GPU    10.0.10.11
node2   6           True    GPU    10.0.10.12
```

Make sure that `status.totalNics`, `status.scaleOutNics`, and
`status.storageNics` match the real NIC configuration of the node. For the
six-NIC example above, the result looks like this:

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node1
status:
  totalNics: 6
  scaleOutNics:
    - name: eth1
      rdmaDeviceName: mlx5_0
      rdma: true
      state: up
    - name: eth2
      rdmaDeviceName: mlx5_1
      rdma: true
      state: up
    - name: eth3
      rdmaDeviceName: mlx5_2
      rdma: true
      state: up
    - name: eth4
      rdmaDeviceName: mlx5_3
      rdma: true
      state: up
  storageNics:
    - name: eth5
      rdmaDeviceName: mlx5_4
      rdma: true
      state: up
    - name: eth6
      rdmaDeviceName: mlx5_5
      rdma: true
      state: up
```

Running Pods and a `FabricNode` status that matches the node confirm that the
deployment is complete.

## Next Steps

After the deployment, continue with the feature guides:

- Go deeper into topology discovery: declare upper SONiC switches, deploy the
  switch-agent, and understand the label semantics and the `Topology` resource
  with the [topology discovery guide](./usage/topology-discovery.md).
- Use the labels in a scheduler with the
  [Kueue TAS workload example](./usage/workload-tas.md).
- Render the topology with the
  [topology visualization guide](./usage/topology-visualization.md).
- Observe RDMA metrics with the [host and Pod RDMA NIC metrics guide](./usage/traffic-observability/rdma-metrics.md).
- Tune the installation with the [Helm values reference](../chart/README.md).

## Troubleshooting

### FabricNode Does Not Discover the Expected RDMA NICs

First make sure that the preconditions hold:

1. Make sure that the Agent Pods are running:

   ```bash
   kubectl -n unifabric-system get daemonset unifabric-agent
   kubectl -n unifabric-system logs daemonset/unifabric-agent -c agent
   ```

   ```text
   NAME              DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
   unifabric-agent   2         2         2       2            2            <none>          12m

   time=2026-10-20T09:52:31.104Z level=INFO msg="starting agent" node=node1
   time=2026-10-20T09:52:31.388Z level=INFO msg="discovered RDMA device" device=mlx5_0 ifname=eth1 kind=scaleOut
   time=2026-10-20T09:52:31.391Z level=INFO msg="discovered RDMA device" device=mlx5_4 ifname=eth5 kind=storage
   ```

2. Make sure that the RDMA devices of the node are visible to the Agent:

   ```bash
   kubectl -n unifabric-system exec daemonset/unifabric-agent -c agent -- \
     ls /sys/class/infiniband
   ```

   ```text
   mlx5_0  mlx5_1  mlx5_2  mlx5_3  mlx5_4  mlx5_5
   ```

3. Make sure that `fabricNode.scaleOutInterfaceSelector`,
   `storageInterfaceSelector`, and `scaleUpInterfaceSelector` match the real
   interface names or CIDRs.
4. Make sure that no NIC is misclassified. NICs that the storage or scale-up
   selector matches do not appear in `status.scaleOutNics`.

## Uninstall

Remove the Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
release "unifabric" uninstalled
```

If you no longer need the CRDs, delete them. This also deletes all `FabricNode`,
`Switch`, and `Topology` resources:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
customresourcedefinition.apiextensions.k8s.io "fabricnodes.unifabric.io" deleted
customresourcedefinition.apiextensions.k8s.io "switches.unifabric.io" deleted
customresourcedefinition.apiextensions.k8s.io "topologies.unifabric.io" deleted
```
