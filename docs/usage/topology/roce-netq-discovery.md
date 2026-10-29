# NVIDIA NetQ Discovery in RoCE Networks

中文版: [roce-netq-discovery.zh.md](./roce-netq-discovery.zh.md)

## Introduction

Spectrum-X is the NVIDIA Ethernet RoCE network platform for AI clusters. NVIDIA
NetQ manages its fabric and already knows the full topology. NetQ also contains
the NVLink management function (previously NMX-M). This function can supply the
NVLink domains of a multi-node NVLink system.

Unifabric imports the topology from NetQ through NVIDIA Topograph. Topograph is
an NVIDIA open-source service. It reads the network topology from vendor data
sources and writes Node labels. This document tells you how to configure NVIDIA NetQ
discovery in RoCE networks.

| Item | Description |
| --- | --- |
| Applicable networks | Scale-out, Storage, Scale-up |
| Topology source | Topograph reads the fabric topology and the NVLink domains through the NetQ API. |
| Discovered tiers | Scale-out: the fabric data in NetQ sets the tiers. Usually these are leaf, spine, and all tiers above them. Scale-up: tier 1, that is, the NVLink domain. |
| Switch side requirements | NetQ manages the fabric, and NetQ has the topology data for it. |
| Cluster side requirements | The node that runs the Topograph Pod can connect to the NetQ API. You have the NetQ credentials. |
| Helm configuration | `topoDiscovery.scaleOut.mode=nv-topograph`, `nvidiaTopograph.provider.name=netq`, and the NetQ credentials. For the Scale-up network, add `topoDiscovery.scaleUp.mode=nv-topograph`. |

Limits:

- This method does not create `Switch` resources. The `Topology` resources and
  the Node labels are complete. But in topology visualization, the switch member
  card of each performance domain is empty. See
  [Optional: add Switch resources](#optional-add-switch-resources).
- Unifabric does not yet separate the Scale-out network and the Storage network
  in the topology that it imports from NetQ. The import result is used only as
  the Scale-out network. If the Storage network runs on Spectrum switches, use
  [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) or [Custom network topology](./custom-network-topology.md)
  for it.

For the discovery method to use for each network on each network device, see
[Network topology discovery](./README.md#choose-a-discovery-method-by-network).

## Prerequisites

- You completed the deployment as described in
  [Deploy Unifabric](../../getting-started.md).
- NetQ manages the Spectrum-X fabric, and NetQ has the fabric topology data for it.
- The cluster can connect to the NetQ API. Make sure that the firewall between
  the cluster and NetQ allows this path.
- You have the NetQ API URL, the username, and the password.

## Configuration

1. Create the namespace and the NetQ credentials Secret. The commands below do
   not write the credentials to local files, Helm values, or ConfigMaps. The
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

   On a production cluster, also enable encryption at rest for Kubernetes
   Secrets. Also restrict the RBAC permissions that can read this Secret.

2. Set the topology discovery parameters:

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

   If the cluster is a multi-node NVLink system and NetQ manages the NVLink
   domains, also add `--set topoDiscovery.scaleUp.mode=nv-topograph`. Unifabric
   then also discovers the Scale-up network.

   Parameter description:

   | Helm value | Purpose |
   | --- | --- |
   | `topoDiscovery.scaleOut.mode` | Set to `nv-topograph`. NVIDIA Topograph then writes the scale-out labels. |
   | `topoDiscovery.scaleUp.mode` | Optional. Set to `nv-topograph`. NVIDIA Topograph then writes the scale-up labels. |
   | `nvidiaTopograph.provider.name` | Set to `netq`. Topograph then discovers the topology through the NetQ API. |
   | `nvidiaTopograph.provider.params.apiUrl` | The NetQ API URL. |
   | `nvidiaTopograph.credentialsSecretName` | The name of the existing NetQ credentials Secret. The Secret must contain the `credentials.yaml` key. Topograph mounts the credentials read-only into the Topograph Pod. |

## Verification

Make sure that the scale-out labels are on the Nodes:

```bash
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

If you configured the Scale-up network, make sure that the scale-up labels are
on the Nodes:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1
```

For the meaning of the labels and for how to view the `Topology` resources, see
[Network topology discovery](./README.md#view-topology-and-node-labels).

## Optional: add Switch resources

This method does not create `Switch` resources. If you want topology
visualization to show the switches, see
[Create Switch instances](./topograph-switch-resources.md).

## Troubleshooting

### Nodes have no topology labels

1. Make sure that the Topograph Deployment runs, and read its logs:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

2. Make sure that the NetQ credentials and the API address are correct.
   Make sure that the Topograph Pod can connect to the NetQ API.

## References

- [Network topology discovery](./README.md): how to choose a discovery method
  for each network, and how to view the results.
- [InfiniBand network discovery](./infiniband-discovery.md): a different discovery
  method that also uses NVIDIA Topograph.
- [Create Switch instances](./topograph-switch-resources.md): an
  optional step.
- [Topograph NetQ provider](https://github.com/dsx-ai-factory/topograph/blob/main/docs/providers/netq.md):
  the upstream document.
