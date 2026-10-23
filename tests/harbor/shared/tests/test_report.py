"""Self-check for the local run report in report.py.

The report reads the metrics record (verifier/metrics.json) and harbor's
result.json of every trial under the paths it is given, and nothing else. It
prints one row per recorded run grouped by suite and scenario, or one run's
step breakdown with `--run`, flags a step taking more than a third of its
run's wall time, and prints a value it cannot read as `unknown`, never as
zero. Each case here builds a small jobs tree by hand under tmp_path, with
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

# The reference plan run's steps: wall 948.39 s, assemble 368.8 s (39%).
REFERENCE_STEPS = {
    "unit": "characters",
    "workflows": [
        {
            "kind": "plan",
            "name": NAME,
            "steps": [
                {"step": "discovery", "started_at": "2026-10-20T10:00:05Z", "duration_s": 120.0, "output": 4210},
                {"step": "architecture", "started_at": "2026-10-20T10:02:05Z", "duration_s": 210.5, "output": 9876},
                {"step": "assemble", "started_at": "2026-10-20T10:05:36Z", "duration_s": 368.8, "output": 31502},
                {"step": "verification", "started_at": "2026-10-20T10:11:44Z", "duration_s": 45.3, "output": None},
            ],
        }
    ],
    "orchestrator_output": 1500,
}

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
        "schema_version": 1,
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
        "steps": steps if steps is not None else {"unit": "characters", "workflows": []},
        "subagents": subagents
        if subagents is not None
        else {"started": 0, "completed": 0, "failed": 0, "stopped": 0, "durations_s": []},
        "unknown": unknown if unknown is not None else [],
    }


def totals(wall, turns, out, cache_read, cost):
    return {
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
            totals=totals(948.39, 87, 41230, 1234567, 4.12),
            steps=REFERENCE_STEPS,
            subagents={"started": 3, "completed": 2, "failed": 1, "stopped": 0, "durations_s": [12.5, 40.0, 7.3]},
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
            totals=totals(1500.0, 120, 60000, 2000000, 7.0),
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
trial                 agent        model              wall m  turns  out tok  cache-read tok  cost $  checks        result
--------------------  -----------  -----------------  ------  -----  -------  --------------  ------  ------------  ------
plan-workflow__e5Qk2  bob          claude-haiku-4-5     10.0     40    9,000         500,000    0.50  12/12 (1.00)  pass
plan-workflow__a1Xy9  claude-code  claude-sonnet-4-6    15.8     87   41,230       1,234,567    4.12  11/12 (0.92)  pass
plan-workflow__b2Lm4  claude-code  claude-sonnet-4-6    25.0    120   60,000       2,000,000    7.00  6/12 (0.50)   fail
"""

SPEC_GROUP = """\
== spec-workflow ==
trial                 agent  model             wall m  turns  out tok  cache-read tok  cost $  checks      result
--------------------  -----  ----------------  ------  -----  -------  --------------  ------  ----------  ------
spec-workflow__d4Rt7  bob    claude-haiku-4-5     5.5     22    3,100          75,000    0.25  4/8 (0.50)  fail
"""

