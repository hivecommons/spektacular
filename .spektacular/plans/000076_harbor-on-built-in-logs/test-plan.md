---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000076_harbor-on-built-in-logs

These checks need Docker, the Harbor CLI and the agents' logins, so they are run by a person. Run them from the main checkout (`/home/nicj/code/github.com/hivecommons/spektacular`) once this spec is merged, with a `spektacular` built from the merged tree (`make build`) on `PATH` or named by `SPEKTACULAR_BIN`, so `report.py`, `metrics.py` and `timeline.py` can run `spektacular stats` locally. The suite images get a binary built from the working tree by the Makefile's render targets.

Throughout, `<trial>` is a collected trial directory such as `tests/harbor/jobs/<job>/<suite>-<agent>__<id>`.

## Success metric: the same steps and helper counts for every agent

**What to measure.** For one scenario, the metrics records of Claude, Codex and Bob runs have:
- the same set of steps in `steps.workflows[*].steps[*].step`;
- a helper count for every helper-starting step;
- no agent-specific fields.

**How.**
1. `make harbor-test-implement` runs Claude only (its target pins `AGENT = claude`). For the same implement scenario under all three agents, run the implement-epic suite once per agent, since its lanes are implement runs: `make harbor-test-implement-epic-claude`, `make harbor-test-implement-epic-codex` and `make harbor-test-implement-epic-bob`.
2. For each trial, run `python3 -c "import json,sys; r=json.load(open(sys.argv[1])); print(sorted(r)); print([[s['step'] for s in w['steps']] for w in r['steps']['workflows']]); print([(s['step'], s['helpers']) for w in r['steps']['workflows'] for s in w['steps']])" <trial>/verifier/metrics.json`.

**Expected result.**
- Every record has the same top-level keys.
- Each workflow lists the same step names, in the same order, wherever the agents followed the same path.
- Every step that started a helper shows an integer `helpers`, never `null`.
- The records differ only in `totals` token, cost and model-time fields, `run.model` and `subagents.briefs`.

**Who / when.** The maintainer, after merge and before relying on cross-agent comparisons.

## Success metric: no suite's pass rate drops (5 Claude runs per suite, before and after)

**What to measure.** For each suite, the pass count out of 5 Claude runs on the merged harness must not be lower than the baseline.

The baseline is the 50 jobs under `tests/harbor/jobs/`, prefix `base76-`, collected 2026-10-25 from commit bfd5dc1b52bfa2392f40826d8c0bc42df4f7491a. Their pass counts:

| Suite target | Baseline passes |
|---|---|
| harbor-test-background-child | 5/5 |
| harbor-test-epic-claude | 5/5 |
| harbor-test-implement | 5/5 |
| harbor-test-standing-rules-claude | 5/5 |
| harbor-test-goal-check-claude | 4/5 |
| harbor-test-plan-claude | 3/5 |
| harbor-test-implement-epic-claude | 2/5 |
| harbor-test-repo-delegated | 1/5 |
| harbor-test-repo-one-at-a-time | 0/5 |
| harbor-test-spec-claude | 0/5 |

The spec suite's baseline failures were `TestUserWaits::test_constraints_question_is_a_user_wait` and `TestVerificationStep::test_review_was_a_briefed_child`, every run.

**How.**
1. Run each target above 5 times on the merged tree, naming the jobs `after76-<suite target>-r<1..5>` (rename the job directories under `tests/harbor/jobs/`, or set the job name through `HARBOR_RUN_FLAGS`).
2. Run `python3 tests/harbor/shared/report.py 'tests/harbor/jobs/after76-*'` for the table and its `result` column.
3. Run `python3 tests/harbor/shared/report.py --baseline 'tests/harbor/jobs/base76-*' --candidate 'tests/harbor/jobs/after76-*'` for the medians, which now include user wait, helper wait and gap time.

**Expected result.**
- Each suite's after-count of `pass` rows out of 5 is at least its baseline count.
- Any drop is examined. A check that now fails because a helper launch or user question was not marked as a span is reported, not loosened (plan Open Questions).

**Who / when.** The maintainer, after merge, on the same machine and logins as the baseline.

## Manual review: step timings match `stats run` for Claude, Codex and Bob

**Where.** The three implement-epic trials from the first metric (Claude, Codex, Bob), and the `make harbor-test-implement` trial for Claude.

