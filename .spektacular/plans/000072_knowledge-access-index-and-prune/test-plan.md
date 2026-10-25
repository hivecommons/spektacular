---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test Plan: 000072_knowledge-access-index-and-prune

These procedures cover the success metrics and reviews the plan's Testing Approach marks as manual. Everything else (exact discovery and retrieval counts, parallel updates, the age rule, verdict claim checks, the byte-identical dry run, prune's untracked reads, the template wording) is covered by the Go test suite (`go test ./...`).

Run every procedure with a binary built from the merged branch (`make build`, then `./bin/spektacular` or an installed copy), between workflows, never during one. Where a procedure uses this repository's own knowledge base, run it from the project root after reinstalling the binary.

## 1. Zero "should I save this?" questions in the end-to-end suites

- **What to measure**: knowledge-save questions asked of the user across a full run of the plan, implement, epic, implement-epic and goal-check Harbor suites. Threshold: 0.
- **How**: from the spektacular repo root, `make harbor-install` once, then run `make harbor-test-plan`, `make harbor-test-implement`, `make harbor-test-epic`, `make harbor-test-implement-epic` and `make harbor-test-goal-check` (Claude by default; repeat with the `-codex`/`-bob` variants if those agents are in scope). For each job under `tests/harbor/jobs/`, search the agent transcript for a question about saving knowledge, for example `grep -riE "save (this|it) to the knowledge|want me to save|should I save|knowledge base\?" tests/harbor/jobs/<job>/`.
- **Expected result**: no transcript contains a question asking whether to save knowledge. Any knowledge written appears as a `knowledge write` call followed by a report naming the entry, and the suites still reach their finished step.
- **Who / when**: the maintainer, before the release that ships this change.

## 2. A prune preview on this project's own knowledge base

- **What to measure**: the dry run completes and lists every entry with a verdict. Threshold: every entry `knowledge list` reports (category READMEs aside) appears in the final report exactly once.
- **How**: from the project root, record `spektacular knowledge list > /tmp/before-list.json` and hash the stores (`find .spektacular/knowledge -type f -exec sha256sum {} + | sort > /tmp/before.sha`). Start an agent session, ask it to "preview a prune of the knowledge base" so the `spek-knowledge` skill runs `spektacular knowledge prune --dry-run`, and let it answer every entry with `knowledge prune verdict` until the final report. Then hash the stores again (`/tmp/after.sha`).
- **Expected result**: the report has `"complete": true`, one row per listed entry (verdicts `would_remove`, `would_correct`, `would_delete` or `current`), and `diff /tmp/before.sha /tmp/after.sha` is empty, including `.usage-index.json`. `.spektacular/knowledge-prune.json` no longer exists.
- **Who / when**: the maintainer, after installing the release, between workflows.

## 3. Evidence a reviewer can check without opening the code

- **What to measure**: every corrected or deleted entry in a prune report carries evidence naming a specific file, command, flag or behaviour. Threshold: 100% of `corrected`/`deleted` (or `would_correct`/`would_delete`) rows.
- **How**: take the final report from procedure 2 (or 5) and read each change row's `evidence` field without opening the code.
- **Expected result**: each one names what was checked and what was found (for example "the --legacy flag no longer exists; cmd/knowledge.go registers no such flag"), specific enough to confirm with one command or file lookup. Vague evidence ("looks outdated") fails.
- **Who / when**: a reviewer other than the person who ran the prune, right after procedure 2 or 5.

## 4. Review: a learned fact is saved without a question (spec acceptance "A learned fact is saved without a question")

- **What the reviewer looks at**: an agent session in a project initialised with the new binary (`spektacular init claude` in a throwaway git project with one registered repo).
- **How**: confirm `AGENTS.md`'s "Knowledge-Worthy Discovery Recognition" section says to write via `spek-knowledge` and then tell the user which entry, where and why. Then ask the agent a question whose answer needs a non-obvious discovery (for example "why does this function retry three times?" in code with an undocumented reason in a commit message), and let it work.
- **What counts as passing**: afterwards `spektacular knowledge list` shows a new or updated entry, `spektacular knowledge usage` shows its record with a `created` date, the transcript contains no question asking whether to save it, and the agent's reply names the entry's tier, store, path and why it was kept.

## 5. Review: an agent-driven prune corrects, deletes and keeps the right entries

- **What the reviewer looks at**: a throwaway project seeded with three entries in its repo store: `gotchas/wrong-flag.md` describing a real command's flag wrongly (for example claiming `knowledge search` takes `--limit`), `architecture/removed-command.md` describing a command that does not exist (for example `spektacular knowledge export`), and `conventions/all-writes-through-cli.md` stating a standard the code does not yet meet.
- **How**: run `spektacular knowledge prune` in an agent session through the `spek-knowledge` skill ("is the knowledge base still accurate?") and let it finish.
- **What counts as passing**: the report lists `wrong-flag.md` as `corrected` (its file now describes the flag correctly, and `knowledge usage` shows its counts unchanged), `removed-command.md` as `deleted` (file and usage record gone), and `all-writes-through-cli.md` as `current` with its bytes unchanged; the two changes carry checkable evidence, and the agent asked nothing before acting.

## 6. Review: the website pages read correctly

- **What the reviewer looks at**: the docs repo's knowledge-base and configuration pages in a local build.
- **How**: in the docs repo, `npm ci`, then `npm run dev` and open `/knowledge-base/` and `/configuration/`.
- **What counts as passing**: the knowledge-base page reads in order (lifecycle, "How usage is recorded", "Pruning the knowledge base", ranking, tagging, configuration, why) with alternating band shading, says agents write without asking, describes the usage index, prune's two passes, the dry-run preview and `knowledge.prune.max_age_days`, and its code blocks render; the configuration page's `knowledge` key shows the default `prune.max_age_days: 90` and describes the setting. No em dashes in the new prose.
