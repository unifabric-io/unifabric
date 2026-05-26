# General SONiC RoCE

中文版: [getting-started-sonic-roce.zh.md](./getting-started-sonic-roce.zh.md)

This guide explains how to deploy Unifabric in a cluster where SONiC switches carry the RoCE network. In this scenario, Unifabric discovers scale-out leaf, spine, and core topology from node RDMA NICs, `FabricNode` LLDP neighbor information, and switch-side switch-agent LLDP snapshots.

## Deployment Goals

After deployment, the cluster should achieve the following goals:

- Nodes are labeled with topology labels consumed by schedulers. By default these include
  `unifabric.io/scale-out-leaf`, `unifabric.io/scale-out-spine`, and
  `unifabric.io/scale-out-core`.
- Node RDMA state is observable through Unifabric Agent metrics, including RDMA device, port,
  priority, and Pod attribution metrics.
- `FabricNode` and `Switch` CRs expose the input state, and schedulers consume the topology labels written to Nodes.

> With the default hash naming mode, Node label values include
> `leaf-`, `spine-`, and `core-` prefixes, for example `leaf-0a42746`.

## Prerequisites

- An accessible Kubernetes cluster.
- `kubectl` and Helm 3 installed.
- Switches have Docker or another container runtime. If a switch cannot run
  containers, use the [switch-agent systemd installation](./usage/switch-agent-systemd.md).

## Install Unifabric in the Cluster

