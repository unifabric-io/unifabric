# Template: Reference

Common constraints and prose rules: see [.agent/skills/unifabric-docs/SKILL.md](../../.agent/skills/unifabric-docs/SKILL.md).

**Position**: lookup only, for users who look up fields, parameters, or
metrics. One CRD, component, CLI, or metric set per document. No narrative and
no tutorial content.

## Skeleton (CRD)

```markdown
# <CRD name, for example FabricNode>

## Overview             <!-- 1-2 sentences on the responsibility; link to concepts -->
## Example YAML         <!-- one complete, commented example -->
## Field Definitions
### Metadata            <!-- conventional labels/annotations, if any -->
### Spec                <!-- field table: Field | Type | Required | Default | Description -->
### Status              <!-- same table; enumerate conditions and the trigger of each value -->
```

## Skeleton (other subtypes)

- **Metrics reference**: one table with Metric | Type | Labels | Meaning.
- **CLI or component reference**: Flags | Environment variables | Default | Description.
- **Annotation or label reference**: one document that collects all items in the project.

## Constraints

1. **Tables are the body of the document.** Do not write paragraph
   explanations. Each field description has 2 sentences at most.
2. **Stay in sync with the code.** The Go type definitions
   (`pkg/.../types.go`) and the generated CRD manifests are the single source
   of truth. Check every field against them. Do not copy from existing
   documents.
3. **Cover all fields and metrics.** Do not select only the common ones.
   Completeness is the purpose of a reference document.
4. Do not write operational steps. For "how to use", link to the usage
   document.
5. **Enumerate all values of each enumeration** (for example condition
   reasons) and state the trigger of each value.
