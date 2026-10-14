# Template: feature usage guide (usage)

**Audience**: users who have installed unifabric. Write one document for each
top-level feature or scenario (topology visualization, RDMA metrics, workload
TAS, switch-agent deployment, and so on). The main path is the best practice:
write the recommended procedure directly and do not write second-best
procedures.

Common constraints and prose rules: see
[.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

## Skeleton

```markdown
# <feature name>

## Introduction         <!-- The problem it solves and when to use it, in 2-3
                             sentences. The first paragraph must state the
                             capability boundary or key limitation (for
                             example "supports host-initiated RDMA only"). -->
## Applicability        <!-- Conditionally required: write this section ONLY
                             when a real boundary exists, that is, when some
                             scenario is not supported (for example rdma-flow
                             supports only CPU-initiated RDMA). This matters
                             most for features whose support differs across
                             scenarios. When every scenario is supported, omit
                             this section, state behavior differences in
                             inline Notes, and put required components in
                             Prerequisites. Quantify with a table:
                             library/component | version range | key
                             environment variables and values | supported
                             (yes/no) | evidence (official URL).
                             Record the version of each library that you
                             analyzed, so that the conclusion is traceable. -->
## Prerequisites        <!-- unifabric is installed, plus the dependencies
                             that this feature needs. -->
## Configuration and usage
                        <!-- Write the recommended procedure (the best
                             practice) directly: steps + commands/YAML + real
                             output. Put production cautions in Note/Warning
                             blocks inline, next to the related step, not in a
                             separate section. When the feature has several
                             cases, keep them as sections of this document and
                             start with the criteria for choosing a case. -->
## Verification         <!-- End to end, from the user's point of view: for
                             example "open the Grafana dashboard and see
                             data" (with a real screenshot). A check of an
                             intermediate store is a troubleshooting step,
                             not a verification. -->
## Troubleshooting      <!-- (Optional) First rule out the preconditions (for
                             example: does the application start RDMA from
                             the CPU, did it produce traffic at all), then
                             examine the components. -->
## References           <!-- Link to reference (fields/metrics) and design. -->
```

## Constraints

1. One document covers **one top-level feature**. Keep the different usages or
   cases of that feature as sections of the same document. Do not split them
   into separate documents.
2. Do not repeat the install steps. Link to getting started at the top.
3. **The main path is the best practice.** Do not write a plain procedure
   first and attach a "best practices" section. Put production advice inline,
   next to the related step.
4. Explain the mechanism in one paragraph at most. When you need more, link to
   concepts or design. Do not list the full field set. Link to reference.
5. **Applicability gives conclusions, not derivations.** State "supported" or
   "not supported" for each scenario. Do not write vague phrases such as
   "possibly" or "confirm with the logs". Translate each low-level term into a
   conclusion in one phrase (for example "a normal NCCL collective =
   CPU-initiated = supported").
6. **Compatibility conclusions are traceable.** Record the library version
   that you analyzed (for example NCCL 2.28.x) and link to the official
   documentation or code. A compatibility conclusion without a version decays
   over time and counts as invalid content.
7. **Verification is end to end.** Judge by the final effect that the user can
   see (the dashboard has data, the CR has status). Do not substitute a check
   of an internal store or middleware.
