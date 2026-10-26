---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Test Plan: 000070_harbor-run-metrics

Manual verification for the success metrics and reviews that need real harbor runs: Docker, agent logins and 15–25 minutes per run. The automated self-checks (`uvx --with pytest pytest --import-mode=importlib tests/harbor/shared/tests`, 228 checks) already cover every rule on synthetic input. These procedures confirm the same behaviour in real containers.

**Before you start**
- Run everything from a checkout that has this change, either the merged `main` or the `spek/000070_harbor-run-metrics` branch, with `make harbor-install` done. Claude and Codex use the login on this machine, so run `bob` and sign in once for the Bob targets.
- Runs land in `tests/harbor/jobs/` of the checkout you run them from. Give each batch its own job name so it is easy to select. Each `make harbor-test-*` target accepts harbor's flags only through the Makefile, so after a run, rename the timestamped job directory, e.g. `mv tests/harbor/jobs/2026-10-24__09-00-00 tests/harbor/jobs/m70-claude-plan`.
- Old jobs from before this change have no record and are skipped by the report. That is expected.

Every record below is `<job>/<trial>/verifier/metrics.json`.

---

## 1. Every suite and agent target leaves a record of the same shape, with the reward unchanged

**Manual review**: the spec's first acceptance criterion, and the constraint that the reward keeps its meaning.

**How**: run every existing agent target once:

```bash
make harbor-test-spec-claude harbor-test-spec-codex
make harbor-test-plan-claude harbor-test-plan-codex harbor-test-plan-bob
make harbor-test-implement
make harbor-test-repo-one-at-a-time harbor-test-repo-delegated
make harbor-test-epic-claude harbor-test-epic-codex harbor-test-epic-bob
make harbor-test-implement-epic-claude harbor-test-implement-epic-codex harbor-test-implement-epic-bob
make harbor-test-standing-rules-claude harbor-test-standing-rules-codex harbor-test-standing-rules-bob
make harbor-test-goal-check-claude harbor-test-goal-check-codex harbor-test-goal-check-bob
```

Then, over the new jobs:

```bash
python3 - tests/harbor/jobs/m70-* <<'EOF'
import json, sys, glob
keys = {"schema_version", "run", "result", "totals", "steps", "subagents", "unknown"}
for job in sys.argv[1:]:
    for trial in sorted(glob.glob(f"{job}/*/")):
        rec, reward = f"{trial}verifier/metrics.json", f"{trial}verifier/reward.txt"
        try:
            r = json.load(open(rec))
        except OSError:
            print("NO RECORD", trial); continue
        written = open(reward).read().strip()
        checks = r["result"]["checks"]
        all_pass = checks["total"] is not None and checks["passed"] == checks["total"]
        print(trial, "keys ok" if set(r) == keys else f"KEYS {sorted(set(r) ^ keys)}",
              "reward", written, "record", r["result"]["reward"], "checks", checks["passed"], "/", checks["total"],
              "OK" if (written == "1") == all_pass else "REWARD MISMATCH")
EOF
```

**Expected result**: every trial prints `keys ok` and `OK`, with no `NO RECORD`. `reward.txt` is `1` exactly when `passed == total`, and the record's `result.reward` equals `reward.txt`. `verifier/test-stdout.txt` ends with the recorder's one line, `metrics: <agent> wall …`, and never with a traceback from `metrics.py`.

**Who / when**: the developer merging this change, before acceptance. Running all targets is about 25 runs, so it can be split across days.

---

## 2. A Claude run's totals and sub-agents equal the agent's own summary

**Manual review**: the acceptance criteria "Claude totals match the agent's own log" and "Sub-agent counts match the agent's own log".

**How**: take one Claude plan trial with sub-agents (from step 1, `m70-claude-plan`) and one epic trial (`m70-claude-epic`, several `result` events because of background sub-agents). For each one:

```bash
T=tests/harbor/jobs/m70-claude-plan/plan-workflow-claude__<id>
python3 - "$T" <<'EOF'
import json, sys
t = sys.argv[1]
objs = [json.loads(l) for l in open(f"{t}/agent/claude-code.txt") if l.strip()]
res = [o for o in objs if o.get("type") == "result"]
last = res[-1]
mu = last["modelUsage"].values()
ts = [o["timestamp"] for o in objs if "timestamp" in o]
print("results", len(res))
print("wall  ", res[0]["duration_ms"] / 1000 if len(res) == 1 else f"span {ts[0]} .. {ts[-1]}")
print("model ", last["duration_api_ms"] / 1000, "turns", sum(r["num_turns"] for r in res), "cost", last["total_cost_usd"])
print("tokens", sum(m["inputTokens"] for m in mu), sum(m["outputTokens"] for m in mu),
      sum(m["cacheReadInputTokens"] for m in mu), sum(m["cacheCreationInputTokens"] for m in mu))
s = last["subagent_stats"]; print("subagents", s["spawned"], s["completed"], s["failed"], sum(s["killed"].values()))
r = json.load(open(f"{t}/verifier/metrics.json"))
print("record", json.dumps(r["totals"]), json.dumps(r["subagents"]))
EOF
```

**Expected result**: `totals.wall_time_s` equals `duration_ms/1000` for one result, or the first-to-last timestamp span for several. `model_time_s`, `turns`, `cost_usd`, and `input_tokens`, `output_tokens`, `cache_read_tokens` and `cache_write_tokens` equal the printed values exactly. `subagents.started`, `completed`, `failed` and `stopped` equal the printed sub-agent counts. `durations_s` has one entry per sub-agent that sent a `task_notification` with a duration. The record's `unknown` list has nothing under `totals.` or `steps`.

**Who / when**: the developer, before acceptance.

---

