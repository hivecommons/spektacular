---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Test Plan: 000069_compact-agents-md

The automated tests cover the following deterministic parts:

- the byte budget, line limits and link resolution;
- conversion of a pre-change project, with user content preserved;
- idempotency and guidance refresh;
- the omp rule files;
- that no guidance file is imported eagerly;
- the phrase needles on both surfaces.

The automated tests live in:

- `internal/agent/standing_rules_test.go`
- `cmd/standing_rules_test.go`
- the per-rule `internal/agent/*_test.go`

The procedures below cover what only a person or a live agent can check. Run them from the `spektacular` repository at the branch `spek/000069_compact-agents-md`, or at `main` after the merge. Never run `spektacular init` or `migrate` in the Spektacular repository itself: every procedure uses a throwaway project.

Two commits mark the "before" and "after" of the change:

- **Before:** `d8bad79` (Milestone 1). It has the standing-rules harbor suite and the Bob harbor agent move, but still installs the full-text rules.
- **After:** `07d004e` (Milestone 2) or later. It installs the compact rules and the guidance files.

Build each into its own checkout. For example, use `git worktree add /tmp/spek-before d8bad79` and `/tmp/spek-after` at the later commit, and run the make targets from each.

## Metric 1: tokens spent on standing instructions fall by at least 75%

- **What to measure:** the context a fresh Claude Code session spends on project instructions (CLAUDE.md plus the imported AGENTS.md) in a newly initialised project, before and after. The threshold is an after/before ratio of 0.25 or less. The byte-budget test already pins 4,254 bytes against 21,722 (19.6%); this confirms it in tokens.
- **How:**
  1. Build both binaries: `go build -o /tmp/spek-before/bin/spektacular .` in the before checkout, and `go build -o /tmp/spek-after/bin/spektacular .` in the after checkout.
  2. Run `mkdir /tmp/ctx-before && cd /tmp/ctx-before && git init -q && /tmp/spek-before/bin/spektacular init claude`. Repeat with `/tmp/ctx-after` and the after binary.
  3. In each directory, start `claude`. Before sending any message, run `/context` and note the tokens listed for memory files (`CLAUDE.md` / `AGENTS.md`).
- **Expected result:** the AGENTS.md (memory file) tokens after are at most 25% of before. Expect about 20%.
- **Who / when:** the maintainer, once before release, with the current Claude Code version.

## Metric 2: no end-to-end suite's pass rate drops for any agent

- **What to measure:** the pass rate (verifier reward 1) of every harbor suite, for each harbor-supported agent, before and after the change. The threshold is that no after rate is lower than its before rate. Run each suite/agent pair at least 3 times on each side, because agent runs vary.
- **How:** in each checkout, with harbor, Docker and agent credentials set up as the README's harbor section describes (`claude` login, `codex` auth, and a `bob` login or `BOBSHELL_API_KEY`):
  - spec: `make harbor-test-spec-claude`, `make harbor-test-spec-codex`
  - plan: `make harbor-test-plan-claude`, `make harbor-test-plan-codex`, `make harbor-test-plan-bob`
  - implement: `make harbor-test-implement` (claude only)
  - repo: `make harbor-test-repo`, `make harbor-test-repo-delegated` (claude only)
  - epic: `make harbor-test-epic-claude`, `make harbor-test-epic-codex`, `make harbor-test-epic-bob`
  - implement-epic: `make harbor-test-implement-epic-claude`, `make harbor-test-implement-epic-codex`, `make harbor-test-implement-epic-bob`
  - standing-rules (new): `make harbor-test-standing-rules-claude`, `make harbor-test-standing-rules-codex`, `make harbor-test-standing-rules-bob`

  Read each reward from `tests/harbor/jobs/*/<suite>-<agent>__*/verifier/reward.txt` (the target also prints the verifier output). Record a before/after table: suite, agent, passes/runs.
- **Expected result:** for every suite and agent, the after pass rate is at least the before pass rate. The standing-rules suite in particular passes on both sides; its verifier checks `repo list`, the answer 37, `spec file read` with the spec unchanged, no `spec new`, and an offer of a spec.

  If an agent breaks a rule after the change that it honoured before, follow the plan's Open Question. Strengthen that rule's short statement in `templates/agents/short/<slug>.md`, within the budget, and re-run. Never move binding text back into the guidance file alone, and never raise the budget unasked.
