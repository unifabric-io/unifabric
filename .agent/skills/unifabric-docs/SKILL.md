---
name: unifabric-docs
description: |
  Write or rewrite unifabric documentation. Use when the user asks to create,
  rewrite, or review any document in this repository: README, concepts,
  getting started, usage, reference, or design. The skill selects the template
  for the document type, applies the common constraints, writes the English
  document in Simplified Technical English, and then translates it to Chinese.
---

# Unifabric Documentation Skill

This skill tells you how to write one unifabric document. Follow the four steps
in order.

## Workflow

1. Ask the user which document type they want. If the request already names the
   type, do not ask. The six types are in the table below.
2. Read the template for that type in `docs/templates/`. The template defines
   the required sections, the content of each section, and the constraints of
   that type. Follow it.
3. Write the English document first. Apply the common constraints below and the
   Simple English rules below to all prose.
4. Translate the English document to Chinese. Name the file `<name>.zh.md` and
   keep the section structure identical. The Chinese version of the root
   `README.md` is `README-zh_cn.md`.

## Document types

| Type | Template | Directory | Reader | Question it answers |
| --- | --- | --- | --- | --- |
| Project README | [project-readme.md](../../../docs/templates/project-readme.md) | repository root | evaluators | What is this? Is it worth a try? |
| Concepts | [concepts.md](../../../docs/templates/concepts.md) | `docs/concepts/` | evaluators, advanced users | How does it work? |
| Getting started | [getting-started.md](../../../docs/templates/getting-started.md) | `docs/` | new users | How do I install it and make it run? |
| Usage | [usage.md](../../../docs/templates/usage.md) | `docs/usage/` | installed users | How do I use this feature well? |
| Reference | [reference.md](../../../docs/templates/reference.md) | `docs/reference/` | users who look up fields | What does this field mean? |
| Design | [design.md](../../../docs/templates/design.md) | `docs/design/` | contributors, maintainers | Why is it designed this way? |

Each type has one responsibility. Do not mix them:

- Getting started and usage documents do not explain internals. They link to
  concepts or design documents.
- Concepts and design documents do not contain operational steps. They link to
  usage documents.
- Reference documents contain facts only, with no narrative. They link to usage
  documents for procedures.

## Common constraints

These constraints apply to every document type.

1. **Bilingual synchronization.** English is the primary document (`<name>.md`).
   Chinese is `<name>.zh.md` with the same section structure. When you change
   one side, you must change the other side.
2. **Every command shows its expected output.** Put a code block with the
   expected output, or a screenshot, directly after each command. The reader
   must be able to tell at each step whether they are on track.
3. **Verification output must be real.** Output in a verification section must
   come from a real environment. Do not write output from memory. Before you
   write, ask whether an environment is available. If yes, run each command and
   capture the output. If no, leave the placeholder
   `<!-- TODO: capture output from a real environment -->` in the English
   document and `<!-- TODO: 待真实环境采集输出 -->` in the Chinese document.
4. **Single source of truth.** Maintain each fact in one place and link to it
   from all other places. Install commands live in getting started. The
   architecture diagram lives in concepts. Field definitions come from the
   code.
5. **YAML examples are complete.** The reader can apply each example with
   `kubectl apply`. Do not replace required fields with `...`.
6. **One term, one translation.** Define each domain term once in a concepts
   document. Use the same English term and the same Chinese translation in
   every document.
7. **Multiple cases stay in one document.** When one feature or one environment
   has several valid procedures, keep them as sections of one document. Start
   the document with the criteria for choosing a case. Do not split the cases
   into separate documents.
8. **Conclusion first, no vague words.** Do not write "may", "probably", or
   "confirm this yourself". When the behavior depends on conditions, list the
   conditions and give a clear "supported" or "not supported" conclusion for
   each one. Write "Not supported when `NCCL_GIN_TYPE=3`", not "may not work
   with GIN, check the logs".
9. **Do not assume low-level knowledge.** When a low-level mechanism appears
   (for example `ibv_post_send`), either remove it or translate it to its
   relation with the conclusion in one phrase, for example "= CPU-initiated =
   supported".
10. **Omit information that does not support a conclusion.** Do not describe
    configuration that works by default and needs no user action.
11. **English prose follows the Simple English rules below.**

## Simple English rules

These rules come from ASD-STE100 Simplified Technical English
([AminBlg/SimpleEnglish](https://github.com/AminBlg/SimpleEnglish)). Apply them
to all English prose. Never change code, identifiers, commands, flags, file
paths, quoted errors, or product names.

1. Procedural text: imperative mood, 20 words per sentence at most, one
   instruction per sentence. Descriptive text: simple tenses, 25 words per
   sentence at most, one topic per paragraph.
2. Condition before command, with a comma: "If the Pod is not Ready, read the
   Agent log."
3. Active voice and simple tenses. No present perfect. Name the actor: "The
   Agent writes the result."
4. Modals: only can, will, must. Never should, would, may, might, could. A
   required "should" becomes "must". An optional one is deleted.
5. Complete grammar: no contractions, keep articles, keep "that". No
   semicolons and no em-dashes. Write two sentences, or name the relation.
6. One word, one meaning. Use "make sure that" for check, verify, confirm,
   validate, ensure. Use "configuration" for config, settings, options.
7. Define a concept term at its first use, in under ten words. Do not define
   product names or standard names.
8. State the fact, not its importance. Delete words that carry no fact:
   simply, seamlessly, robust, powerful, comprehensive, leverage, crucial.
9. Warnings: command or condition first, then the risk.
10. American spelling.

## Self-check before you deliver

1. The document follows the section skeleton of its template.
2. Every command has an expected-output block, real or TODO placeholder.
3. Search the English prose for `should`, `may`, `might`, `'` (contractions),
   `;`, and `—`. Fix each hit.
4. The Chinese document exists and its sections align with the English one.
5. All relative links resolve.