The following commands use the latest release version. You can also specify a fixed version. See the [releases](https://github.com/unifabric-io/unifabric/releases) page for available versions.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"
```

Then run the installation command.

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set switchTopologyDiscovery.enabled=true \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `switchTopologyDiscovery.enabled` | Enables topology discovery based on switch neighbors. |
| `switchTopologyDiscovery.ignoreSwitchPorts` | Optional: local switch ports ignored by the controller before topology calculation. Defaults to `mgmt*`, `Management*`, and `oob*`. |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | Optional: restricts RDMA NICs that participate in scale-out topology discovery. When unset, all RDMA NICs not matched by storage / scale-up selectors participate. |
| `nodeTopologyDiscovery.storageInterfaceSelector` | Optional: selects storage RDMA NICs and excludes them from scale-out topology. Metrics are labeled `kind=storage`. Supports `interface=eth9` or `cidr=172.20.0.0/16`. |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | Optional: selects scale-up RDMA NICs and excludes them from scale-out leaf grouping. Metrics are labeled `kind=scaleUp`. |
| `nodeMetrics.enabled` | Optional: enables Agent metrics for node RDMA observability. |
| `nodeMetrics.serviceMonitor.enabled` | Optional: creates the `ServiceMonitor` used by Prometheus Operator. |
| `grafanaDashboard.enabled` | Optional: deploys the built-in RDMA dashboard. |
| `topologyLabels.scaleOutLeaf` | Leaf Node label key. Default: `unifabric.io/scale-out-leaf`. |
| `topologyLabels.scaleOutSpine` | Spine Node label key. Default: `unifabric.io/scale-out-spine`. |
| `topologyLabels.scaleOutCore` | Core Node label key. Default: `unifabric.io/scale-out-core`. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

If you are in mainland China, you can add the following parameters to speed up image pulls:

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set agent.lldp.image.repository=ghcr.io/unifabric-io/unifabric-agent \
```

## Install switch-agent and Switch Resources on Switches

The purpose of deploying the switch-side `switch-agent` is to collect local LLDP neighbor relationships from each switch and synchronize them to the in-cluster Unifabric controller, which improves scale-out topology discovery.

Without `switch-agent`, Unifabric can still identify the leaf switch connected to each node from node-side `FabricNode` LLDP neighbor information. However, leaf-to-spine and leaf-to-core uplinks are only visible from the switch side, so spine and core layers cannot be identified reliably. In larger multi-switch clusters, relying only on node-side LLDP can produce incomplete or less accurate topology groups. For a small cluster with a single switch or a single leaf layer, leaf discovery is usually sufficient.

After `switch-agent` is deployed, switch-side LLDP snapshots complete the uplink relationships between leaf, spine, and core switches. The controller uses those relationships to identify the full scale-out network layers and write topology labels back to Kubernetes Nodes.

The Helm installation only deploys the controller and node agent inside Kubernetes. It does not automatically install `switch-agent` on switches. Every leaf, spine, and core switch that should participate in scale-out topology calculation needs a separate `switch-agent` instance and a corresponding `Switch` resource in the cluster.

Before running it, confirm the following:

- The switch management network is reachable from the in-cluster Unifabric controller.
- The switch can run the `switch-agent` container and can pull or pre-load the corresponding image.
  If Docker is not available, install the release binary with
  [systemd](./usage/switch-agent-systemd.md) instead.
- LLDP is enabled on the switch, and `lldpcli show neighbors -f json0` on the switch host shows the expected neighbors.
  - `socket` mode only requires the container to mount and access `/run/lldpd.socket`.
  - If the switch cannot mount or access `/run/lldpd.socket`, use `hostProc` mode instead. That mode requires privileged permissions and mounts the host `/proc`.

Pay attention to the following impact:

- `switch-agent` exposes a gRPC port on the switch management network through container port mapping. The default port is `8090`, and pinned mTLS is enabled by default.

The actual onboarding steps are below.

### Export switch-agent pinned mTLS certificates

Run the following certificate export commands on the Kubernetes control node, or on another management host that already has `kubectl` access to the cluster.

```bash
mkdir -p ./tmp-switch-mtls

kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.crt}' | base64 -d > ./tmp-switch-mtls/tls.crt
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.key}' | base64 -d > ./tmp-switch-mtls/tls.key
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.peer\.crt}' | base64 -d > ./tmp-switch-mtls/peer.crt
```

After exporting the files, log in to the target switch and prepare the directory there:

```bash
sudo mkdir -p /opt/unifabric-switch-agent/mtls
```

Then copy `tls.crt`, `tls.key`, and `peer.crt` to `/opt/unifabric-switch-agent/mtls/` on the switch.

### Start switch-agent on the switch

By default, switch-agent reads LLDP through the mounted `lldpd` socket and uses the packaged `lldpcli` `1.0.16`. The following command uses Docker bridge networking and publishes the gRPC port to the switch management IP through `-p 8090:8090`. It does not require host network, host UTS, or privileged permissions.

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:${LATEST_TAG}"
export SWITCH_NAME="$(hostname)"

docker pull "${SWITCH_AGENT_IMAGE}"

docker rm -f unifabric-switch-agent 2>/dev/null || true

docker run -d \
  --name unifabric-switch-agent \
  --restart unless-stopped \
  -p 8090:8090 \
  -e UNIFABRIC_SWITCH_AGENT_SWITCH_NAME="${SWITCH_NAME}" \
  -v /run/lldpd.socket:/run/lldpd.socket \
  -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
  "${SWITCH_AGENT_IMAGE}" \
  /usr/bin/unifabric/switch-agent
```

Common environment variables:

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `UNIFABRIC_SWITCH_AGENT_SWITCH_NAME` | `$hostname` | Local switch name reported in LLDP snapshots. |
| `UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS` | `:8090` | gRPC listen address exposed by switch-agent. |
| `UNIFABRIC_SWITCH_AGENT_LLDP_REFRESH_INTERVAL` | `10s` | Local LLDP snapshot refresh interval. |
| `UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE` | `socket` | LLDP collection mode. The default is `socket`. If the switch cannot mount the `lldpd` socket, use the host `/proc` namespace collection mode instead. See [switch-agent hostProc LLDP collection](./usage/switch-agent-host-proc.md). |
| `UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION` | `1.0.16` | Packaged CLI version used in `socket` mode. Use `1.0.4` for SONiC 202006 through 202311. Keep the default `1.0.16` for other versions. |

After startup, first check whether the container is running and whether the logs contain errors:

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

### Create Switch resources

After switch-agent is running on the switches, create one `Switch` YAML for each switch. The Unifabric Controller connects by `spec.mgmtIP` to read LLDP information.

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf1
spec:
  mgmtIP: <leaf1-mgmt-ip>
  role: ScaleOut
  grpcPort: 8090
```

Fields to care about in `spec`:

- `mgmtIP`: required. Management address used by the controller to connect to switch-agent.
- `role`: optional. Identifies whether the switch belongs to the scale-out, scale-up, or storage network. Supported values are `ScaleOut`, `ScaleUp`, and `Storage`. Defaults to `ScaleOut` when omitted.
- `grpcPort`: optional. gRPC port of switch-agent. Defaults to `8090`.

You can choose `role` based on the NICs connected to the switch and the purpose of the network:

- `ScaleOut`: the switch connects to host-side RDMA NICs used for cross-node GPU training or service traffic. These NICs participate in scale-out leaf / spine / core topology calculation.
- `ScaleUp`: the switch connects to GPU-side scale-up NICs used for GPU-to-GPU communication within the same scale-up domain. These NICs do not participate in scale-out topology calculation.
- `Storage`: the switch connects to host-side NICs used for storage access. These NICs should correspond to the storage NICs selected by `nodeTopologyDiscovery.storageInterfaceSelector`.

After the YAML is ready, run `kubectl apply -f <switch>.yaml` on the Kubernetes control node to create the switch CR.

Then use `kubectl get switch` to check whether neighbor information has been synchronized. The deployment verification section below includes example output.

## Verify the Deployment

Wait for the controller and agent to be ready:

```bash
kubectl -n unifabric-system get pods
kubectl -n unifabric-system rollout status deployment/unifabric-controller
kubectl -n unifabric-system rollout status daemonset/unifabric-agent
```

Inspect `FabricNode`:

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

Check:

- `status.scaleOutNics` contains the expected scale-out RDMA NICs.
- `status.storageNics` contains only storage network interfaces.
- `status.scaleOutNics[*].lldpNeighbor.hostname` is present.
- `Ready` and `LLDPNeighborsReady` in `status.conditions` are `True`.

Inspect switch status and Node labels:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
kubectl get nodes -L unifabric.io/scale-out-leaf,unifabric.io/scale-out-spine,unifabric.io/scale-out-core,kubernetes.io/hostname
```

For example, in the current test environment you may see output similar to:

```bash
$ kubectl get switch
NAME     MGMTIP            ROLE       HEALTHY   NEIGHBORS
leaf1    192.168.122.72    ScaleOut   true      2
leaf2    192.168.122.80    ScaleOut   true      2
spine1   192.168.122.163   ScaleOut   true      2
```

Check:

- `Switch.status.healthy` is `true`.
- `Switch.status.lldpNeighborCount` is greater than `0`.
- Nodes have `unifabric.io/scale-out-leaf`, `unifabric.io/scale-out-spine`, and `unifabric.io/scale-out-core` labels.

When configuring Kueue, Volcano, or KAI Scheduler, use only labels that are actually written to Nodes by the command above. If the current network topology has only a leaf layer and no upper-layer switches, empty spine/core labels are expected.

Verify RDMA metrics resources:

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

Check the Agent metrics endpoint directly:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

Check RDMA metric NIC classification:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

## Troubleshooting

### `FabricNode` Has No Scale-Out NIC

- Confirm that RDMA devices are visible under `/sys/class/infiniband` on the node.
- If `nodeTopologyDiscovery.scaleOutInterfaceSelector` is explicitly configured, confirm that it matches the real interface name or CIDR.
- Confirm that scale-out interfaces were not accidentally matched as storage or scale-up. Those interfaces are excluded from scale-out grouping.

### `LLDPNeighborsReady=False`

- Confirm that LLDP is enabled on switches.
- Confirm that node-side `lldpd` works and the Agent Pod can read LLDP information.
- Confirm that `nodeTopologyDiscovery.initialScanDelay` is long enough, so the first Agent scan does not run before LLDP learning completes.

### No Node Label

- Confirm that `switchTopologyDiscovery.enabled=true`.
- Confirm that `FabricNode.status.nodeRole` is not `Storage`.
- Confirm that at least one `scaleOutNics` entry has both `state=up` and `lldpNeighbor.hostname`.
- Confirm that `Switch.status.healthy=true` and that `Switch.status.lldpNeighbors` already contains neighbor data.
- Check controller logs:

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  ```

- Check switch-agent logs on the switch:

  ```bash
  docker logs --tail 100 unifabric-switch-agent
  ```

### RDMA Metrics Do Not Include RoCE NICs

- Confirm that the Agent Pod is running and RDMA devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and that the `ServiceMonitor` is selected by the Prometheus selector.
- Query the Agent metrics endpoint directly first. If `unifabric_` metrics exist there, troubleshoot Prometheus target discovery next.

## Uninstall

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

If CRDs are no longer needed, delete them manually:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io
```

## Next Steps

- Return to the [documentation index](./README.md).
- Read the [Kueue TAS workload example](./usage/workload-tas.md).
- See the [Helm values reference](../chart/README.md).
