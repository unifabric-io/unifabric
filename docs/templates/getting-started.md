# Template: getting started

Common constraints and prose rules: see [.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

**Position**: for new users with no unifabric knowledge. One document per
environment or hardware form (InfiniBand / Spectrum-X / SONiC RoCE / ...).
Goal: the reader goes from an empty cluster to "I can see unifabric work" in
30 minutes.

## Skeleton

```markdown
# Deploy unifabric on <environment>   <!-- The title has no "Getting Started:"
                                           prefix. The document location
                                           already shows the type. -->

## Introduction and Deployment Goals   <!-- 2-3 sentences: what this document
                                           completes, and what the reader sees
                                           at the end. When the document has
                                           multiple cases, give the criteria
                                           for choosing a case here. -->
## Prerequisites        <!-- A checklist: hardware (switch and NIC models),
                             Kubernetes version, dependent components,
                             permissions. The reader can check each item. -->
## Installation         <!-- helm install and similar steps. Each step shows
                             the command and the expected output. If a
                             scenario guide depends on the basic installation
                             guide, state "First complete the
                             [basic installation](...)" at the start and add
                             the link. -->
## Environment Configuration  <!-- Configuration specific to this environment,
                                   for example topology discovery. -->
## Verification         <!-- The smallest verifiable example: run
                             kubectl get fabricnodes and see the topology,
                             or open the visualization page and see the graph.
                             Attach real output or a screenshot. -->
## Troubleshooting      <!-- Common problems in the deployment stage of this
                             environment. -->
## Uninstall
## Next Steps           <!-- Links: usage documents for each feature,
                             reference documents. -->
```

## Constraints

1. **Each command is followed by its expected output.** The output must be a
   real capture. See common constraint 3 in the skill.
2. Do not assume any unifabric knowledge. When the document needs a concept,
   explain it in one sentence and link to the concepts document. Do not
   explain internals.
3. **Multiple deployment cases for one environment stay in one document.**
   Give the criteria for choosing a case at the start. Inside each case, the
   reader follows one straight line to the end. Do not branch again.
4. Each prerequisite must be concrete and checkable.
   (Correct: "NVIDIA Spectrum-X switch, SONiC >= 202305."
   Wrong: "a compatible switch.")
5. When a scenario guide depends on the basic installation guide, state the
   dependency at the start and add the link. Do not assume it implicitly.
6. Shared configuration lives in the basic installation guide. A scenario
   guide contains only the values that are specific to that scenario, and
   links to the basic installation guide for the rest. When a common step
   repeats in several documents, extract it into one linked location, so that
   the copies do not become stale.
7. Section names are identical across the whole series. All documents use
   "Introduction and Deployment Goals". Do not use "Deployment Goal" in one
   document and "Choose a Scenario" in another.