A1_BREAKDOWN = """\
== plan-workflow__a1Xy9 ==
plan-workflow (default), claude-code on claude-sonnet-4-6, pass
wall 15.8 m, turns 87, cost $4.12, checks 11/12 (0.92)

plan 20260101000000-user-auth
  step          seconds  share  characters
  ------------  -------  -----  ----------
  discovery       120.0    13%       4,210
  architecture    210.5    22%       9,876
  assemble        368.8    39%      31,502  ◀ dominant
  verification     45.3     5%     unknown

Orchestrator output, outside every lane: 1,500 characters

Sub-agents: started 3, completed 2, failed 1, stopped 0; durations (s): 12.5, 40.0, 7.3
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


# BUG: a metrics.json that parses but is not a JSON object (e.g. `[]`) is
# skipped silently. read_json returns None for a non-dict without calling
# warn(), so the run vanishes with nothing on stderr. Input: a trial whose
# verifier/metrics.json is `[]`; expected a "report: skipping ..." warning;
# actual: stderr is empty.
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


# BUG: `--run <trial dir>` without positional paths only searches the default
# tests/harbor/jobs, so a trial directory anywhere else is never found, even
# though the docstring's usage line is `report.py --run <trial dir or trial
# name>`. Input: main(["--run", "<tmp>/jobs/fix-plan-2026-10-20/plan-workflow__a1Xy9"]);
# expected: exit 0 with that run's breakdown; actual: exit 2, "no recorded run matches".
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
    assert "  overview         60.0    60%         100  ◀ dominant" in text
    assert "  requirements     30.0    30%         200\n" in text
    assert "  discovery     10.0    10%         300\n" in text


# 4. Unknown values.


def null_record():
    return make_record(
        model=None,
        reward=None,
        checks={"passed": None, "total": None, "score": None},
        totals=NULL_TOTALS,
        steps={
            "unit": "characters",
            "workflows": [
                {
                    "kind": "plan",
                    "name": NAME,
                    "steps": [{"step": "discovery", "started_at": None, "duration_s": None, "output": None}],
                }
            ],
            "orchestrator_output": None,
        },
        subagents=dict.fromkeys(["started", "completed", "failed", "stopped", "durations_s"]),
        unknown=[{"field": "totals", "reason": "no agent log"}],
    )


NULL_BREAKDOWN = """\
== plan-workflow__n0Ul1 ==
plan-workflow (default), claude-code on unknown, unknown
wall unknown m, turns unknown, cost $unknown, checks unknown

plan 20260101000000-user-auth
  step       seconds    share  characters
  ---------  -------  -------  ----------
  discovery  unknown  unknown     unknown

Orchestrator output, outside every lane: unknown characters

Sub-agents: started unknown, completed unknown, failed unknown, stopped unknown; durations (s): unknown

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
        "plan-workflow__n0Ul1  claude-code  unknown  unknown  unknown  unknown"
        "         unknown  unknown  unknown  unknown"
    )
    assert "0.0" not in lines[3] and " 0 " not in lines[3]


def test_unknown_values_print_as_unknown_in_the_breakdown(tmp_path, capsys):
    jobs = tmp_path / "jobs"
    write_trial(jobs, "job", "plan-workflow__n0Ul1", null_record(), result=False)

    assert main([str(jobs), "--run", "plan-workflow__n0Ul1"]) == 0

    assert capsys.readouterr().out == NULL_BREAKDOWN + "\n"


def test_null_checks_section_and_unknown_workflows_print_as_unknown():
    record = make_record(steps={"unit": "characters", "workflows": None})
    record["result"]["checks"] = None
    record["unknown"] = [{"field": "steps", "reason": "no session log"}]

    text = breakdown(run_of(record, trial="plan-workflow__w1", reward=None))

    assert "plan-workflow (default), claude-code on claude-sonnet-4-6, unknown\n" in text
    assert "wall 10.0 m, turns 40, cost $0.50, checks unknown\n" in text
    assert "Steps: unknown (see below)\n" in text
    assert "  steps: no session log\n" in text


def test_null_subagent_durations_alone_print_as_unknown_and_empty_as_none():
    record = make_record(subagents={"started": 2, "completed": 2, "failed": 0, "stopped": 0, "durations_s": None})
    assert (
        "Sub-agents: started 2, completed 2, failed 0, stopped 0; durations (s): unknown\n"
        in breakdown(run_of(record))
    )

    record = make_record()  # no sub-agents: an empty list, which is known
    assert (
        "Sub-agents: started 0, completed 0, failed 0, stopped 0; durations (s): none\n"
        in breakdown(run_of(record))
    )
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
