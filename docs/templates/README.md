# Documentation Templates

This directory holds one template for each unifabric document type. When you
create or rewrite a document, select the template for its type and follow its
skeleton and constraints. The taxonomy follows the CNCF TechDocs guidance for
sandbox projects (conceptual, task-based, and reference documentation).

The writing workflow, the common constraints for all types, and the English
prose rules live in the documentation skill:
[`.agent/skills/unifabric-docs/SKILL.md`](../../.agent/skills/unifabric-docs/SKILL.md).
Read the skill first, then the template for your type.

## Document types

| Type | Template | Directory | Reader | Question it answers |
| --- | --- | --- | --- | --- |
| Project README | [project-readme.md](./project-readme.md) | repository root | evaluators | What is this? Is it worth a try? |
| Concepts | [concepts.md](./concepts.md) | `docs/concepts/` | evaluators, advanced users | How does it work? |
| Getting started | [getting-started.md](./getting-started.md) | `docs/` | new users | How do I install it and make it run? |
| Usage | [usage.md](./usage.md) | `docs/usage/` | installed users | How do I use this feature well? |
| Reference | [reference.md](./reference.md) | `docs/reference/` | users who look up fields | What does this field mean? |
| Design | [design.md](./design.md) | `docs/design/` | contributors, maintainers | Why is it designed this way? |

Each type has one responsibility. Getting started and usage documents do not
explain internals. Concepts and design documents do not contain operational
steps. Reference documents contain facts only.

```
getting started (install) → usage (use each feature) → reference (look up fields)
                                            ↖ concepts / design (understand why)
```

The templates arrive one per pull request. A type without a merged template
does not have one yet.
