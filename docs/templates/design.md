# Template: design document

Common constraints and prose rules: see [.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

**Purpose**: a design document is for contributors and maintainers. It records
why the feature is designed this way. Write one document per feature and put it
in `docs/design/`. The format is a reduced form of a Kubernetes Enhancement
Proposal (KEP).

## Skeleton

```markdown
# <feature name> design

## Summary              <!-- 3 to 5 sentences: what it does and why -->
## Motivation           <!-- problem background, current pain points,
                             user stories (As a ..., I want ...) -->
### Goals               <!-- bullets, measurable -->
### Non-Goals           <!-- bullets, what the design does not do,
                             to stop scope creep -->
## Proposal             <!-- high-level design: architecture or flow
                             diagram, component responsibilities,
                             data flow -->
## Detailed Design      <!-- CRD and API definitions, key algorithms,
                             component interaction sequence,
                             security and permissions,
                             upgrade compatibility -->
## Risks and Mitigations <!-- optional -->
## Alternatives         <!-- what was considered and why it was not
                             chosen, at least one paragraph -->
## Implementation History <!-- date + milestone: proposed, reviewed,
                               implemented, released version -->
```

## Constraints of this type

1. Write the document before or during implementation. Record the reasoning at
   the time of the decision. When the code later diverges from the design, note
   the evolution in "Implementation History". Do not rewrite the proposal to
   pretend foresight.
2. When you reconstruct a design document or a section after the
   implementation, mark it as a post-hoc reconstruction in
   "Implementation History".
3. "Non-Goals" and "Alternatives" are mandatory. These sections are the core
   value that separates a design document from a usage document.
4. Take the dates in "Implementation History" from the git history. Do not
   invent dates.
5. CRD fields in a design document show the design draft. The source of truth
   is the reference document and the code. A design document does not promise
   to stay in sync with the latest code.
6. Do not include user steps. Link to the usage document for how to use the
   feature.
7. Align the file name with the feature, for example `topology-crd.md`. The
   same name as a usage document is acceptable because the two types have
   different responsibilities.
