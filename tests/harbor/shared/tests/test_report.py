"""Self-check for the local run report in report.py.

The report reads the metrics record (verifier/metrics.json) and harbor's
result.json of every trial under the paths it is given, and nothing else. It
prints one row per recorded run grouped by suite and scenario, or one run's
step breakdown and its child agents with `--run` (and each child's full
context with `--prompts`), flags a step taking more than a third of its run's
wall time and a run idle for more than a quarter of it, and prints a value it
cannot read as `unknown`, never as zero. Each case here builds a small jobs tree by hand under tmp_path, with
hand-written records and hand-worked expected text, and runs the report over
it, mostly through `main` exactly as `make harbor-report` does.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_report.py
"""

import json
import os
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import report  # noqa: E402
from report import Run, breakdown, dominant_steps, find_run, load_runs, main, table  # noqa: E402

NAME = "20260101000000-user-auth"
SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"

# The trials of the standard tree. Directory names are the trial names.
E5 = "plan-workflow__e5Qk2"  # nightly job, oldest
A1 = "plan-workflow__a1Xy9"  # the reference plan run
B2 = "plan-workflow__b2Lm4"  # no result.json: no started_at, record reward
C3 = "plan-workflow__c3Np8"  # no metrics record: skipped
D4 = "spec-workflow__d4Rt7"  # second suite, no scenario

def step(name, started_at, duration, helper=0.0, user=0.0, entries=1, commands=2, helpers=0, incomplete=0):
    """A schema-3 step row, its split adding up to its duration."""
    return {
        "step": name, "started_at": started_at, "entries": entries, "duration_s": duration,
        "working_s": round(max(0.0, duration - helper - user), 3), "helper_wait_s": helper, "user_wait_s": user,
        "commands": commands, "helpers": helpers, "incomplete": incomplete,
    }


# The reference plan run's steps: wall 948.39 s, assemble 368.8 s (39%).
REFERENCE_STEPS = {
    "workflows": [
        {
            "run_id": "4bf92f3577b34da6a3ce929d0e0e4736", "kind": "plan", "name": NAME, "status": "finished",
            "wall_s": 900.0, "user_wait_s": 30.0, "helper_wait_s": 80.0, "gap_s": 0.0, "gaps": 0,
            "steps": [
                step("discovery", "2026-10-20T10:00:05Z", 120.0, helper=80.0, helpers=1),
                step("architecture", "2026-10-20T10:02:05Z", 210.5, user=30.0, entries=2),
                step("assemble", "2026-10-20T10:05:36Z", 368.8, commands=7),
                step("verification", "2026-10-20T10:11:44Z", 45.3, incomplete=1),
            ],
        }
    ],
}
REFERENCE_HELPERS = [
    {"span_id": "h1", "parent": "s1", "name": "research", "spec": NAME, "kind": "plan", "step": "discovery", "task_id": None,
     "outside_run": False, "started_at": "2026-10-20T10:00:20Z", "ended_at": "2026-10-20T10:01:40Z", "duration_s": 80.0,
     "end_reason": "stopped", "status": "ok", "complete": True},
    {"span_id": "h2", "parent": None, "name": "plan alpha", "spec": NAME, "kind": None, "step": None, "task_id": None,
     "outside_run": True, "started_at": "2026-10-20T09:59:00Z", "ended_at": "2026-10-20T09:59:00Z", "duration_s": 0.0,
     "end_reason": "open", "status": None, "complete": False},
]
REFERENCE_BRIEFS = [
    {"id": "toolu_01AbCdEfGh", "parent": None, "kind": "general-purpose", "description": "Research the codebase",
     "models": [SONNET], "prompt": "Find every caller of the auth module.", "prompt_chars": 37},
]

NULL_TOTALS = dict.fromkeys(
    [
        "wall_time_s",
        "model_time_s",
        "turns",
        "cost_usd",
        "input_tokens",
        "output_tokens",
        "cache_read_tokens",
        "cache_write_tokens",
    ]
)


def make_record(
    suite="plan-workflow",
    scenario="default",
    agent="claude-code",
    model=SONNET,
    reward=1.0,
    checks=None,
    totals=None,
    steps=None,
    subagents=None,
    unknown=None,
):
    """A full record in metrics.py's shape, with a few fields overridden."""
    return {
        "schema_version": 3,
        "run": {
            "suite": suite,
            "scenario": scenario,
            "agent": agent,
            "model": model,
            "spektacular_version": "0.42.0",
            "commit": "abc1234",
            "dirty": False,
        },
        "result": {
            "reward": reward,
            "checks": checks if checks is not None else {"passed": 12, "total": 12, "score": 1.0},
            "groups": None,
        },
        "totals": totals
        if totals is not None
        else {
            "wall_time_s": 600.0,
            "model_time_s": 500.0,
            "turns": 40,
            "cost_usd": 0.5,
            "input_tokens": 1000,
            "output_tokens": 9000,
            "cache_read_tokens": 500000,
            "cache_write_tokens": 20000,
        },
        "steps": steps if steps is not None else {"workflows": []},
        "subagents": subagents
        if subagents is not None
        else {"started": 0, "completed": 0, "failed": 0, "unfinished": 0, "durations_s": [], "helpers": [], "briefs": []},
        "unknown": unknown if unknown is not None else [],
    }


def totals(wall, turns, out, cache_read, cost, user=0.0, helper=0.0, gap=0.0, gaps=0):
    return {
        "user_wait_s": user,
        "helper_wait_s": helper,
        "gap_s": gap,
        "gaps": gaps,
        "gap_threshold_s": 300.0,
        "wall_time_s": wall,
        "model_time_s": None,
        "turns": turns,
        "cost_usd": cost,
        "input_tokens": 100,
        "output_tokens": out,
        "cache_read_tokens": cache_read,
        "cache_write_tokens": 0,
    }


def write_trial(jobs: Path, job: str, trial: str, record=None, started_at=None, reward=None, result=True) -> Path:
    """<jobs>/<job>/<trial>/ with verifier/metrics.json (when a record is
    given) and harbor's result.json (unless result is False)."""
    trial_dir = jobs / job / trial
    (trial_dir / "verifier").mkdir(parents=True)
    if record is not None:
        (trial_dir / "verifier" / "metrics.json").write_text(json.dumps(record, indent=2))
    if result:
        (trial_dir / "result.json").write_text(
            json.dumps(
                {
                    "trial_name": trial,
                    "started_at": started_at,
                    "verifier_result": {"rewards": {"reward": reward}},
                }
            )
        )
    return trial_dir


