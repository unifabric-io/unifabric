# Create Switch Instances

中文版: [topograph-switch-resources.zh.md](./topograph-switch-resources.zh.md)

## Introduction

In the NVIDIA Spectrum-X and InfiniBand scenarios, NVIDIA Topograph produces the
Node labels and the `Topology` resource. Unifabric LLDP discovery in RoCE networks takes
no part, so the cluster has no `Switch` resources. In the `Topology` resource,
the `switchMember` list of each performance domain is empty. In topology
visualization, the switch member card of each domain is also empty. Scheduling
and the labels do not change.

If you want topology visualization to show the switches, create one label-only
`Switch` resource for each switch. A label-only `Switch` resource has no
`mgmtIP` and no tier, and it does not need switch-agent. The Controller reads
only its `unifabric.io/domain` label. It adds the switch to the matching
performance domain. This step is optional.

## Prerequisites

- You completed the configuration in [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md) or
  [InfiniBand network discovery](./infiniband-discovery.md), and
  `kubectl get nodes -L scale-out.unifabric.io/tier-1` shows topology labels.
- You know which performance domain each switch belongs to. For example, the NetQ
  output or the `ibnetdiscover` output shows which leaf domain contains `leaf1`.

## Configuration and Usage

1. List the domain names in the `Topology` resource:

   ```bash
   kubectl get topologies.unifabric.io scaleout -o jsonpath='{range .status.domains[*]}{.tier}{"\t"}{.name}{"\n"}{end}'
   ```

   ```text
   2	tier2-group1
   1	tier1-group1
   1	tier1-group2
   ```

2. Create one `Switch` resource for each switch. Set `unifabric.io/domain` to the
   domain name of the switch. Set `role` to the same value as the `Topology`
   resource, `ScaleOut` or `Storage`:

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

> Note: In the Topograph scenarios, only Topograph writes the Node labels. A
> label-only `Switch` resource takes no part in the topology computation. It
> does not change any Node label.

## Verification

```bash
kubectl get topologies.unifabric.io scaleout -o jsonpath='{range .status.domains[*]}{.name}{"\t"}{.switchMember}{"\n"}{end}'
```

```text
tier2-group1	["spine1"]
tier1-group1	["leaf1"]
tier1-group2	[]
```

When the switch name appears in `switchMember` of the matching domain, refresh
the Grafana topology dashboard. The switch member card then shows the switch.
See [Topology visualization](./topology-visualization.md).

## References

- [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md), [InfiniBand network discovery](./infiniband-discovery.md):
  the discovery methods that use this step.
- [Switch reference](../../reference/switch.md): the fields of the `Switch`
  resource.
- [Topology CRD design](../../design/topology-crd.md): the rules that fill
  `switchMember`.
