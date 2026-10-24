---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Test plan: 000071_harbor-child-agents

These checks need real agents in containers: Docker, the `harbor` CLI and the machine's own Claude (and, for the unknowns review, Codex and Bob) login. They are not in CI. Run them from the repo root after the branch is merged, before the change is called released.

**Before you start:** a harbor run leaves root-owned `agent/sessions` directories under the gitignored `tests/harbor/jobs/`. While they are there, `go list ./...`, and so `go test ./...`, cannot walk the tree. Move old jobs out of the repo (or remove them with root) before running the Go suite.

The nine Claude suites are:
- `make harbor-test-spec`
- `make harbor-test-plan`
- `make harbor-test-implement`
- `make harbor-test-repo-one-at-a-time`
- `make harbor-test-repo-delegated`
- `make harbor-test-epic`
- `make harbor-test-implement-epic`
- `make harbor-test-standing-rules`
- `make harbor-test-goal-check`

Call this the full Claude batch. Its results land in `tests/harbor/jobs/<timestamp>/<trial>/`, each with `verifier/metrics.json` (schema version 2).

## Success metrics

### 1. Every child agent appears in its run's record, with its full context

- **What to measure:** for every run in a full Claude batch, the record lists 100% of the child agents the run started, each with its full launch prompt.
- **How:** run the full Claude batch, then for each trial compare the record with the transcript's own count:
  ```
  python3 - <<'EOF'
  import json, glob, sys
  sys.path.insert(0, "tests/harbor/shared")
  import transcripts as tx
  from pathlib import Path
  for rec in glob.glob("tests/harbor/jobs/<batch>/*/verifier/metrics.json"):
      trial = Path(rec).parents[1]
      objs = tx.read_jsonl(trial / "agent/claude-code.txt")
      results = [o for o in objs if o.get("type") == "result"]
      spawned = (results[-1].get("subagent_stats") or {}).get("spawned") if results else None
      children = json.load(open(rec))["subagents"]["children"] or []
      prompts = {b["id"]: b["input"].get("prompt") for o in objs if o.get("type") == "assistant"
                 for b in (o.get("message") or {}).get("content") or [] if isinstance(b, dict) and b.get("name") == "Agent"}
      ok = all(c["prompt"] == prompts.get(c["id"]) and c["prompt_chars"] == len(c["prompt"] or "") for c in children)
      print(trial.name, len(children), spawned, "prompts identical" if ok else "PROMPT MISMATCH")
  EOF
  ```
- **Expected result:** on every line, the child count equals `spawned` (or both are 0) and the prompts are identical. Nine suites, nine lines, no mismatch.
- **Who / when:** the maintainer, on the first full Claude batch after merge, and again after a Claude Code upgrade.

### 2. A full Claude batch passes with no required delegation skipped and no run flagged idle

- **What to measure:** all nine Claude suites pass (reward 1). None fails a delegation check: `missing_child`, `missing_spec_child`, `context_gap`, `wakeup_while_children_run`, `child_in_background` or `turn_ended_while_child_ran`. No run carries the `◀ idle` mark.
- **How:** after the full Claude batch, run `make harbor-report ARGS='tests/harbor/jobs/<batch>'`. For any failed trial, read `verifier/test-stdout.txt` (or the JUnit file) for the rule names above.
- **Expected result:**
  - every row shows `pass` and no row shows `◀ idle`;
  - no verifier output contains any of the rule names above.
- **If a required step still runs inline:** if `test` or `verify` is still done without a child, the checks are doing their job. Do not relax them. Raise it, since the step wording may need strengthening further (plan Open Questions).
- **Who / when:** the maintainer, on the post-merge batch on the current default model.

### 3. Epic-planning idle time is under a quarter of wall time on the current default model

- **What to measure:** `totals.idle_s / totals.wall_time_s < 0.25` for a Claude epic-planning run. The 2026-10-23 Sonnet 5.5 reference was about 16 of 20.8 minutes, 75%.
- **How:** `make harbor-test-epic`, then `make harbor-report ARGS='--run <the new epic-workflow-claude trial>'` and read the `Idle:` line.
- **Expected result:** `Idle: <s> s of <wall> s (<share>)` with share under 25% and no `◀ idle` mark.
- **Who / when:** the maintainer, after merge, on the current default model.

### 4. No batch failure comes from the walkthrough or per-repo changelog checks misreading how a command was written

- **What to measure:** in the full Claude batch, `test_walkthrough_rendered_before_finished` (plan suite) and the three `*_changelog_was_committed_via_cli` checks (implement suite) fail only when the session log truly lacks the advance or write.
- **How:** if any of these fails, open the trial's session log `artifacts/spektacular/debug/*.jsonl`.
  - For the walkthrough, list each successful `plan new`/`plan goto` event's `state_after.current_step`.
  - For the changelog, list successful `changelog file write` events and their `--repo`.