def standard_jobs(tmp_path: Path) -> Path:
    """Three jobs, two suites (one with a scenario), one trial without a
    record and one without result.json."""
    jobs = tmp_path / "jobs"
    write_trial(
        jobs,
        "fix-plan-2026-10-20",
        A1,
        make_record(
            reward=1.0,
            checks={"passed": 11, "total": 12, "score": 0.9167},
            totals=totals(948.39, 87, 41230, 1234567, 4.12, user=30.0, helper=80.0),
            steps=REFERENCE_STEPS,
            subagents={"started": 2, "completed": 1, "failed": 0, "unfinished": 1, "durations_s": [80.0],
                       "helpers": REFERENCE_HELPERS, "briefs": REFERENCE_BRIEFS},
        ),
        started_at="2026-10-20T10:00:00Z",
        reward=1.0,
    )
    write_trial(
        jobs,
        "fix-plan-2026-10-20",
        B2,
        make_record(
            reward=0.0,
            checks={"passed": 6, "total": 12, "score": 0.5},
            totals=totals(1500.0, 120, 60000, 2000000, 7.0, user=120.0, gap=600.0, gaps=1),
        ),
        result=False,
    )
    write_trial(jobs, "fix-plan-2026-10-20", C3, None, started_at="2026-10-20T11:00:00Z", reward=1.0)
    write_trial(
        jobs,
        "fix-spec-2026-10-21",
        D4,
        # The record says 1.0 but result.json's 0.0 wins: zero is a reward, not a gap.
        make_record(
            suite="spec-workflow",
            scenario=None,
            agent="bob",
            model=HAIKU,
            reward=1.0,
            checks={"passed": 4, "total": 8, "score": 0.5},
            totals=totals(330.0, 22, 3100, 75000, 0.25),
        ),
        started_at="2026-10-21T09:00:00Z",
        reward=0.0,
    )
    write_trial(
        jobs,
        "nightly-2026-10-19",
        E5,
        # The record says 0.0 but result.json's 1.0 wins.
        make_record(agent="bob", model=HAIKU, reward=0.0, totals=totals(600.0, 40, 9000, 500000, 0.5)),
        started_at="2026-10-19T08:00:00Z",
        reward=1.0,
    )
    return jobs


PLAN_GROUP = """\
== plan-workflow (default) ==
trial                 agent        model              wall m  user wait m  gap m  turns  out tok  cache-read tok  cost $  checks        result
--------------------  -----------  -----------------  ------  -----------  -----  -----  -------  --------------  ------  ------------  ------
plan-workflow__e5Qk2  bob          claude-haiku-4-5     10.0          0.0    0.0     40    9,000         500,000    0.50  12/12 (1.00)  pass
plan-workflow__a1Xy9  claude-code  claude-sonnet-4-6    15.8          0.5    0.0     87   41,230       1,234,567    4.12  11/12 (0.92)  pass
plan-workflow__b2Lm4  claude-code  claude-sonnet-4-6    25.0          2.0   10.0    120   60,000       2,000,000    7.00  6/12 (0.50)   fail  ◀ gaps
"""

SPEC_GROUP = """\
== spec-workflow ==
trial                 agent  model             wall m  user wait m  gap m  turns  out tok  cache-read tok  cost $  checks      result
--------------------  -----  ----------------  ------  -----------  -----  -----  -------  --------------  ------  ----------  ------
spec-workflow__d4Rt7  bob    claude-haiku-4-5     5.5          0.0    0.0     22    3,100          75,000    0.25  4/8 (0.50)  fail
"""

A1_BREAKDOWN = """\
== plan-workflow__a1Xy9 ==
plan-workflow (default), claude-code on claude-sonnet-4-6, pass
wall 15.8 m, turns 87, cost $4.12, checks 11/12 (0.92)

plan 20260101000000-user-auth: wall 900.0 s, user wait 30.0 s, helper wait 80.0 s, gaps 0.0 s
  step          entries  seconds  share  working  helper wait  user wait  helpers
  ------------  -------  -------  -----  -------  -----------  ---------  -------
  discovery           1    120.0    13%     40.0         80.0        0.0        1
  architecture        2    210.5    22%    180.5          0.0       30.0        0
  assemble            1    368.8    39%    368.8          0.0        0.0        0  ◀ dominant
  verification        1     45.3     5%     45.3          0.0        0.0        0

Totals: wall 948.4 s, user wait 30.0 s, helper wait 80.0 s, gaps over 300.0 s: 0.0 s (0), 0% of wall
Helpers: started 2, completed 1, failed 0, unfinished 1; durations (s): 80.0

Helper spans:
  helper      spec                      step             seconds  end
  ----------  ------------------------  ---------------  -------  -------
  research    20260101000000-user-auth  discovery           80.0  stopped
  plan alpha  20260101000000-user-auth  outside any run      0.0  open*
"""


def run_of(record, trial="plan-workflow__x", reward=1.0) -> Run:
    return Run(trial_dir=Path("/nonexistent") / trial, trial=trial, job="job", started_at=None, reward=reward, record=record)


# 1. The run table.


def test_table_prints_one_row_per_recorded_run_grouped_by_suite_and_scenario(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs)]) == 0

    captured = capsys.readouterr()
    assert captured.out == PLAN_GROUP + "\n" + SPEC_GROUP + "\n"
    assert C3 not in captured.out
    assert captured.err == ""


def test_table_function_matches_main_output(tmp_path):
    runs = load_runs([str(standard_jobs(tmp_path))])

    assert table(runs) == PLAN_GROUP + "\n" + SPEC_GROUP


def test_runs_are_sorted_by_started_at_with_unstarted_last(tmp_path):
    runs = load_runs([str(standard_jobs(tmp_path))])

    assert [r.trial for r in runs] == [E5, A1, D4, B2]
    assert [r.started_at for r in runs] == [
        "2026-10-19T08:00:00Z",
        "2026-10-20T10:00:00Z",
        "2026-10-21T09:00:00Z",
        None,
    ]


def test_result_json_reward_wins_and_record_reward_is_the_fallback(tmp_path):
    runs = {r.trial: r for r in load_runs([str(standard_jobs(tmp_path))])}

    assert runs[E5].reward == 1.0  # result.json 1.0 over record 0.0
    assert runs[D4].reward == 0.0  # result.json 0.0 over record 1.0
    assert runs[B2].reward == 0.0  # no result.json: the record's 0.0
    assert runs[A1].job == "fix-plan-2026-10-20"


