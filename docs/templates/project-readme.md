# Template: Project README

Common constraints and prose rules: see [.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

**Purpose**: the front page of the project, written for evaluators. The reader must
understand what the project is in 10 seconds. The reader must decide in 2 minutes
whether the project is worth a try. The README is a navigation entry, not the
documentation itself.

## Skeleton

```markdown
# <project name>                    <!-- project name + optional logo -->
[badges]                            <!-- license / go report / release / CI -->

> One-line positioning              <!-- what it is + what problem it solves, 30 words at most -->

## Introduction                     <!-- 2-3 paragraphs: pain point, how it solves it, where it fits -->
## Core Features             <!-- one subsection per major capability, 2-4 subsections;
                                  each is 1-3 sentences that cover what it does, the
                                  problem it solves, and the supported environments
                                  when it is environment-specific -->
## Architecture                     <!-- one overall diagram + 3-5 sentences, details link to concepts -->
## Quick Start                      <!-- only links to the getting started guides, no commands -->
## Documentation                    <!-- a table of links to each docs/ category -->
## Community and Contributing       <!-- contact channels, CONTRIBUTING link -->
## License
```

## Constraints

1. **Do not put any copy-and-run install command in the README.** Each command lives
   in one getting started guide only. Two copies of a command go stale in two places.
2. The body must stay under 150 lines. Content that is longer than 3 paragraphs must
   move to docs/ with a link.
3. Write each feature as what the user gets, not as an implementation term.
   Correct: "Automatic discovery of the RoCE network topology."
   Wrong: "CRD reporting based on an LLDP agent."
4. **Group the features by major capability.** Give each capability one subsection
   whose prose covers what it does, the problem it solves, and its supported
   environments. Do not write these dimensions as literal labels, and do not mix
   capabilities in one flat list. Do not expand second-level details, such as
   supported switch models or individual metrics. Leave them to the usage and
   reference documents.
5. The architecture diagram is the same image as in the concepts documents. Keep one
   source for it.
6. The English and Chinese versions (README.md and README-zh_cn.md) must align
   section by section.