**How.** For each trial:
1. Run `spektacular stats run --dir <trial>/artifacts/spektacular/logs --format json`.
2. Compare each `steps[i].duration_ms` with `metrics.json`'s `steps.workflows[0].steps[i].duration_s` × 1000. For an epic trial, repeat with `--run <run_id>` for each `steps.workflows[*].run_id`.

**Pass.** Every step and every duration agrees, to the millisecond, for all three agents.

## Manual review: Codex helpers are counted and placed

**Where.** The Codex implement-epic trial (its implementing children start goal-check reviewers).

**How.**
1. Read `<trial>/verifier/metrics.json`: `subagents.started` and `subagents.helpers`.
2. Cross-check with `spektacular stats spans --dir <trial>/artifacts/spektacular/logs --kind implement`.

**Pass.**
- `subagents.started` is non-zero.
- Every helper names its `spec` and `step`; the goal-check reviewer's step is `check_goal`.
- The spans listing shows the same spans.

## Manual review: a spec run reports the per-step split and totals

**Where.** A `make harbor-test-spec-claude` trial.

**How.**
1. Read `metrics.json`: `totals.user_wait_s`, `totals.gap_s`, `totals.helper_wait_s`, and each step's `working_s`, `helper_wait_s` and `user_wait_s`.
2. Run `python3 tests/harbor/shared/report.py --run <trial>`.

**Pass.**
- All the figures are numbers, not `unknown`.
- Each step's three figures add up to its `duration_s`, unless its waits overlapped.
- The constraints step shows user wait above 0 when the agent marked the scripted question.
- The breakdown shows the same figures.

## Manual review: the epic timeline from a trial with no debug directory

**Where.** A `make harbor-test-implement-epic-claude` trial.

**How.** Run `python3 tests/harbor/shared/timeline.py <trial>/artifacts/spektacular/logs <trial>/artifacts/agent-logs/bob`. The Bob path may be absent. Check that `<trial>/artifacts/spektacular/debug` does not exist.

**Pass.**
- Each of the four specs has a lane row with its start, finish and slowest step.
- Each has a hand-off row whose spawned-after and picked-up-after gaps are measured from the orchestrator's `sub-agent` spans: `hand-back`, or the orchestrator's next act for a span never stopped. A child the orchestrator launched with no span shows `no span`.
- The merged-after gap appears for every merged spec.

## Manual review: a collected trial holds the logs, and `stats` reads the copy

**Where.** Any trial from the runs above.

**How.**
1. Run `ls <trial>/artifacts/spektacular/logs/`.
2. Run `spektacular stats run --dir <trial>/artifacts/spektacular/logs` from a folder outside any project.

**Pass.**
- The folder holds the run files and a `standalone_<day>.jsonl`.
- The report prints the trial's steps and timings with no refusal.

## Manual review: missing logs make the measures unknown, and the reward is untouched

**Where.** A copy of any collected trial: `cp -r <trial> /tmp/trial-copy`.

**How.**
1. Run `rm -rf /tmp/trial-copy/artifacts/spektacular/logs`.
2. Run `python3 tests/harbor/shared/metrics.py --agent-dir /tmp/trial-copy/agent --log-dir /tmp/trial-copy/artifacts/spektacular/logs --junit /tmp/trial-copy/verifier/junit.xml --reward /tmp/trial-copy/verifier/reward.txt --out /tmp/metrics.json; echo exit $?`.
3. Run `cat /tmp/trial-copy/verifier/reward.txt`.

**Pass.**
- The exit status is 0.
- `/tmp/metrics.json` has `steps.workflows`, `subagents.helpers`, `subagents.started`, `totals.user_wait_s`, `totals.helper_wait_s`, `totals.gap_s` and `totals.trace_calls` as `null`, each listed under `unknown` with the reason "no Spektacular logs at ...".
- Tokens, cost and briefs are still filled in.
- `reward.txt` is unchanged.

## Manual review: `report.py --compare` over two real trials reads clearly

**Where.** Two trials of the same suite, for example two of the after-runs above.

**How.** Run `python3 tests/harbor/shared/report.py --compare <trial A> <trial B>`.

**Pass.**
- Each workflow is matched across the trials, by name or in order within its kind, since generated spec names differ.
- Each step row shows A, B and B-A for seconds, working, helper, user and helpers, and a step only one trial entered shows `-`, never 0.
- The totals table shows wall, user wait, helper wait, gaps, cost and tokens with their differences.
- A person reading it can tell where B spent more or less time than A.