def test_glob_expands_to_several_job_dirs(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([f"{jobs}/fix*"]) == 0

    out = capsys.readouterr().out
    assert "== plan-workflow (default) ==" in out
    assert "== spec-workflow ==" in out
    assert A1 in out and B2 in out and D4 in out
    assert E5 not in out  # nightly-* does not match fix*
    assert [r.trial for r in load_runs([f"{jobs}/fix*"])] == [A1, D4, B2]


def test_glob_matching_nothing_warns_and_reports_no_records(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    pattern = f"{jobs}/weekly*"

    assert main([pattern]) == 0

    captured = capsys.readouterr()
    assert captured.err == f"report: {pattern} matches nothing\n"
    assert captured.out == f"No metrics records under {pattern}. Runs from before the recorder have none.\n"


def test_trial_dir_and_job_dir_both_work_as_paths(tmp_path):
    jobs = standard_jobs(tmp_path)

    assert [r.trial for r in load_runs([str(jobs / "fix-plan-2026-10-20" / A1)])] == [A1]
    assert [r.trial for r in load_runs([str(jobs / "fix-plan-2026-10-20")])] == [A1, B2]
    assert [r.trial for r in load_runs([str(jobs / "fix-plan-2026-10-20" / C3)])] == []


def test_the_same_record_reached_twice_is_listed_once(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    job = jobs / "fix-spec-2026-10-21"

    runs = load_runs([str(job), str(job / D4), str(jobs), str(job)])

    assert sorted(r.trial for r in runs) == sorted([A1, B2, D4, E5])
    assert main([str(job), str(job / D4)]) == 0
    assert capsys.readouterr().out.count(D4) == 1


def test_malformed_record_is_skipped_with_a_warning(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    broken = write_trial(jobs, "fix-spec-2026-10-21", "spec-workflow__zz999", None, started_at="2026-10-21T10:00:00Z", reward=1.0)
    (broken / "verifier" / "metrics.json").write_text('{"schema_version": 1, "run": ')

    assert main([str(jobs / "fix-spec-2026-10-21")]) == 0

    captured = capsys.readouterr()
    assert captured.err.startswith(f"report: skipping unreadable {broken / 'verifier' / 'metrics.json'}: ")
    assert captured.err.count("\n") == 1
    assert "spec-workflow__zz999" not in captured.out
    assert D4 in captured.out


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 file")
def test_unreadable_record_is_skipped_with_a_warning(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    record = jobs / "fix-spec-2026-10-21" / D4 / "verifier" / "metrics.json"
    record.chmod(0)
    try:
        assert main([str(jobs)]) == 0
        captured = capsys.readouterr()
    finally:
        record.chmod(0o644)

    assert captured.err.startswith(f"report: skipping unreadable {record}: ")
    assert D4 not in captured.out
    assert "== spec-workflow ==" not in captured.out
    assert captured.out == PLAN_GROUP + "\n"


# A metrics.json that parses but is not a JSON object (e.g. `[]`) is skipped
# with a warning, not silently.
def test_non_object_record_is_skipped_with_a_warning(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    trial = write_trial(jobs, "job", "plan-workflow__list1", None, started_at="2026-10-20T10:00:00Z", reward=1.0)
    (trial / "verifier" / "metrics.json").write_text("[]")

    assert main([str(jobs)]) == 0

    captured = capsys.readouterr()
    assert "plan-workflow__list1" not in captured.out
    assert "metrics.json" in captured.err


def test_no_records_prints_the_message_and_exits_zero(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    write_trial(jobs, "old-job", C3, None, started_at="2026-01-01T00:00:00Z", reward=1.0)

    assert main([str(jobs)]) == 0

    captured = capsys.readouterr()
    assert captured.out == f"No metrics records under {jobs}. Runs from before the recorder have none.\n"
    assert captured.err == ""


# 2. One run's step breakdown.


def test_run_by_trial_name_prints_every_step_with_duration_and_output(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs), "--run", A1]) == 0

    assert capsys.readouterr().out == A1_BREAKDOWN + "\n"


def test_run_by_unique_prefix(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs), "--run", "plan-workflow__a1"]) == 0

    assert capsys.readouterr().out == A1_BREAKDOWN + "\n"


def test_run_by_trial_dir(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs), "--run", str(jobs / "fix-plan-2026-10-20" / A1)]) == 0

    assert capsys.readouterr().out == A1_BREAKDOWN + "\n"


def test_find_run_by_trial_dir_picks_that_trial_when_names_collide(tmp_path):
    jobs = tmp_path / "jobs"
    first = write_trial(jobs, "job-one", "plan-workflow__same1", make_record(), started_at="2026-10-20T10:00:00Z", reward=1.0)
    second = write_trial(jobs, "job-two", "plan-workflow__same1", make_record(), started_at="2026-10-21T10:00:00Z", reward=0.0)
    runs = load_runs([str(jobs)])

    assert find_run(runs, str(second)).job == "job-two"
    assert find_run(runs, str(first)).job == "job-one"
    with pytest.raises(LookupError) as exc:
        find_run(runs, "plan-workflow__same1")
    assert str(first) in str(exc.value) and str(second) in str(exc.value)


def test_ambiguous_prefix_exits_2_naming_the_matching_trial_dirs(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs), "--run", "plan-workflow__"]) == 2

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == (
        "report: 'plan-workflow__' matches 3 runs; pass one trial directory instead:\n"
        f"  {jobs / 'nightly-2026-10-19' / E5}\n"
        f"  {jobs / 'fix-plan-2026-10-20' / A1}\n"
        f"  {jobs / 'fix-plan-2026-10-20' / B2}\n"
    )