## 3. Bob and Codex record unknowns, not zeros, with wall time and turns present

**Manual review**: the acceptance criteria "Bob runs record unknowns, not zeros" and "Codex runs record unknowns, not zeros". This also settles the plan's open question about Codex's trajectory at verify time.

**How**: on the `m70-bob-plan` and `m70-codex-plan` trials from step 1:

```bash
for t in tests/harbor/jobs/m70-bob-plan/*/ tests/harbor/jobs/m70-codex-plan/*/; do
  python3 -c 'import json,sys; r=json.load(open(sys.argv[1]+"verifier/metrics.json")); print(sys.argv[1]); print(" totals", r["totals"]); print(" subagents", r["subagents"]); [print("  unknown", u["field"], "-", u["reason"]) for u in r["unknown"]]' "$t"
done
```

**Expected result**:
- **Bob:** `cost_usd`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens` and `model_time_s` are `null`. Each of them is listed in `unknown` with a "bob reports no …" reason. `wall_time_s` and `turns` are numbers greater than 0, and there is no `totals.wall_time_s` entry in `unknown`.
- **Codex:** every `subagents` field is `null`, listed with "codex reports no sub-agents". `wall_time_s`, `turns` and `output_tokens` are numbers greater than 0.
- **Stop condition:** if a Codex record has `wall_time_s` or `turns` unknown, STOP and raise it before changing the Codex reader. That would mean harbor's `trajectory.json` was not there at verify time and the rollout fallback did not fill them either, and the spec requires both.

**Who / when**: the developer, before acceptance.

---

## 4. The plan workflow's slowest step is identifiable from the report alone

**Success metric**: finding where a workflow spends its time takes one report command, with no hand-written scripts.

**How**: on the real Claude plan run from step 1:

```bash
make harbor-report ARGS='tests/harbor/jobs/m70-claude-plan --run plan-workflow-claude'
```

**Expected result**: the breakdown lists every plan step from `overview` to `finished`, each with seconds, share and characters. The slowest step, which was `assemble` on the reference run at 368.8 s and 39% of 948.4 s, carries `◀ dominant` when it is over a third of the run's wall time. A person can name the slowest step from this output alone.

**Who / when**: the developer, before acceptance. Repeat whenever the plan workflow changes.

---

## 5. A speed-up is judged by a before-and-after comparison

**Success metric**: the next workflow speed-up is judged by a before-and-after comparison from the report rather than by impression.

**How**: run the plan suite three times on the commit before a workflow change and three times after it. Rename the job directories `before-plan-1..3` and `after-plan-1..3`, then:

```bash
make harbor-report ARGS="--baseline 'tests/harbor/jobs/before-plan-*' --candidate 'tests/harbor/jobs/after-plan-*'"
```

**Expected result**: a `== plan-workflow ==` section with 3 runs in each set, showing the median wall time, turns, output tokens, total tokens, cost and check score for each set, plus the change. A measure whose candidate median is at least 2× the baseline's, or a check-score drop of 0.10 or more, is marked `◀ regression`, and nothing else is. Both sets' `run.commit` values, visible in each record, show the two commits being compared.

**Who / when**: whoever makes the next plan-workflow speed-up, such as the deterministic assemble step, on that change's branch.

---

## 6. Every Claude run's record is complete

**Success metric**: every Claude run's record is complete, with no unknown totals, turns, tokens, cost or step breakdown.

**How**: over every Claude trial from step 1:

```bash
for f in tests/harbor/jobs/m70-*/*claude*/verifier/metrics.json tests/harbor/jobs/m70-*/implement-workflow-claude*/verifier/metrics.json tests/harbor/jobs/m70-*/repo-workflow*/verifier/metrics.json; do
  [ -f "$f" ] && python3 -c 'import json,sys; r=json.load(open(sys.argv[1])); bad=[u["field"] for u in r["unknown"] if u["field"].startswith(("totals","steps"))]; print(sys.argv[1], "COMPLETE" if not bad else bad)' "$f"
done
```

**Expected result**: every line ends in `COMPLETE`. Repo-workflow runs drive no workflow, so they show `workflows: []`, which is complete rather than unknown. A run with background sub-agents that never reported a duration may list `subagents.durations_s`, which is outside this metric.

**Who / when**: the developer, before acceptance.

---

## 7. The report runs as the normal user over collected jobs

**Manual review**: the report needs no elevated permissions despite root-owned agent files.

**How**: as your normal user, never with `sudo`, in the checkout that holds the step 1 jobs:

```bash
find tests/harbor/jobs -maxdepth 4 ! -user "$(id -u)" | head -3   # shows root-owned files exist
make harbor-report; echo "exit=$?"
```

**Expected result**: the `find` lists at least one root-owned path. `make harbor-report` prints one table per suite (and scenario) covering every step 1 trial, with no `Permission denied`, no traceback, and `exit=0`. Older jobs without a record are absent from the table.

**Who / when**: the developer, before acceptance.

---

## 8. The README's harbor documentation is accurate and its commands work

**Manual review**: the acceptance criterion "Usage is documented with the harbor suites".

**How**: open `README.md`, section "End-to-end (Harbor)", and read "Test results", "Run metrics" and "Comparing runs" against a real record from step 1. Run each command in the "Comparing runs" bash block as written, with `<id>` replaced by a real trial name. Run the hand-run recorder command on one trial, writing `--out` to a scratch file. Run the shared self-check command from the self-check paragraph.

**Expected result**: every field in a real `metrics.json` is described in the field table. The Bob and Codex rows match what step 3 showed. Every command exits 0 with the output the text describes, and the self-check reports `228 passed` or more.

**Who / when**: a reviewer who did not write the change, before acceptance.
