# Add Switch Resources for Topograph Scenarios

中文版: [topograph-switch-resources.zh.md](./topograph-switch-resources.zh.md)

## Introduction

In the NVIDIA Spectrum-X and InfiniBand scenarios, NVIDIA Topograph produces the
Node labels and the `Topology` resource. The LLDP discovery of Unifabric is not
involved, so the cluster has no `Switch` resources. In the `Topology` resource,
the `members` list of each performance domain is empty. In topology
visualization, the switch member card of each domain is also empty. Scheduling
and the labels are not affected.

If you want topology visualization to show the switches, create one label-only
`Switch` resource for each switch. A label-only `Switch` resource has no
`mgmtIP`, no tier, and no switch-agent. The Controller reads only its
`unifabric.io/domain` label and adds the switch to the matching performance
domain. This step is optional.

## Prerequisites

- Unifabric is deployed for the Spectrum-X or InfiniBand scenario according to the
  [basic installation](../../getting-started.md), and
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
   domain name of the switch. Set `role` to match the `Topology` resource,
   `ScaleOut` or `Storage`:

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

> **Note:** In the Topograph scenarios, only Topograph writes the Node labels. A
> label-only `Switch` resource takes no part in the topology computation and does
> not change any Node label.

## Verification

```bash
kubectl get topologies.unifabric.io scaleout -o jsonpath='{range .status.domains[*]}{.name}{"\t"}{.members}{"\n"}{end}'
```

```text
tier2-group1	["spine1"]
tier1-group1	["leaf1"]
tier1-group2	[]
```

When the switch name appears in `members` of the matching domain, refresh the
Grafana topology dashboard. The switch member card shows the switch. See
[Topology Visualization](./topology-visualization.md).

## References

- [Network Topology Discovery](./README.md): the NetQ and InfiniBand methods.
- [Switch reference](../../reference/switch.md): the fields of the `Switch`
  resource.
- [Topology CRD design](../../design/topology-crd.md): the rules that fill
  `switchMember`.