def test_no_match_exits_2(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main([str(jobs), "--run", "implement-workflow__q"]) == 2

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == (
        "report: no recorded run matches 'implement-workflow__q'; "
        "pass a trial directory or a trial name from the table\n"
    )


# `--run <trial dir>` finds a trial directory wherever it lives, without it
# also being passed as a path.
def test_run_by_trial_dir_outside_the_default_jobs_without_paths(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)

    assert main(["--run", str(jobs / "fix-plan-2026-10-20" / A1)]) == 0

    assert capsys.readouterr().out == A1_BREAKDOWN + "\n"


# 3. Dominant steps.


def test_reference_plan_run_flags_assemble_only(tmp_path):
    run = find_run(load_runs([str(standard_jobs(tmp_path))]), A1)

    flagged = dominant_steps(run)

    assert [(lane, step) for lane, step, _ in flagged] == [(NAME, "assemble")]
    assert flagged[0][2] == pytest.approx(0.38887, abs=1e-5)  # 368.8 / 948.39


def test_step_at_033_of_wall_is_not_flagged_and_just_over_a_third_is():
    steps = {
        "unit": "characters",
        "workflows": [
            {
                "kind": "plan",
                "name": NAME,
                "steps": [
                    {"step": "point-33", "started_at": None, "duration_s": 312.9687, "output": 10},  # 0.33 x 948.39
                    {"step": "over-third", "started_at": None, "duration_s": 316.2, "output": 10},  # 948.39 / 3 = 316.13
                ],
            }
        ],
    }
    run = run_of(make_record(totals=totals(948.39, 1, 1, 1, 1.0), steps=steps))

    assert [step for _, step, _ in dominant_steps(run)] == ["over-third"]


def test_step_at_exactly_a_third_is_not_flagged():
    steps = {
        "unit": "characters",
        "workflows": [
            {
                "kind": "plan",
                "name": NAME,
                "steps": [
                    {"step": "third", "started_at": None, "duration_s": 100.0, "output": 1},
                    {"step": "rest", "started_at": None, "duration_s": 99.0, "output": 1},
                ],
            }
        ],
    }
    run = run_of(make_record(totals=totals(300.0, 1, 1, 1, 1.0), steps=steps))

    assert dominant_steps(run) == []
    assert "◀ dominant" not in breakdown(run)


def test_unknown_wall_time_uses_the_sum_of_step_durations():
    steps = {
        "unit": "characters",
        "workflows": [
            {
                "kind": "spec",
                "name": NAME,
                "steps": [
                    {"step": "overview", "started_at": None, "duration_s": 60.0, "output": 100},
                    {"step": "requirements", "started_at": None, "duration_s": 30.0, "output": 200},
                ],
            },
            {
                "kind": "plan",
                "name": NAME,
                "steps": [{"step": "discovery", "started_at": None, "duration_s": 10.0, "output": 300}],
            },
        ],
    }
    run = run_of(make_record(totals=totals(None, 5, 1, 1, 1.0), steps=steps))

    assert [(lane, step) for lane, step, _ in dominant_steps(run)] == [(NAME, "overview")]
    assert dominant_steps(run)[0][2] == pytest.approx(0.6)
    text = breakdown(run)
    assert "wall unknown m, turns 5, cost $1.00, checks 12/12 (1.00)" in text
    # a version-1 record: no entries or split, which print as unknown
    assert "  overview      unknown     60.0    60%  unknown      unknown    unknown  unknown  ◀ dominant" in text
    assert "  requirements  unknown     30.0    30%  unknown      unknown    unknown  unknown\n" in text
    assert "  discovery  unknown     10.0    10%  unknown      unknown    unknown  unknown\n" in text


# 4. Unknown values.


def null_record():
    return make_record(
        model=None,
        reward=None,
        checks={"passed": None, "total": None, "score": None},
        totals=NULL_TOTALS,
        steps={
            "workflows": [
                {
                    "kind": "plan",
                    "name": NAME,
                    "steps": [dict.fromkeys(["step", "started_at", "entries", "duration_s", "working_s", "helper_wait_s", "user_wait_s", "commands", "helpers", "incomplete"]) | {"step": "discovery"}],
                }
            ],
        },
        subagents=dict.fromkeys(["started", "completed", "failed", "unfinished", "durations_s", "helpers", "briefs"]),
        unknown=[{"field": "totals", "reason": "no agent log"}],
    )


NULL_BREAKDOWN = """\
== plan-workflow__n0Ul1 ==
plan-workflow (default), claude-code on unknown, unknown
wall unknown m, turns unknown, cost $unknown, checks unknown

plan 20260101000000-user-auth: wall unknown s, user wait unknown s, helper wait unknown s, gaps unknown s
  step       entries  seconds    share  working  helper wait  user wait  helpers
  ---------  -------  -------  -------  -------  -----------  ---------  -------
  discovery  unknown  unknown  unknown  unknown      unknown    unknown  unknown

Totals: wall unknown s, user wait unknown s, helper wait unknown s, gaps over unknown s: unknown s (unknown)
Helpers: started unknown, completed unknown, failed unknown, unfinished unknown; durations (s): unknown

Helper spans: unknown (see below)

Unknown:
  totals: no agent log
"""


def test_unknown_values_print_as_unknown_in_the_table(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    write_trial(jobs, "job", "plan-workflow__n0Ul1", null_record(), result=False)

    assert main([str(jobs)]) == 0

    lines = capsys.readouterr().out.splitlines()
    assert lines[0] == "== plan-workflow (default) =="
    assert lines[3] == (
        "plan-workflow__n0Ul1  claude-code  unknown  unknown      unknown  unknown  unknown  unknown"
        "         unknown  unknown  unknown  unknown"
    )
    assert "0.0" not in lines[3] and " 0 " not in lines[3]


def test_unknown_values_print_as_unknown_in_the_breakdown(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    write_trial(jobs, "job", "plan-workflow__n0Ul1", null_record(), result=False)

    assert main([str(jobs), "--run", "plan-workflow__n0Ul1"]) == 0

    assert capsys.readouterr().out == NULL_BREAKDOWN + "\n"


def test_null_checks_section_and_unknown_workflows_print_as_unknown():
    record = make_record(steps={"workflows": None})
    record["result"]["checks"] = None
    record["unknown"] = [{"field": "steps", "reason": "no session log"}]

    text = breakdown(run_of(record, trial="plan-workflow__w1", reward=None))

    assert "plan-workflow (default), claude-code on claude-sonnet-4-6, unknown\n" in text
    assert "wall 10.0 m, turns 40, cost $0.50, checks unknown\n" in text
    assert "Steps: unknown (see below)\n" in text
    assert "  steps: no session log\n" in text


def test_null_subagent_durations_alone_print_as_unknown_and_empty_as_none():
    record = make_record(subagents={"started": 2, "completed": 2, "failed": 0, "unfinished": 0, "durations_s": None, "helpers": [], "briefs": []})
    assert (
        "Helpers: started 2, completed 2, failed 0, unfinished 0; durations (s): unknown\n"
        in breakdown(run_of(record))
    )

    record = make_record()  # no helpers: an empty list, which is known
    assert (
        "Helpers: started 0, completed 0, failed 0, unfinished 0; durations (s): none\n"
        in breakdown(run_of(record))
    )
    assert "Helper spans: none\n" in breakdown(run_of(record))
    assert "Steps: the run entered no workflow\n" in breakdown(run_of(record))


# 5. Unreadable agent files.


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 directory")
def test_report_runs_as_a_normal_user_over_unreadable_agent_files(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    trial = jobs / "fix-plan-2026-10-20" / A1
    agent = trial / "agent"
    (agent / "sessions").mkdir(parents=True)
    log = agent / "claude-code.txt"
    log.write_text("root-owned agent log\n")
    log.chmod(0)
    (agent / "sessions").chmod(0)
    agent.chmod(0)
    try:
        assert main([str(jobs)]) == 0
        from_jobs = capsys.readouterr()
        assert main([str(jobs / "fix-plan-2026-10-20")]) == 0
        from_job = capsys.readouterr()
        assert main([str(trial)]) == 0
        from_trial = capsys.readouterr()
        assert main([str(jobs), "--run", A1]) == 0
        from_run = capsys.readouterr()
    finally:
        agent.chmod(0o755)
        (agent / "sessions").chmod(0o755)
        log.chmod(0o644)

    assert from_jobs.out == PLAN_GROUP + "\n" + SPEC_GROUP + "\n"
    assert from_jobs.err == ""
    assert "plan-workflow__a1Xy9  claude-code  claude-sonnet-4-6    15.8" in from_job.out
    assert from_job.err == ""
    assert "plan-workflow__a1Xy9  claude-code  claude-sonnet-4-6    15.8" in from_trial.out
    assert from_trial.err == ""
    assert from_run.out == A1_BREAKDOWN + "\n"


def test_thresholds_are_the_documented_values():
    assert report.DOMINANT_STEP_SHARE == 1 / 3
    assert report.GAP_SHARE == 0.25


def test_comparison_thresholds_are_the_documented_values():
    assert report.REGRESSION_RATIO == 2.0
    assert report.SCORE_DROP == 0.1


# 6. Comparing a baseline set with a candidate set.

PLAN = ("plan-workflow", "default")


def compared_trial(
    jobs: Path, job: str, trial: str, wall, turns, out, cache_read, cost, score, suite="plan-workflow", scenario="default"
):
    """A trial in <jobs>/<job>/ whose record has these totals (input tokens
    100, cache-write tokens 0) and this check score."""
    record = make_record(
        suite=suite,
        scenario=scenario,
        checks={"passed": None if score is None else round(score * 10), "total": 10, "score": score},
        totals=totals(wall, turns, out, cache_read, cost),
    )
    return write_trial(jobs, job, trial, record, started_at="2026-10-20T10:00:00Z", reward=1.0)


def comparison_jobs(tmp_path: Path) -> tuple[Path, Path]:
    """Three plan-workflow runs in each set, chosen so every median differs
    from the mean, plus a suite only in the baseline and one only in the
    candidate.

    Baseline medians: wall 660 s, turns 11, out 1,100, total 12,200 (11,100 /
    12,200 / 55,100), cost 1.10, score 0.9. Candidate medians: wall 1,320 s
    (exactly twice), turns 12, out 900, total 10,600 (10,600 / 21,000 /
    10,100), cost 2.00 (x1.82), score 0.8 (a drop of 0.10)."""
    base, cand = tmp_path / "base", tmp_path / "cand"
    compared_trial(tmp_path, "base", "plan-workflow__b1", 600.0, 10, 1000, 10000, 1.0, 1.0)
    compared_trial(tmp_path, "base", "plan-workflow__b2", 660.0, 11, 1100, 11000, 1.1, 0.9)
    compared_trial(tmp_path, "base", "plan-workflow__b3", 3000.0, 50, 5000, 50000, 5.0, 0.5)
    compared_trial(tmp_path, "base", "spec-workflow__b4", 300.0, 5, 500, 5000, 0.5, 1.0, suite="spec-workflow", scenario=None)
    compared_trial(tmp_path, "cand", "plan-workflow__c1", 1200.0, 5, 500, 10000, 2.0, 0.8)
    compared_trial(tmp_path, "cand", "plan-workflow__c2", 1320.0, 12, 900, 20000, 2.5, 0.7)
    compared_trial(tmp_path, "cand", "plan-workflow__c3", 1400.0, 100, 9000, 1000, 0.1, 1.0)
    compared_trial(tmp_path, "cand", "implement-workflow__c4", 900.0, 30, 3000, 9000, 1.5, 1.0, suite="implement-workflow")
    return base, cand


FULL_COMPARISON = """\
== plan-workflow (default) ==  baseline 3 run(s), candidate 3 run(s)
median         baseline  candidate          change
-------------  --------  ---------  --------------
wall m             11.0       22.0   +11.0 (x2.00)  ◀ regression
user wait m         0.0        0.0            +0.0
helper wait m       0.0        0.0            +0.0
gap m               0.0        0.0            +0.0
turns                11         12      +1 (x1.09)
out tok           1,100        900    -200 (x0.82)
total tok        12,200     10,600  -1,600 (x0.87)
cost $             1.10       2.00   +0.90 (x1.82)
check score        0.90       0.80           -0.10  ◀ regression

Unmatched, in one set only:
  spec-workflow: baseline only
  implement-workflow (default): candidate only
"""


def test_comparison_prints_medians_change_and_unmatched_suites(tmp_path, capsys):
    base, cand = comparison_jobs(tmp_path)

    assert main(["--baseline", str(base), "--candidate", str(cand)]) == 0

    captured = capsys.readouterr()
    assert captured.out == FULL_COMPARISON + "\n"
    assert captured.err == ""


def test_regressions_returns_the_flags_of_the_full_comparison(tmp_path):
    base, cand = comparison_jobs(tmp_path)

    flags = report.regressions(load_runs([str(base)]), load_runs([str(cand)]))

    assert flags == [
        report.Flag(PLAN, "wall m", 660.0, 1320.0),
        report.Flag(PLAN, "check score", 0.9, 0.8),
    ]


def test_no_suite_in_both_sets_says_so_and_lists_the_unmatched(tmp_path, capsys):
    compared_trial(tmp_path, "base", "spec-workflow__b1", 300.0, 5, 500, 5000, 0.5, 1.0, suite="spec-workflow", scenario=None)
    compared_trial(tmp_path, "cand", "plan-workflow__c1", 600.0, 10, 1000, 10000, 1.0, 1.0)

    assert main(["--baseline", str(tmp_path / "base"), "--candidate", str(tmp_path / "cand")]) == 0

    assert capsys.readouterr().out == (
        "No suite has runs in both sets.\n"
        "\n"
        "Unmatched, in one set only:\n"
        "  spec-workflow: baseline only\n"
        "  plan-workflow (default): candidate only\n"
        "\n"
    )


# Each cost-like measure: one baseline run, and a candidate run that changes
# only that measure, to exactly 2.0x and to 1.99x. The baseline's total tokens
# are 100 + 1,000 + 8,900 + 0 = 10,000; the total-token case moves cache-read
# tokens only, so the output-token measure is unchanged.
BASE_TOTALS = {"wall": 600.0, "turns": 100, "out": 1000, "cache_read": 8900, "cost": 1.0}
RATIO_CASES = [
    ("wall m", {"wall": 1200.0}, {"wall": 1194.0}, 600.0, 1200.0),
    ("turns", {"turns": 200}, {"turns": 199}, 100, 200),
    ("out tok", {"out": 2000}, {"out": 1990}, 1000, 2000),
    ("total tok", {"cache_read": 18900}, {"cache_read": 18800}, 10000, 20000),
    ("cost $", {"cost": 2.0}, {"cost": 1.99}, 1.0, 2.0),
]


def ratio_runs(**override) -> list[Run]:
    t = {**BASE_TOTALS, **override}
    return [run_of(make_record(totals=totals(t["wall"], t["turns"], t["out"], t["cache_read"], t["cost"])))]


@pytest.mark.parametrize("label, doubled, under, base_median, cand_median", RATIO_CASES, ids=[c[0] for c in RATIO_CASES])
def test_ratio_of_exactly_two_is_flagged_and_just_under_is_not(label, doubled, under, base_median, cand_median):
    baseline = ratio_runs()

    assert report.regressions(baseline, ratio_runs(**doubled)) == [report.Flag(PLAN, label, base_median, cand_median)]
    assert report.regressions(baseline, ratio_runs(**under)) == []

    flagged = [line for line in report.compare(baseline, ratio_runs(**doubled)).splitlines() if line.startswith(label + " ")]
    assert len(flagged) == 1 and flagged[0].endswith("(x2.00)  ◀ regression")
    plain = [line for line in report.compare(baseline, ratio_runs(**under)).splitlines() if line.startswith(label + " ")]
    assert len(plain) == 1 and plain[0].endswith("(x1.99)")


def score_runs(*scores) -> list[Run]:
    return [run_of(make_record(checks={"passed": None, "total": 10, "score": s})) for s in scores]


@pytest.mark.parametrize(
    "base, cand, flagged",
    [
        (1.0, 0.9, True),  # 0.09999999999999998 in floating point: still a tenth
        (0.8, 0.7, True),  # 0.10000000000000009
        (0.3, 0.2, True),  # 0.09999999999999998
        (0.95, 0.85, True),
        (0.9, 0.5, True),
        (1.0, 0.91, False),
        (0.5, 0.41, False),
        (0.9, 0.9, False),
        (0.5, 1.0, False),  # a rise never flags
        (0.0, 0.9, False),
    ],
)
def test_check_score_drop_of_a_tenth_or_more_is_flagged(base, cand, flagged):
    flags = report.regressions(score_runs(base), score_runs(cand))

    assert flags == ([report.Flag(PLAN, "check score", base, cand)] if flagged else [])


def test_check_score_line_shows_the_signed_change():
    # The change column is as wide as "+0.00 (x1.00)", the other rows' change.
    text = report.compare(score_runs(1.0), score_runs(0.9))
    assert text.endswith("check score        1.00       0.90          -0.10  ◀ regression\n")

    text = report.compare(score_runs(0.5), score_runs(1.0))
    assert text.endswith("check score        0.50       1.00          +0.50\n")


def test_score_median_is_over_the_runs_of_each_set():
    # Baseline median 0.9 (mean 0.8), candidate median 0.85 (mean 0.6): no flag.
    assert report.regressions(score_runs(1.0, 0.9, 0.5), score_runs(0.0, 0.85, 0.95)) == []
    # Baseline median 0.95 of an even count, candidate 0.85: flagged.
    assert report.regressions(score_runs(1.0, 0.9), score_runs(0.85)) == [report.Flag(PLAN, "check score", 0.95, 0.85)]


UNKNOWN_BASELINE = """\
== plan-workflow (default) ==  baseline 3 run(s), candidate 3 run(s)
median         baseline  candidate   change
-------------  --------  ---------  -------
wall m          unknown       10.0  unknown
user wait m     unknown    unknown  unknown
helper wait m   unknown    unknown  unknown
gap m           unknown    unknown  unknown
turns           unknown         40  unknown
out tok         unknown      9,000  unknown
total tok       unknown    530,000  unknown
cost $          unknown       0.50  unknown
check score     unknown       1.00  unknown
"""


def null_compared_record():
    return make_record(checks={"passed": None, "total": None, "score": None}, totals=NULL_TOTALS)


def test_measure_unknown_in_one_set_prints_unknown_and_never_flags():
    unknown = [run_of(null_compared_record(), trial=f"plan-workflow__n{i}") for i in range(3)]
    known = [run_of(make_record(), trial=f"plan-workflow__k{i}") for i in range(3)]

    assert report.compare(unknown, known) == UNKNOWN_BASELINE
    assert report.regressions(unknown, known) == []
    assert report.regressions(known, unknown) == []
    assert "◀ regression" not in report.compare(known, unknown)
    assert "0.0" not in report.compare(known, unknown).replace("10.0", "")


def test_main_prints_unknown_for_an_all_null_baseline(tmp_path, capsys):
    for i in range(3):
        write_trial(tmp_path, "base", f"plan-workflow__n{i}", null_compared_record(), reward=1.0)
        write_trial(tmp_path, "cand", f"plan-workflow__k{i}", make_record(), reward=1.0)

    assert main(["--baseline", str(tmp_path / "base"), "--candidate", str(tmp_path / "cand")]) == 0

    assert capsys.readouterr().out == UNKNOWN_BASELINE + "\n"


def mixed_totals(wall, turns, inp, out, cost):
    return {
        "wall_time_s": wall,
        "model_time_s": None,
        "turns": turns,
        "cost_usd": cost,
        "input_tokens": inp,
        "output_tokens": out,
        "cache_read_tokens": 900,
        "cache_write_tokens": 0,
    }


def mixed_run(trial, score, wall, turns, inp, out, cost) -> Run:
    checks = {"passed": None, "total": None if score is None else 10, "score": score}
    return run_of(make_record(checks=checks, totals=mixed_totals(wall, turns, inp, out, cost)), trial=trial)


MIXED_BASELINE = """\
== plan-workflow (default) ==  baseline 3 run(s), candidate 3 run(s)
median         baseline  candidate          change
-------------  --------  ---------  --------------
wall m             10.8       20.8   +10.0 (x1.92)
user wait m     unknown    unknown         unknown
helper wait m   unknown    unknown         unknown
gap m           unknown    unknown         unknown
turns                20         20      +0 (x1.00)
out tok           1,000      1,000      +0 (x1.00)
total tok         2,000      4,000  +2,000 (x2.00)  ◀ regression
cost $             2.00       2.00   +0.00 (x1.00)
check score        0.90       0.90           +0.00
"""


def test_mixed_known_and_unknown_runs_take_the_median_of_the_known_ones():
    # Baseline: wall [-, 600, 700] -> 650 (not 600, as a missing value read as
    # zero would give, which would make the candidate's 1,250 a 2.08x flag);
    # turns [10, -, 30] -> 20; out [1,000, 1,000, -] -> 1,000; total tokens
    # known only for the second run, whose four parts are all known -> 2,000;
    # cost [-, 1.0, 3.0] -> 2.0; score [-, 1.0, 0.8] -> 0.9.
    # mixed_run(trial, score, wall, turns, input tokens, output tokens, cost)
    baseline = [
        mixed_run("plan-workflow__m1", None, None, 10, None, 1000, None),
        mixed_run("plan-workflow__m2", 1.0, 600.0, None, 100, 1000, 1.0),
        mixed_run("plan-workflow__m3", 0.8, 700.0, 30, 100, None, 3.0),
    ]
    # Candidate: total tokens [4,000, -, -] -> 4,000, exactly twice.
    candidate = [
        mixed_run("plan-workflow__k1", 0.9, 1250.0, 20, 2100, 1000, 2.0),
        mixed_run("plan-workflow__k2", 0.9, 1250.0, 20, None, 1000, 2.0),
        mixed_run("plan-workflow__k3", 0.9, 1250.0, 20, None, 1000, 2.0),
    ]

    assert report.compare(baseline, candidate) == MIXED_BASELINE
    assert report.regressions(baseline, candidate) == [report.Flag(PLAN, "total tok", 2000, 4000)]


ZERO_BASELINE = """\
== plan-workflow (default) ==  baseline 1 run(s), candidate 1 run(s)
median         baseline  candidate    change
-------------  --------  ---------  --------
wall m              0.0       10.0     +10.0
user wait m     unknown        0.0   unknown
helper wait m   unknown        0.0   unknown
gap m           unknown        0.0   unknown
turns                 0         40       +40
out tok               0      9,000    +9,000
total tok             0    509,100  +509,100
cost $             0.00       5.00     +5.00
check score        1.00       1.00     +0.00
"""


def test_zero_baseline_never_flags_a_ratio():
    zeros = {**dict.fromkeys(NULL_TOTALS, 0), "wall_time_s": 0.0, "model_time_s": None, "cost_usd": 0.0}
    baseline = [run_of(make_record(totals=zeros))]
    candidate = [run_of(make_record(totals=totals(600.0, 40, 9000, 500000, 5.0)))]

    assert report.regressions(baseline, candidate) == []
    assert report.compare(baseline, candidate) == ZERO_BASELINE


def test_scenario_groups_are_matched_separately(tmp_path, capsys):
    # Pooled, both sets would have wall medians of 900 s and 1,200 s: no flag.
    for job, trial, wall, scenario in [
        ("base", "repo-workflow__b1", 600.0, "default"),
        ("base", "repo-workflow__b2", 1200.0, "one-at-a-time"),
        ("cand", "repo-workflow__c1", 1200.0, "default"),
        ("cand", "repo-workflow__c2", 1200.0, "one-at-a-time"),
    ]:
        compared_trial(tmp_path, job, trial, wall, 10, 1000, 10000, 1.0, 1.0, suite="repo-workflow", scenario=scenario)

    assert main(["--baseline", str(tmp_path / "base"), "--candidate", str(tmp_path / "cand")]) == 0

    out = capsys.readouterr().out
    titles = [line for line in out.splitlines() if line.startswith("==")]
    assert titles == [
        "== repo-workflow (default) ==  baseline 1 run(s), candidate 1 run(s)",
        "== repo-workflow (one-at-a-time) ==  baseline 1 run(s), candidate 1 run(s)",
    ]
    walls = [line for line in out.splitlines() if line.startswith("wall m")]
    assert walls == [
        "wall m             10.0       20.0  +10.0 (x2.00)  ◀ regression",
        "wall m             20.0       20.0   +0.0 (x1.00)",
    ]
    assert "Unmatched" not in out
    assert report.regressions(load_runs([str(tmp_path / "base")]), load_runs([str(tmp_path / "cand")])) == [
        report.Flag(("repo-workflow", "default"), "wall m", 600.0, 1200.0)
    ]


def test_baseline_glob_pools_several_job_dirs_into_one_set(tmp_path, capsys):
    compared_trial(tmp_path, "base-1", "plan-workflow__b1", 600.0, 10, 1000, 10000, 1.0, 1.0)
    compared_trial(tmp_path, "base-1", "plan-workflow__b2", 660.0, 11, 1100, 11000, 1.1, 0.9)
    compared_trial(tmp_path, "base-2", "plan-workflow__b3", 3000.0, 50, 5000, 50000, 5.0, 0.5)
    compared_trial(tmp_path, "cand-1", "plan-workflow__c1", 1200.0, 5, 500, 10000, 2.0, 0.8)
    compared_trial(tmp_path, "cand-2", "plan-workflow__c2", 1320.0, 12, 900, 20000, 2.5, 0.7)
    compared_trial(tmp_path, "cand-2", "plan-workflow__c3", 1400.0, 100, 9000, 1000, 0.1, 1.0)

    assert main(["--baseline", f"{tmp_path}/base-*", "--candidate", str(tmp_path / "cand-1"), str(tmp_path / "cand-2")]) == 0

    captured = capsys.readouterr()
    assert captured.out == FULL_COMPARISON.split("\nUnmatched")[0] + "\n"
    assert captured.err == ""
    assert [r.trial for r in load_runs([f"{tmp_path}/base-*"])] == ["plan-workflow__b1", "plan-workflow__b2", "plan-workflow__b3"]


USAGE_ERROR = (
    "report: a comparison takes --baseline and --candidate together, each with job directories "
    "or globs, and no other paths or --run\n"
)


@pytest.mark.parametrize(
    "argv",
    [
        ["--baseline", "{base}"],
        ["--candidate", "{cand}"],
        ["--run", "plan-workflow__b1", "--baseline", "{base}", "--candidate", "{cand}"],
        ["{base}", "--baseline", "{base}", "--candidate", "{cand}"],
    ],
    ids=["baseline only", "candidate only", "with --run", "with a positional path"],
)
def test_comparison_usage_errors_exit_2(tmp_path, capsys, argv):
    base, cand = comparison_jobs(tmp_path)

    assert main([a.format(base=base, cand=cand) for a in argv]) == 2

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == USAGE_ERROR


def test_empty_candidate_set_exits_2(tmp_path, capsys):
    base, _ = comparison_jobs(tmp_path)
    empty = tmp_path / "old"
    write_trial(empty, "job", C3, None, started_at="2026-01-01T00:00:00Z", reward=1.0)

    assert main(["--baseline", str(base), "--candidate", str(empty)]) == 2

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == "report: the candidate set has no metrics records; pass job directories whose runs have one\n"


def test_empty_baseline_glob_exits_2(tmp_path, capsys):
    _, cand = comparison_jobs(tmp_path)
    pattern = f"{tmp_path}/nightly-*"

    assert main(["--baseline", pattern, "--candidate", str(cand)]) == 2

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == (
        f"report: {pattern} matches nothing\n"
        "report: the baseline set has no metrics records; pass job directories whose runs have one\n"
    )


# 7. Helpers, briefs and gap time, from a version-3 record.


def test_run_view_shows_the_split_the_totals_and_every_helper(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    assert main([str(jobs), "--run", A1]) == 0
    out = capsys.readouterr().out
    assert "  discovery           1    120.0    13%     40.0         80.0        0.0        1\n" in out
    assert "Totals: wall 948.4 s, user wait 30.0 s, helper wait 80.0 s, gaps over 300.0 s: 0.0 s (0), 0% of wall\n" in out
    assert "  research    20260101000000-user-auth  discovery           80.0  stopped\n" in out
    # an orchestrator's helper, started outside any run, still running when the logs end
    assert "  plan alpha  20260101000000-user-auth  outside any run      0.0  open*\n" in out


def test_prompts_prints_every_helpers_brief_after_the_breakdown(tmp_path, capsys):
    jobs = standard_jobs(tmp_path)
    assert main([str(jobs), "--run", A1, "--prompts"]) == 0
    out = capsys.readouterr().out
    assert out == A1_BREAKDOWN + "\n" + (
        "── toolu_01AbCdEfGh general-purpose Research the codebase (37 chars)\n"
        "Find every caller of the auth module.\n"
        "\n"
    )


def gap_run(gap, wall=1000.0):
    return run_of(make_record(totals=totals(wall, 10, 1, 1, 1.0, gap=gap, gaps=1)))


@pytest.mark.parametrize("gap, marked", [(260.0, True), (250.0, False), (240.0, False)])
def test_a_run_whose_gaps_take_over_a_quarter_of_its_wall_time_is_marked(gap, marked):
    run = gap_run(gap)
    assert report.is_gappy(run) is marked
    assert ("◀ gaps" in report.table([run])) is marked
    assert ("◀ gaps" in breakdown(run)) is marked


def test_unknown_gap_time_is_never_marked():
    run = run_of(make_record())  # a version-1 total: no gap_s
    assert report.gap_share(run) is None
    assert not report.is_gappy(run)
    assert "gaps over unknown s: unknown s (unknown)" in breakdown(run)


def v2_record():
    """A version-2 record: per-step characters and child agents, the
    longest idle stretch, and none of version 3's figures."""
    record = make_record(
        totals={**totals(600.0, 10, 1, 1, 1.0), "idle_s": 120.0},
        steps={"unit": "characters", "workflows": [{"kind": "plan", "name": NAME, "steps": [
            {"step": "discovery", "started_at": None, "duration_s": 300.0, "output": 900, "children": 1}]}]},
        subagents={"started": 1, "completed": 1, "failed": 0, "stopped": 0, "durations_s": [12.0],
                   "children": [{"id": "toolu_x", "prompt": "old", "prompt_chars": 3}]},
    )
    record["schema_version"] = 2
    for key in ("user_wait_s", "helper_wait_s", "gap_s", "gaps", "gap_threshold_s"):
        del record["totals"][key]
    return record


def test_a_version_2_record_loads_with_the_new_figures_unknown(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    write_trial(jobs, "old", "plan-workflow__v2", v2_record(), reward=1.0)
    assert main([str(jobs)]) == 0
    row = capsys.readouterr().out.splitlines()[3]
    assert row.split()[3:6] == ["10.0", "unknown", "unknown"]
    assert main([str(jobs), "--run", "plan-workflow__v2", "--prompts"]) == 0
    out = capsys.readouterr().out
    assert "  discovery  unknown    300.0    50%  unknown      unknown    unknown  unknown  ◀ dominant\n" in out
    assert "user wait unknown s, helper wait unknown s" in out
    assert "Helper spans: not recorded (schema 2)\n" in out
    assert "No briefs recorded (schema 2).\n" in out
    assert "idle" not in out and "children" not in out.lower()


def test_unknown_helpers_and_briefs_say_so():
    record = make_record(subagents={**dict.fromkeys(["started", "completed", "failed", "unfinished", "durations_s"]), "helpers": None, "briefs": None})
    run = run_of(record)
    assert "Helper spans: unknown (see below)\n" in breakdown(run)
    assert report.prompts(run) == "Briefs: unknown (see the breakdown's Unknown list)."


def test_medians_include_user_wait_helper_wait_and_gap_time():
    labels = [m.label for m in report.MEASURES]
    assert labels[:4] == ["wall m", "user wait m", "helper wait m", "gap m"]
    base = [run_of(make_record(totals=totals(600.0, 10, 1, 1, 1.0, user=60.0, helper=120.0, gap=0.0)), trial=f"plan-workflow__b{i}") for i in range(3)]
    cand = [run_of(make_record(totals=totals(600.0, 10, 1, 1, 1.0, user=180.0, helper=120.0, gap=600.0)), trial=f"plan-workflow__c{i}") for i in range(3)]
    text = report.compare(base, cand)
    assert "user wait m         1.0        3.0   +2.0 (x3.00)  ◀ regression\n" in text
    assert "helper wait m       2.0        2.0   +0.0 (x1.00)\n" in text
    # a zero baseline never flags a ratio
    assert "gap m               0.0       10.0          +10.0\n" in text
    assert [f.measure for f in report.regressions(base, cand)] == ["user wait m"]


# 8. Two trials side by side.


def trial_record(steps, user, helper, gap, cost, name=NAME):
    return make_record(
        totals=totals(sum(s["duration_s"] for s in steps), 10, 1000, 10000, cost, user=user, helper=helper, gap=gap),
        steps={"workflows": [{"kind": "plan", "name": name, "status": "finished", "steps": steps}]},
    )


def compare_jobs(tmp_path):
    jobs = tmp_path / "jobs"
    a = trial_record(
        [step("discovery", None, 120.0, helper=80.0, helpers=1), step("architecture", None, 200.0, user=50.0)],
        user=50.0, helper=80.0, gap=0.0, cost=1.0,
    )
    b = trial_record(
        [step("discovery", None, 90.0, helper=30.0, helpers=1), step("architecture", None, 260.0, user=20.0), step("walkthrough", None, 30.0)],
        user=20.0, helper=30.0, gap=310.0, cost=1.5, name="20260101000001-user-auth",
    )
    write_trial(jobs, "job", "plan-workflow__ta", a, reward=1.0)
    write_trial(jobs, "job", "plan-workflow__tb", b, reward=1.0)
    return jobs


COMPARE_TRIALS = """\
== plan-workflow__ta (A) against plan-workflow__tb (B) ==

plan 20260101000000-user-auth (A) / plan 20260101000001-user-auth (B)
  step          secs A  secs B  secs B-A  work A  work B  work B-A  helper A  helper B  helper B-A  user A  user B  user B-A  helpers A  helpers B  helpers B-A
  ------------  ------  ------  --------  ------  ------  --------  --------  --------  ----------  ------  ------  --------  ---------  ---------  -----------
  discovery      120.0    90.0     -30.0    40.0    60.0     +20.0      80.0      30.0       -50.0     0.0     0.0      +0.0          1          1           +0
  architecture   200.0   260.0     +60.0   150.0   240.0     +90.0       0.0       0.0        +0.0    50.0    20.0     -30.0          0          0           +0
  walkthrough        -    30.0         -       -    30.0         -         -       0.0           -       -     0.0         -          -          0            -

Totals
  total               A       B     B-A
  -------------  ------  ------  ------
  wall s          320.0   380.0   +60.0
  user wait s      50.0    20.0   -30.0
  helper wait s    80.0    30.0   -50.0
  gap s             0.0   310.0  +310.0
  cost $           1.00    1.50   +0.50
  total tok      11,100  11,100      +0
"""


def test_compare_shows_each_trials_split_and_totals_and_bs_difference(tmp_path, capsys):
    jobs = compare_jobs(tmp_path)
    assert main([str(jobs), "--compare", "plan-workflow__ta", "plan-workflow__tb"]) == 0
    assert capsys.readouterr().out == COMPARE_TRIALS + "\n"


def test_a_step_only_one_trial_entered_is_absent_not_zero(tmp_path):
    jobs = compare_jobs(tmp_path)
    runs = load_runs([str(jobs)])
    text = report.compare_trials(find_run(runs, "plan-workflow__ta"), find_run(runs, "plan-workflow__tb"))
    walkthrough = next(l for l in text.splitlines() if l.strip().startswith("walkthrough")).split()
    assert walkthrough[1] == "-" and walkthrough[3] == "-"
    assert "0.0" not in walkthrough[1:4:2]


def test_compare_by_trial_directory_and_old_records(tmp_path, capsys):
    jobs = compare_jobs(tmp_path)
    write_trial(jobs, "old", "plan-workflow__v2", v2_record(), reward=1.0)
    assert main(["--compare", str(jobs / "job" / "plan-workflow__ta"), str(jobs / "old" / "plan-workflow__v2")]) == 0
    out = capsys.readouterr().out
    discovery = next(l for l in out.splitlines() if l.strip().startswith("discovery")).split()
    # the version-2 record has the step and its duration, but none of the split
    assert discovery[:4] == ["discovery", "120.0", "300.0", "+180.0"]
    assert discovery[5] == "unknown" and discovery[6] == "unknown"
    assert "  user wait s      50.0  unknown  unknown\n" in out


@pytest.mark.parametrize("extra", [["--run", "x"], ["--baseline", "a", "--candidate", "b"]])
def test_compare_with_another_mode_is_a_usage_error(tmp_path, capsys, extra):
    assert main(["--compare", "a", "b", *extra]) == 2
    assert "--compare takes two trials" in capsys.readouterr().err


def test_compare_of_an_unknown_trial_exits_2(tmp_path, capsys):
    jobs = compare_jobs(tmp_path)
    assert main([str(jobs), "--compare", "plan-workflow__ta", "plan-workflow__zz"]) == 2
    assert "no recorded run matches 'plan-workflow__zz'" in capsys.readouterr().err
