# Template: Concepts / Architecture Document (concepts)

Common constraints and prose rules: see [.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

**Position**: the technical overview that CNCF sandbox requires. The readers are
evaluators, who decide whether the project fits their needs, and advanced users or
contributors, who want to understand how the system works. The document answers
"what is it, why does it exist, how does it work". It does not teach operations.

The core is one `docs/concepts/architecture.md`. As the project grows, you can
split independent concept pages out of it.

## Skeleton

```markdown
# <project name> Architecture

## Overview            <!-- project position + design goals: which problems it solves, which it does not -->
## Overall Architecture <!-- the one architecture diagram + one paragraph of responsibility per component (controller/agent/CRD...) -->
## Core Concepts       <!-- domain glossary: 2-4 sentences per concept + the relations between concepts -->
## Workflows           <!-- 2-3 key data flows (discover, report, aggregate, consume), flow diagram + text -->
## Use Cases           <!-- 3-5 typical use cases, one paragraph each -->
```

## Constraints

1. **No operational steps and no commands.** If `kubectl` appears, move that
   content to a usage document.
2. For each CRD or component, describe only its responsibility and its relations.
   Link to the reference document for field details.
3. The architecture diagram lives only here. The README links to it.
4. Define each term here once. All documents reuse the same English term and the
   same Chinese translation. A glossary table is allowed.
5. Deep discussion of design trade-offs, such as why the project chose one
   approach and which alternatives it rejected, belongs in a design document,
   not here.
