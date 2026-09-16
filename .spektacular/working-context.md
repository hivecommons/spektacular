# Working context — plan 000052_document-status-vocabulary

## Carried from the spec discussion
- User decisions: rename field `status` -> `document_status`; values draft/final/superseded/archived; no backward compat or migration; unknown/old values read as blank (never an error); CLI input strict; flags renamed `--document-status`, command `set-status` -> `set-document-status`, list JSON key `document_status`; blank = open but distinct from draft; writes without flag preserve blank; closed_date kept.
- Out of scope: rewriting this repo's ~93 existing artifacts (user does separately); website only needs the example in `src/pages/projects.mdx` (docs repo root /home/nicj/code/github.com/jumppad-labs/spektacular-website).
- User prefers driving workflows straight through, stopping only for real design decisions.
- Implement workflow closes test-plan and changelog as well as plan docs (internal/steps/implement/steps.go:159,168).

## Plan walkthrough
- User approved plan 000052_document-status-vocabulary without changes (all drafting assumptions accepted).
- Spec renamed from 36-document-status-vocabulary to 000052_document-status-vocabulary: spec.id_method is `counter`, and plan writes require the counter ID prefix. Passing an external `id` to `spec new` under counter mode produced a non-conforming name.
