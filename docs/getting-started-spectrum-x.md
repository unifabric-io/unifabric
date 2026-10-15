# Deploy Unifabric on a Spectrum-X Fabric

中文版: [getting-started-spectrum-x.zh.md](./getting-started-spectrum-x.zh.md)

## Introduction and Deployment Goals

This guide deploys Unifabric topology discovery in a cluster with Spectrum-X
switches. In this environment, NVIDIA Topograph reads the fabric topology from
the NetQ API, and Unifabric writes the result to Node labels. NetQ is the
NVIDIA network management platform for Spectrum-X fabrics.

After you finish this guide, the cluster will show the following results:

- Nodes carry the `scale-out.unifabric.io/tier-N` topology labels that
  schedulers consume.
- Node RDMA state is observable through Agent metrics, including RDMA device,
  port, priority, and Pod attribution metrics.

If the fabric does not match this environment, select another guide:

- If NetQ is not deployed and the network is InfiniBand, read
  [Deploy Unifabric on an InfiniBand Fabric](./getting-started-infiniband.md).
- If the network is RoCE on SONiC switches, read
  [Deploy Unifabric on a General SONiC RoCE Fabric](./getting-started-sonic-roce.md).

## Prerequisites

Make sure that each item below is true before you continue:

- [ ] Unifabric is installed through the [basic installation](./getting-started.md).
- [ ] The Kubernetes cluster contains the target GPU nodes.
- [ ] RDMA devices are visible under `/sys/class/infiniband` on the nodes.
- [ ] NetQ manages the Spectrum-X fabric, and NetQ contains the fabric topology
      data.
- [ ] The cluster can reach the NetQ API.
- [ ] You have the NetQ API URL, the username, and the password.

Make sure that you can reach the cluster:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

```text
<!-- TODO: capture output from a real environment -->
```

## Installation

First complete the [basic installation](./getting-started.md). It deploys the
Controller, the Agent, and the observability components that every scenario
shares. This guide adds only the NetQ credentials and the NVIDIA Topograph NetQ
provider configuration in the next section.

## Environment Configuration

Create the namespace and the NetQ credentials Secret first. The commands below
do not write the credentials to a local file, to Helm values, or to a
ConfigMap. The Secret must contain a key named `credentials.yaml`. Run the
commands on a host with `kubectl` access to the cluster:

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
<!-- TODO: capture output from a real environment -->
```

For a production cluster, enable encryption at rest for Kubernetes Secrets and
restrict RBAC access to this Secret.

Then select the NetQ provider. The command changes only the scenario-specific
values and keeps every other value from the basic installation:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
  --wait
```

```text
<!-- TODO: capture output from a real environment -->
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `topoDiscovery.scaleOut.mode` | Set it to `nv-topograph` to select NVIDIA Topograph as the scale-out label writer. |
| `nvidiaTopograph.provider.name` | Set it to `netq` to discover the topology through the NetQ API. |
| `nvidiaTopograph.provider.params.apiUrl` | The NetQ API URL. |
| `nvidiaTopograph.credentialsSecretName` | The name of an existing NetQ credentials Secret with a `credentials.yaml` key. The credentials are mounted read-only into the Topograph Pod only. |

## Verification

Inspect the topograph components, the NetQ provider configuration, and the Node
labels:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl -n unifabric-system describe secret netq-credentials
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
kubectl get topo
```

```text
<!-- TODO: capture output from a real environment -->
```

When you configure Kueue, Volcano, or KAI Scheduler, use only labels that are
written to Nodes.

Inspect the RDMA metrics resources:

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

```text
<!-- TODO: capture output from a real environment -->
```

Query the Agent metrics endpoint directly:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

```text
<!-- TODO: capture output from a real environment -->
```

Inspect the NIC classification in the RDMA metrics:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

```text
<!-- TODO: capture output from a real environment -->
```

## Troubleshooting

First rule out the general `FabricNode` and RDMA metrics problems with the
[basic installation troubleshooting](./getting-started.md#troubleshooting).

### Node Labels Are Not Written

1. Make sure that NVIDIA topograph and node-observer are running and have
   permission to update Nodes.
2. Make sure that `nvidiaTopograph.provider.name=netq`.
3. Make sure that `nvidiaTopograph.provider.params.apiUrl=<netq-url>` is
   correct.
4. Make sure that `nvidiaTopograph.credentialsSecretName` points to a Secret in
   the installation namespace and that the Secret contains a
   `credentials.yaml` key.
5. Make sure that the NetQ account can access the target premises and that NetQ
   contains the fabric topology data.
6. If `topoDiscovery.*.nodeLabel.keyTemplate` values are customized, update the
   scheduler label keys to match.

### RDMA Metrics Do Not Include RDMA NICs

1. Make sure that the Agent Pod is running and that RDMA devices are visible
   under `/sys/class/infiniband` on the node.
2. Make sure that `fabricNode.scaleOutInterfaceSelector` matches the RDMA NIC
   name or CIDR.
3. Make sure that `nodeMetrics.enabled=true`.
4. If you use Prometheus Operator, make sure that
   `nodeMetrics.serviceMonitor.enabled=true` and that the Prometheus selector
   selects the `ServiceMonitor`.

### Topograph Cannot Reach NetQ

1. Make sure that the NetQ API URL, the username and the password in the
   Secret, the certificates, and the network connectivity are correct.
2. Make sure that the Topograph Pod mounts the credentials at
   `/etc/topograph/credentials/credentials.yaml`.
3. Read the topograph and node-observer logs.
4. Make sure that the topograph Service is reachable.

## Uninstall

Remove the Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
<!-- TODO: capture output from a real environment -->
```

If you no longer need the CRDs, delete them. This also deletes all `FabricNode`,
`Switch`, and `Topology` resources:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
<!-- TODO: capture output from a real environment -->
```

## Next Steps

- Use the labels in a scheduler with the [Kueue TAS workload example](./usage/workload-tas.md).
- Render the topology with the [topology visualization guide](./usage/topology-visualization.md).
- Look up fields in the [FabricNode reference](./reference/fabricnode.md).
- Tune the installation with the [Helm values reference](../chart/README.md).