- **Expected result:** no such failure where the log shows `walkthrough` before `finished`, or a successful write for the repo. Zero misreads.
- **Who / when:** the maintainer, on the post-merge batch.

### 5. A maintainer can say which children a run started and what they were told, from the report alone

- **What to measure:** the run view and the prompts option answer the question without opening the transcript.
- **How:** pick a collected Claude epic or implement-epic run and run:
  ```
  make harbor-report ARGS='--run <trial>'
  make harbor-report ARGS='--run <trial> --prompts'
  ```
- **Expected result:**
  - the run view lists one row per child (nested ones indented with `└`) with kind, spec, step, bg/fg, seconds, ending and context size;
  - `--prompts` prints each child's full context under a `── <id> <kind> <spec>/<step> (<chars> chars)` header;
  - a reviewer can name every child and what it was told without opening `agent/claude-code.txt`.
- **Who / when:** a maintainer other than the implementer, on any post-merge run.

## Manual reviews

### A. The ten-minute probe is on record, and the epic skills state its outcome

- **Where:**
  - `tests/harbor/shared/tests/data/probe-background-child/OUTCOME`;
  - the headless paragraph in `templates/skills/workflows/spek-plan-epic/SKILL.md` (Step 2) and `templates/skills/workflows/spek-implement-epic/SKILL.md` (Step 3).
- **What to look for:**
  - OUTCOME reads `stopped 602 s 2026-10-24 claude-code 2.1.310 …`;
  - both skills keep the foreground workaround because the child was stopped, state "recorded on 2026-10-24 with Claude Code 2.1.310", and say that being told no one answers is not on its own the reason.
- **Passing:** the skills and the OUTCOME file agree, and `delegation.PROBE_OUTCOME` is `"stopped"`.
- **Re-check after any Claude Code upgrade:** run `make harbor-test-background-child` and read the verifier's `PROBE OUTCOME:` line. If the child now completes past 600 s with a notice-woken turn, the skills, `PROBE_OUTCOME` and the epic suites' rule must change together.

### B. Passing Claude epic-planning and epic-implementing runs work their specs the settled way

- **Where:** a passing `make harbor-test-epic` and a passing `make harbor-test-implement-epic` run; `make harbor-report ARGS='--run <trial>'`; the verifier output's `TestChildren` results.
- **What to look for:**
  - every per-spec child is `fg` in the children table;
  - each implement-epic spec's `test`, `verify` and `check_goal` steps have a nested child;
  - no wake-up was set;
  - every spec is planned or implemented (the suites' existing checks).
- **Passing:** all `TestChildren` checks pass, including `test_children_run_as_the_probe_settled`. This is the foreground form, because the probe found background children stopped.

### C. No passing run ends waiting on a reply, and every suite says no one answers

- **Where:**
  - each suite's `tests/harbor/<suite>/instruction.md`;
  - the last assistant message in each passing run's `agent/claude-code.txt`.
- **What to look for:** every instruction has a `## You are running non-interactively` section with "no human at the other end" and "nothing will answer". The suite-wiring self-check already pins this. No passing run's final assistant message is a question to the user.
- **Passing:** no final message in a passing run asks for a reply.

### D. Codex and Bob records mark child details they cannot show as unknown

- **Where:** `make harbor-test-epic-codex` and `make harbor-test-epic-bob` (or the plan suite's `-codex`/`-bob` targets), then each trial's `verifier/metrics.json`.
- **What to look for:**
  - **Codex:** `subagents.children` is `null`, with an `unknown` entry "codex reports no sub-agents".
  - **Bob:** each child has `kind`, `description`, `prompt` and `prompt_chars`, every other child field `null`, and one agent-wide `unknown` entry per missing field (`subagents.children.<field>`, "bob logs no sub-agent …").
  - No child field holds `0` in place of unknown, and neither run fails any delegation check (they skip).
- **Passing:** all of the above hold for both agents.

### E. The README documents the new record, report option and probe, and its commands work

- **Where:** `README.md` `#### Run metrics`, `#### Comparing runs`, and the harbor make-target list.
- **What to look for:**
  - every version-2 field is documented: `totals.idle_s`, each step's `children`, and `subagents.children` with its keys and `ending` values;
  - which agents leave which child fields unknown;
  - the idle flag (`IDLE_SHARE`, a quarter);
  - `make harbor-report ARGS='--run <trial> --prompts'`;
  - the `harbor-test-background-child` probe and its recorded outcome.
  - Run each documented `make harbor-report` command against a real jobs directory.
- **Passing:** every command runs as described, and nothing in the README contradicts the recorder or report output.