- **Who / when:** the maintainer, before release. Bob runs need the Bob harbor agent at `tests/harbor/agents/`, which is now in place.

## Review 1: Copilot and omp sessions follow the compact rules

- **What to look at:** a live Copilot CLI session and a live omp session in a freshly initialised throwaway project with a registered repo outside it.
- **How:**
  1. Build the after binary.
  2. Run `mkdir -p /tmp/sr/{proj,inventory}`. Copy `tests/harbor/standing-rules/environment/inventory/stock.py` into `/tmp/sr/inventory`, then `git init` and commit there.
  3. In `/tmp/sr/proj`, run `spektacular init copilot`, then `spektacular repo add --data '{"name":"inventory","location":"/tmp/sr/inventory","description":"stock-keeping service","role":"service","tags":["python"]}'`.
  4. Seed the spec from `tests/harbor/standing-rules/environment/seed/spec.md` through `.spektacular/tmp/` and `spektacular spec file write 20260101000001-low-stock-alerts --document-status final --from <staged>`.
  5. Start `copilot` with `SPEKTACULAR_SESSION_LOG=on` and ask the three requests from `tests/harbor/standing-rules/instruction.md`.
  6. Repeat in a fresh copy with `spektacular init omp` and `omp`. For omp, also run `python3 tests/omp_smoke.py <abs path to built spektacular>` to confirm each `.omp/rules/spek-*.md` body appears once in omp's system prompt.
- **Passing:**
  - The session log under `.spektacular/debug/` shows `repo list` before any code read.
  - The code answer gives 37 from `/srv`/`/tmp/sr/inventory`.
  - The spec is read only through `spec file read`.
  - No `spec new` is run, and the feature answer offers to capture a spec.
  - omp shows no guidance-file text in its prompt.
  - If omp shows each compact rule twice, record it here and carry on (Open Question 2). Stop only if guidance content is loaded eagerly.

## Review 2: rule-by-rule read of each compact section beside its guidance

- **What to look at:** for each of the eight rules, `templates/agents/short/<slug>.md` beside `templates/agents/<slug>.md`, or a throwaway install's `AGENTS.md` beside `.spektacular/guidance/<slug>.md`.
- **What to look for:**
  - The obligation that always applies is stated in the short section itself. Examples: run `repo list` before code work; never touch store files with your own tools; offer, never start, a spec, design or knowledge write.
  - The trigger for reading further is clear.
  - Every instruction in the old full text is still in the guidance file. Compare the guidance against `git show d8bad79:templates/agents/<slug>.md`: only the `#` title, the banner and the store-access exemption paragraph may differ.
- **Passing:** all eight rules satisfy all three points. Two known wording leftovers in the guidance may be noted but do not fail the review:
  - knowledge-trigger's "per the Memory & Context section above";
  - store-access's "A sub-agent inherits this file".

## Review 3: guidance is not loaded eagerly in a live Claude session

- **What to look at:** a fresh Claude Code session in `/tmp/ctx-after` (from Metric 1).
- **How:**
  1. Run `/context` and `/memory` before any message. Confirm only `CLAUDE.md` and `AGENTS.md` are listed as memory files.
  2. Ask "what does your store-access guidance file say about removal?". Confirm the agent then opens `.spektacular/guidance/store-access.md` with its own read tool.
- **Passing:** no `.spektacular/guidance/*.md` content or file appears in the context before the agent opens one. The agent reads it directly when asked, and does not refuse because of the store-access rule.

## Review 4: documentation site

- **What to look at:** the docs site built from the `docs` repo (`npm ci && npm run dev`, then open `/configuration/`, `/plugins/` and the getting-started tutorial).
- **What to look for:**
  - The new "Standing rules: AGENTS.md and .spektacular/guidance/" section explains:
    - the short statements;
    - the eight-file guidance tree;
    - that guidance is read on demand, should be committed, and should not be edited;
    - how each agent reads `AGENTS.md`.
  - The migrate section's "One run:" list says it converts the standing rules and refreshes the guidance, leaving the team's own content.
  - The omp text in plugins and getting-started says the rules mirror the short sections.
  - Section backgrounds still alternate.
  - No em dashes in the new text.
- **Passing:** all of the above. `npm run build` and `npx astro check` report 0 errors, and `grep -nE "<div|<section|class=" src/pages/*.mdx` prints nothing.
