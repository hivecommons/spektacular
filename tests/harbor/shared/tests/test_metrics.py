"""Self-check for the run metrics recorder in metrics.py (record schema 3).

The recorder reads a finished harbor trial (build info, the agent's own log,
Spektacular's logs through `spektacular stats`, Bob's shell logs, pytest's
JUnit XML and the reward) and writes one metrics record in the same shape for
every agent and suite. Every step, helper, wait and gap comes from the logs,
the same way for every agent; tokens, models, cost and the helpers' briefs
from the agent's log. A measure a source does not supply is null and is named
under `unknown`, and a log-sourced one is never filled in from a transcript;
a missing or broken input costs only its own fields; `main` always returns 0
and never writes the reward.

Each case builds a small trial tree by hand under tmp_path and runs the
recorder over it, mostly through `main` exactly as test.sh does. The run log
reader is fed canned `stats` answers through its injectable runner: the spec
and epic trials of runlog_fixtures.py (which test_runlog_integration.py checks
against a real binary) and the Go golden report under
internal/runstats/testdata, so no binary is needed here.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_metrics.py
"""

import copy
import json
import os
import subprocess
import sys
from datetime import datetime
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import metrics  # noqa: E402
import runlog_fixtures as fx  # noqa: E402
from metrics import Paths, build_record, main, score_checks, write_record  # noqa: E402

ROOT = HERE.parents[3]
GOLDEN_REPORT = ROOT / "internal" / "runstats" / "testdata" / "report.golden.json"

LANE_A = "20261001-user-auth"
LANE_B = "20261001-user-auth-api"  # contains LANE_A: the longer name must win
SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"

TOP_KEYS = ["schema_version", "run", "result", "totals", "steps", "subagents", "unknown"]
RUN_KEYS = ["suite", "scenario", "agent", "model", "spektacular_version", "commit", "dirty"]
RESULT_KEYS = ["reward", "checks", "groups"]
# The totals the agent's own log gives.
TOTALS_KEYS = [
    "wall_time_s",
    "model_time_s",
    "turns",
    "cost_usd",
    "input_tokens",
    "output_tokens",
    "cache_read_tokens",
    "cache_write_tokens",
]
# The totals Spektacular's logs give.
LOG_TOTALS_KEYS = ["trace_calls", "trace_calls_per_task", "user_wait_s", "helper_wait_s", "gap_s", "gaps", "gap_threshold_s"]
RECORD_TOTALS_KEYS = [*TOTALS_KEYS, *LOG_TOTALS_KEYS]
# The helper counts and list Spektacular's logs give; briefs come from the agent's log.
HELPER_COUNT_KEYS = ["started", "completed", "failed", "unfinished", "durations_s", "helpers"]
RECORD_SUBAGENT_KEYS = [*HELPER_COUNT_KEYS, "briefs"]
WORKFLOW_KEYS = ["run_id", "kind", "name", "status", "wall_s", "user_wait_s", "helper_wait_s", "gap_s", "gaps", "steps"]
STEP_KEYS = [
    "step", "started_at", "entries", "duration_s", "working_s", "helper_wait_s", "user_wait_s",
    "commands", "helpers", "incomplete",
]  # fmt: skip
HELPER_KEYS = [
    "span_id", "parent", "name", "spec", "kind", "step", "task_id", "outside_run",
    "started_at", "ended_at", "duration_s", "end_reason", "status", "complete",
]  # fmt: skip
BRIEF_KEYS = ["id", "parent", "kind", "description", "models", "prompt", "prompt_chars"]
# Every log-sourced field listed unknown when the logs cannot answer, in order.
LOG_FIELDS = [
    "steps", "subagents.started", "subagents.helpers",
    "totals.user_wait_s", "totals.helper_wait_s", "totals.gap_s", "totals.trace_calls",
]  # fmt: skip


def build_info(agent):
    return {
        "suite": "plan-workflow",
        "scenario": "default",
        "agent": agent,
        "spektacular_version": "0.42.0",
        "commit": "abc1234",
        "dirty": False,
    }


# Seven test cases: two classes, one function-style module. One skipped, one
# failure, one error, so 4 of 6 counted checks pass.
JUNIT = """<?xml version="1.0" encoding="utf-8"?>
<testsuites name="pytest tests">
  <testsuite name="pytest" errors="1" failures="1" skipped="1" tests="7">
    <testcase classname="test_plan_workflow.TestStepOrder" name="test_overview_first" time="0.001" />
    <testcase classname="test_plan_workflow.TestStepOrder" name="test_finished_last" time="0.001">
      <failure message="assert 'tasks' == 'finished'">AssertionError</failure>
    </testcase>
    <testcase classname="test_plan_workflow.TestPlanFile" name="test_written" time="0.001" />
    <testcase classname="test_plan_workflow.TestPlanFile" name="test_sections" time="0.001">
      <error message="fixture failed">FileNotFoundError</error>
    </testcase>
    <testcase classname="test_plan_workflow.TestPlanFile" name="test_optional" time="0.000">
      <skipped type="pytest.skip" message="not applicable">skipped</skipped>
    </testcase>
    <testcase classname="test_goal_check" name="test_goal_met" time="0.001" />
    <testcase classname="test_goal_check" name="test_goal_named" time="0.001" />
  </testsuite>
</testsuites>
"""

CHECKS = {"passed": 4, "total": 6, "score": 4 / 6}
GROUPS = [
    {"name": "TestStepOrder", "passed": 1, "total": 2, "score": 1 / 2},
    {"name": "TestPlanFile", "passed": 1, "total": 2, "score": 1 / 2},
    {"name": "test_goal_check", "passed": 2, "total": 2, "score": 1.0},
]


# --- Spektacular's logs, through canned `stats` answers -------------------------


class Done:
    def __init__(self, stdout: str):
        self.stdout = stdout


class Stats:
    """A `spektacular stats` stand-in for the run log reader's runner: each
    subcommand's canned answer, `run` by its --run id. `fail` makes every
    call (or only calls to `fail_on`) raise or print what it holds."""

    def __init__(self, answers: dict):
        self.answers = answers
        self.fail = None
        self.fail_on = None
        self.calls: list[list[str]] = []

    def __call__(self, argv, **kw):
        self.calls.append(argv)
        sub = argv[2]
        if self.fail is not None and self.fail_on in (None, sub):
            if isinstance(self.fail, BaseException):
                raise self.fail
            return Done(self.fail if isinstance(self.fail, str) else json.dumps(self.fail))
        if sub == "run":
            return Done(json.dumps(self.answers["run"][argv[argv.index("--run") + 1]]))
        return Done(json.dumps(self.answers[sub]))


# The reader itself, captured before the `stats` fixture stands in for it.
REAL_RUNLOG = metrics.runlog.RunLog


def spec_answers() -> dict:
    """The spec trial: one spec run with a user wait in overview and a helper
    in discovery."""
    return {**fx.spec_stats(), "run": {fx.TRACE_S: fx.spec_report()}}


def golden_report() -> dict:
    return json.loads(GOLDEN_REPORT.read_text())


GOLDEN_RUN = "9a1b2c3d000000000000000000000075"
GOLDEN_NAME = "000075_run-stats-command"


def golden_span(h: dict) -> dict:
    """A `stats spans` row for one of the golden report's helpers."""
    return {
        **{k: h[k] for k in ("span_id", "parent_span_id", "name", "step", "task_id", "started_at", "ended_at",
                             "duration_ms", "end_reason", "status", "message", "complete")},
        "type": "sub-agent", "trace_id": GOLDEN_RUN, "run_id": GOLDEN_RUN, "kind": "implement", "workflow": GOLDEN_NAME,
    }  # fmt: skip


def two_run_answers() -> dict:
    """The spec trial and the Go golden implement run (09:00 to 09:30, two
    tasks, two helpers, one never stopped), listed newest first."""
    answers = spec_answers()
    rep = golden_report()
    answers["list"]["runs"].append({
        "run_id": GOLDEN_RUN, "kind": "implement", "name": GOLDEN_NAME, "agent": "claude",
        "started_at": "2026-10-25T09:00:00Z", "ended_at": "2026-10-25T09:30:00Z", "status": "finished",
    })  # fmt: skip
    answers["spans"]["spans"] = [golden_span(h) for h in rep["helpers"]] + answers["spans"]["spans"]
    answers["run"][GOLDEN_RUN] = rep
    return answers


def span_row(span_id, name, start, end, *, complete=True, status="ok", reason="stopped") -> dict:
    """An orchestrator's `sub-agent` span outside any run, as `stats spans` gives it."""
    return {
        "span_id": span_id, "parent_span_id": None, "name": name, "step": None, "task_id": None,
        "started_at": fx.stamp(fx.at(start)), "ended_at": fx.stamp(fx.at(end)), "end_reason": reason,
        "status": status, "message": None, "complete": complete, "type": "sub-agent",
        "trace_id": f"{span_id:0>31}f", "run_id": None, "kind": None, "workflow": None,
    }  # fmt: skip


def epic_answers() -> dict:
    """The epic trial: two plan lanes launched by an orchestrator, beta
    resumed by a second child, alpha's discovery starting a helper. Two more
    orchestrator helpers launch no child: `oc` runs while alpha's first run
    starts, `od` after both lanes ended."""
    answers = fx.stats()
    ends = {fx.TRACE_A: fx.stamp(fx.at(15.1)), fx.TRACE_B: fx.stamp(fx.at(16.1))}
    for r in answers["list"]["runs"]:
        r["ended_at"] = ends[r["run_id"]]
    answers["spans"]["spans"] += [span_row("oc", "explore", 1.8, 2.2), span_row("od", "tidy", 21, 21.5)]
    answers["run"] = fx.reports()
    return answers


@pytest.fixture(autouse=True)
def stats(monkeypatch) -> Stats:
    """Every RunLog the recorder builds asks this stand-in, the spec trial by
    default, so no test depends on what `spektacular` is on PATH."""
    fake = Stats(spec_answers())
    monkeypatch.setattr(
        metrics.runlog, "RunLog", lambda log_dir, binary=None, runner=None: REAL_RUNLOG(log_dir, binary="spektacular", runner=fake)
    )
    return fake


# What the recorder makes of the spec trial.

SPEC_STEPS = [
    {"step": "overview", "started_at": "2026-10-25T10:00:06Z", "entries": 1, "duration_s": 294.0, "working_s": 174.0,
     "helper_wait_s": 0.0, "user_wait_s": 120.0, "commands": 3, "helpers": 0, "incomplete": 0},
    {"step": "discovery", "started_at": "2026-10-25T10:05:06Z", "entries": 1, "duration_s": 414.0, "working_s": 234.0,
     "helper_wait_s": 180.0, "user_wait_s": 0.0, "commands": 3, "helpers": 1, "incomplete": 0},
    {"step": "finished", "started_at": "2026-10-25T10:12:06Z", "entries": 1, "duration_s": 0.0, "working_s": 0.0,
     "helper_wait_s": 0.0, "user_wait_s": 0.0, "commands": 0, "helpers": 0, "incomplete": 0},
]  # fmt: skip
SPEC_WORKFLOW = {
    "run_id": fx.TRACE_S, "kind": "spec", "name": fx.GAMMA, "status": "finished",
    "wall_s": 726.0, "user_wait_s": 120.0, "helper_wait_s": 180.0, "gap_s": 0.0, "gaps": 0, "steps": SPEC_STEPS,
}  # fmt: skip
SPEC_HELPER = {
    "span_id": "hs1", "parent": "ss2", "name": "research", "spec": fx.GAMMA, "kind": "spec", "step": "discovery",
    "task_id": None, "outside_run": False, "started_at": "2026-10-25T10:06:00Z", "ended_at": "2026-10-25T10:09:00Z",
    "duration_s": 180.0, "end_reason": "stopped", "status": "ok", "complete": True,
}  # fmt: skip
# Four `trace` calls (start and stop of the user wait and of the helper), no task.
SPEC_LOG_TOTALS = {
    "trace_calls": 4, "trace_calls_per_task": None,
    "user_wait_s": 120.0, "helper_wait_s": 180.0, "gap_s": 0.0, "gaps": 0, "gap_threshold_s": 300.0,
}  # fmt: skip
SPEC_HELPERS = {"started": 1, "completed": 1, "failed": 0, "unfinished": 0, "durations_s": [180.0], "helpers": [SPEC_HELPER]}
SPEC_WALL_S = 726.0  # the run's 10:00:00 to 10:12:06, for agents whose log has no wall time


# --- the trial tree -----------------------------------------------------------


def trial(tmp_path, *, agent="claude", build="default", junit=JUNIT, reward="1\n") -> Paths:
    """A trial tree laid out like a collected harbor trial. build is the
    build.json object, raw text, None for no file, or "default". The logs
    directory exists; its answers come from the `stats` fixture."""
    paths = Paths(
        build_info=tmp_path / "tests" / "build.json",
        agent_dir=tmp_path / "agent",
        log_dir=tmp_path / "artifacts" / "spektacular" / "logs",
        bob_logs=tmp_path / "bob-logs",  # absent unless a test writes it
        junit=tmp_path / "verifier" / "junit.xml",
        reward=tmp_path / "verifier" / "reward.txt",
        out=tmp_path / "verifier" / "metrics.json",
    )
    for d in (paths.build_info.parent, paths.agent_dir, paths.log_dir, paths.junit.parent):
        d.mkdir(parents=True)
    if build == "default":
        build = build_info(agent)
    if isinstance(build, dict):
        paths.build_info.write_text(json.dumps(build))
    elif isinstance(build, str):
        paths.build_info.write_text(build)
    if junit is not None:
        paths.junit.write_text(junit)
    if reward is not None:
        paths.reward.write_text(reward)
    return paths


def args_of(paths: Paths, log_flag="--log-dir") -> list[str]:
    return [
        "--build-info", str(paths.build_info),
        "--agent-dir", str(paths.agent_dir),
        log_flag, str(paths.log_dir),
        "--bob-logs", str(paths.bob_logs),
        "--junit", str(paths.junit),
        "--reward", str(paths.reward),
        "--out", str(paths.out),
    ]  # fmt: skip


def reward_state(paths: Paths):
    if not paths.reward.exists():
        return None
    return paths.reward.read_bytes(), paths.reward.stat().st_mtime_ns


def run_main(paths: Paths, log_flag="--log-dir") -> dict:
    """Runs the recorder as test.sh does, checks it returned 0 and left the
    reward untouched, and returns the record it wrote."""
    before = reward_state(paths)
    assert main(args_of(paths, log_flag)) == 0
    assert reward_state(paths) == before
    return json.loads(paths.out.read_text())


def unknown_fields(record) -> list[str]:
    return [u["field"] for u in record["unknown"]]


def reason_of(record, name) -> str:
    return next(u["reason"] for u in record["unknown"] if u["field"] == name)


def write_jsonl(path: Path, objs) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))


# --- agent logs ---------------------------------------------------------------


def claude_ts(clock: str) -> str:
    return f"2026-10-23T{clock}Z"


def text(s):
    return {"type": "text", "text": s}


def assistant(clock, *content, parent=None, model=SONNET):
    return {
        "type": "assistant",
        "timestamp": claude_ts(clock),
        "parent_tool_use_id": parent,
        "message": {"model": model, "role": "assistant", "content": list(content)},
    }


def launch(tid, subagent_type, prompt, description=None):
    inp = {"subagent_type": subagent_type, "prompt": prompt}
    if description is not None:
        inp["description"] = description
    return {"type": "tool_use", "id": tid, "name": "Agent", "input": inp}


def epoch_ms(clock: str) -> int:
    """A clock time on the test day as epoch milliseconds, as task_updated writes it."""
    return int(datetime.fromisoformat(f"2026-10-23T{clock}+00:00").timestamp() * 1000)


def task_started(tool_use_id, task_id, *, background, depth=1, parent_task=None):
    """Claude's task_started event: no timestamp, as in real logs."""
    event = {
        "type": "system",
        "subtype": "task_started",
        "tool_use_id": tool_use_id,
        "task_id": task_id,
        "task_type": "local_agent",
        "is_backgrounded": background,
        "spawn_depth": depth,
    }
    if parent_task is not None:
        event["parent_task_id"] = parent_task
    return event


def task_updated(task_id, clock):
    """Claude's task_updated event: only the task id and the patch."""
    return {
        "type": "system",
        "subtype": "task_updated",
        "task_id": task_id,
        "patch": {"status": "completed", "end_time": epoch_ms(clock)},
    }


def task_notification(tool_use_id, task_id, duration_ms, summary):
    return {
        "type": "system",
        "subtype": "task_notification",
        "tool_use_id": tool_use_id,
        "task_id": task_id,
        "status": "completed",
        "summary": summary,
        "usage": {"duration_ms": duration_ms},
    }


def claude_result(spawned=0, completed=0):
    return {
        "type": "result",
        "subtype": "success",
        "num_turns": 12,
        "duration_ms": 400000,
        "duration_api_ms": 250000,
        "total_cost_usd": 1.5,
        "modelUsage": {
            SONNET: {
                "inputTokens": 100,
                "outputTokens": 2000,
                "cacheReadInputTokens": 30000,
                "cacheCreationInputTokens": 4000,
                "costUSD": 1.5,
            }
        },
        "subagent_stats": {
            "spawned": spawned,
            "completed": completed,
            "failed": 0,
            "killed": {"parent": 0, "user": 0, "system": 0},
        },
    }


# The totals the Claude log itself reports.
CLAUDE_TOTALS = {
    "wall_time_s": 400.0,
    "model_time_s": 250.0,
    "turns": 12,
    "cost_usd": 1.5,
    "input_tokens": 100,
    "output_tokens": 2000,
    "cache_read_tokens": 30000,
    "cache_write_tokens": 4000,
}


def claude_plan_log():
    """A single-agent Claude run that launches no sub-agent."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        assistant("09:59:50", text("hello")),
        assistant("10:00:30", text("a" * 10)),
        assistant("10:02:00", text("b" * 25)),
        assistant("10:05:00", text("c" * 40)),
        assistant("10:06:00.200", text("d" * 5)),
        claude_result(),
    ]


def claude_two_lane_log():
    """An orchestrator launching one background sub-agent per lane; A's
    sub-agent launches a nested foreground one whose prompt names no spec.
    Each child's start, end and hand-back are logged as real Claude logs
    them, and the result reports three sub-agents spawned."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        assistant(
            "10:00:10",
            launch("toolu_A", "general-purpose", f"plan {LANE_A}", "Plan A"),
            launch("toolu_B", "general-purpose", f"plan {LANE_B}", "Plan B"),
        ),
        task_started("toolu_A", "task_a", background=True),
        task_started("toolu_B", "task_b", background=True),
        assistant("10:00:30", text("a" * 20), parent="toolu_A", model=HAIKU),
        assistant("10:01:30", text("b" * 50), parent="toolu_B", model=HAIKU),
        assistant("10:01:40", text("x" * 11), parent="toolu_X", model=HAIKU),  # no known launch
        assistant("10:02:30", text("a" * 30), parent="toolu_A", model=HAIKU),
        assistant("10:03:00", launch("toolu_N", "Explore", "look around", "Look around"), parent="toolu_A", model=HAIKU),
        task_started("toolu_N", "task_n", background=False, depth=2, parent_task="task_a"),
        assistant("10:04:00", text("n" * 40), parent="toolu_N", model=HAIKU),
        assistant("10:05:30", text("m" * 13)),
        assistant("10:06:00.500", text("b" * 7), parent="toolu_B", model=HAIKU),
        assistant("10:07:00", text("a" * 9), parent="toolu_A", model=HAIKU),
        task_updated("task_n", "10:04:01"),
        task_notification("toolu_N", "task_n", 61000, "\nFound two specs.\nBoth are plans."),
        task_updated("task_a", "10:07:05"),
        task_notification("toolu_A", "task_a", 415000, "Plan A written."),
        task_updated("task_b", "10:06:40"),
        task_notification("toolu_B", "task_b", 390000, "Plan B written."),
        claude_result(spawned=3, completed=3),
    ]


CLAUDE_TWO_LANE_BRIEFS = [
    {"id": "toolu_A", "parent": None, "kind": "general-purpose", "description": "Plan A", "models": [HAIKU],
     "prompt": f"plan {LANE_A}", "prompt_chars": 23},
    {"id": "toolu_B", "parent": None, "kind": "general-purpose", "description": "Plan B", "models": [HAIKU],
     "prompt": f"plan {LANE_B}", "prompt_chars": 27},
    {"id": "toolu_N", "parent": "toolu_A", "kind": "Explore", "description": "Look around", "models": [HAIKU],
     "prompt": "look around", "prompt_chars": 11},
]  # fmt: skip


def claude_goal_check_log():
    """An orchestrator launching one implementer per spec; each implementer
    launches a goal-check reviewer, which streams no messages of its own."""
    review = "Check whether the goal is met."
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        assistant(
            "10:00:05",
            launch("toolu_IA", "general-purpose", f"Implement spec {LANE_A}.", "Implement A"),
            launch("toolu_IB", "general-purpose", f"Implement spec {LANE_B}. It builds on {LANE_A}.", "Implement B"),
        ),
        task_started("toolu_IA", "task_ia", background=True),
        task_started("toolu_IB", "task_ib", background=True),
        assistant("10:01:30", text("i" * 12), parent="toolu_IA", model=HAIKU),
        assistant("10:02:30", text("j" * 14), parent="toolu_IB", model=HAIKU),
        assistant("10:03:30", launch("toolu_GA", "general-purpose", review, "Goal check"), parent="toolu_IA", model=HAIKU),
        task_started("toolu_GA", "task_ga", background=False, depth=2, parent_task="task_ia"),
        assistant("10:04:30", launch("toolu_GB", "general-purpose", review, "Goal check"), parent="toolu_IB", model=HAIKU),
        task_started("toolu_GB", "task_gb", background=False, depth=2, parent_task="task_ib"),
        task_updated("task_ga", "10:04:10"),
        task_notification("toolu_GA", "task_ga", 40000, "Goal met."),
        task_updated("task_gb", "10:05:20"),
        task_notification("toolu_GB", "task_gb", 50000, "Goal met."),
        task_updated("task_ia", "10:05:30"),
        task_notification("toolu_IA", "task_ia", 325000, "Spec A implemented."),
        task_updated("task_ib", "10:06:30"),
        task_notification("toolu_IB", "task_ib", 385000, "Spec B implemented."),
        claude_result(spawned=4, completed=4),
    ]


def plan_goto(step):
    return {"command": f"""spektacular plan goto --data '{{"step":"{step}","name":"x"}}'"""}


def bob_trajectory():
    """A Bob ATIF trajectory: no timestamps, tokens, cost or model, and no
    sub-agent call."""

    def agent(step_id, message, *calls):
        return {
            "step_id": step_id,
            "source": "agent",
            "message": message,
            "tool_calls": [
                {"tool_call_id": f"t{step_id}", "function_name": "execute", "arguments": args} for args in calls
            ],
        }

    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "bob", "version": "2.0.6"},
        "steps": [
            {"step_id": 1, "source": "user", "message": "Create a plan"},
            agent(2, "Reading."),
            agent(3, "Moving on.", plan_goto("discovery")),
            {"step_id": 4, "source": "user", "message": "continue"},
            agent(5, "Exploring the code."),
            agent(6, "", plan_goto("architecture")),
            agent(7, "Designing."),
            agent(8, "Done.", plan_goto("finished")),
            agent(9, "Finished."),
        ],
        "final_metrics": {"total_steps": 9},
    }


def bob_children_trajectory():
    """A Bob trajectory with two sub-agent calls; the second names no kind
    and no prompt. Bob logs no times."""

    def call(tid, title, args):
        return {"tool_call_id": tid, "function_name": f"Running subagent: {title}", "arguments": args}

    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "bob", "version": "2.0.6"},
        "steps": [
            {"step_id": 1, "source": "user", "message": "Create a plan"},
            {"step_id": 2, "source": "agent", "message": "Reading.",
             "tool_calls": [call("b1", "map the repo", {"name": "explore", "description": "Map the repo and list its specs."})]},
            {"step_id": 3, "source": "agent", "message": "Moving on.",
             "tool_calls": [{"tool_call_id": "g1", "function_name": "execute", "arguments": plan_goto("discovery")}]},
            {"step_id": 4, "source": "agent", "message": "",
             "tool_calls": [call("b3", "write tests", {})]},
        ],
        "final_metrics": {"total_steps": 4},
    }  # fmt: skip


def codex_trajectory():
    """A Codex ATIF trajectory: timestamps, token totals, a model, no cost."""
    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "codex", "model_name": "gpt-5"},
        "steps": [
            {"step_id": 1, "source": "user", "timestamp": claude_ts("10:00:00"), "message": "go"},
            {
                "step_id": 2,
                "source": "agent",
                "timestamp": claude_ts("10:00:30"),
                "message": [{"type": "text", "text": "Listing."}],
                "tool_calls": [{"tool_call_id": "c1", "function_name": "exec_command", "arguments": {"command": "ls"}}],
            },
            {"step_id": 3, "source": "agent", "timestamp": claude_ts("10:02:30"), "message": "Reading the code."},
            {"step_id": 4, "source": "agent", "timestamp": claude_ts("10:06:30"), "message": "Finished."},
        ],
        "final_metrics": {
            "total_prompt_tokens": 5000,
            "total_completion_tokens": 700,
            "total_cached_tokens": 3000,
            "total_steps": 4,
        },
    }


def write_codex_rollout(agent_dir: Path) -> None:
    write_jsonl(
        agent_dir / "sessions" / "2026" / "10" / "23" / "rollout-2026-10-23T10-00-00-abc.jsonl",
        [
            {"timestamp": claude_ts("10:00:00"), "type": "session_meta", "payload": {}},
            {"timestamp": claude_ts("10:00:01"), "type": "turn_context", "payload": {"model": "gpt-5"}},
            {
                "timestamp": claude_ts("10:02:00"),
                "type": "event_msg",
                "payload": {
                    "type": "token_count",
                    "info": {"total_token_usage": {"input_tokens": 1000, "cached_input_tokens": 600, "output_tokens": 200}},
                },
            },
        ],
    )


def write_bob_logs(bob_logs: Path) -> None:
    """Bob's shell logs: 09:59:58.552 to 10:13:00.552 across two files, 782 s."""
    shell = bob_logs / "shell"
    shell.mkdir(parents=True)
    (shell / "bob-shell-20261023T095958.log").write_text(
        '{"ts":"2026-10-23T09:59:58.552Z","level":"debug","msg":"start"}\n'
        "not json at all\n"
        "[1, 2]\n"
        '{"level":"debug","msg":"no ts"}\n'
        '{"ts":"2026-10-23T10:12:30.052Z","level":"info","msg":"turn"}\n'
    )
    (shell / "bob-shell-20261023T101000.log").write_text('{"ts":"2026-10-23T10:13:00.552Z","msg":"exit"}\n')
    # not a .log file: ignored
    (shell / "notes.txt").write_text('{"ts":"2026-10-23T11:00:00Z"}\n')


# Each agent's log, written into a trial's agent directory.
AGENT_LOGS = {
    "claude": lambda d: write_jsonl(d / "claude-code.txt", claude_two_lane_log()),
    "bob": lambda d: (d / "trajectory.json").write_text(json.dumps(bob_children_trajectory())),
    "codex": lambda d: (d / "trajectory.json").write_text(json.dumps(codex_trajectory())),
    "codex-rollout": write_codex_rollout,
    "oracle": lambda d: None,
}
BUILD_AGENT = {"claude": "claude", "bob": "bob", "codex": "codex", "codex-rollout": "codex", "oracle": "oracle"}


def agent_trial(tmp_path, agent: str) -> Paths:
    paths = trial(tmp_path / agent, agent=BUILD_AGENT[agent])
    AGENT_LOGS[agent](paths.agent_dir)
    return paths


# --- 1. every agent, the same shape -------------------------------------------


def shape(record):
    return (
        list(record),
        list(record["run"]),
        list(record["result"]),
        list(record["totals"]),
        list(record["subagents"]),
        list(record["steps"]),
    )


def nested_keys(record):
    """Each workflow's, step's and helper's keys."""
    workflows = record["steps"]["workflows"] or []
    return (
        [list(w) for w in workflows],
        [list(s) for w in workflows for s in w["steps"]],
        [list(h) for h in record["subagents"]["helpers"] or []],
    )


EXPECTED_SHAPE = (TOP_KEYS, RUN_KEYS, RESULT_KEYS, RECORD_TOTALS_KEYS, RECORD_SUBAGENT_KEYS, ["workflows"])


@pytest.mark.parametrize("agent", list(AGENT_LOGS))
def test_every_agent_gives_the_same_shape(tmp_path, agent):
    record = run_main(agent_trial(tmp_path, agent))
    assert shape(record) == EXPECTED_SHAPE
    assert nested_keys(record) == ([WORKFLOW_KEYS], [STEP_KEYS] * 3, [HELPER_KEYS])
    assert record["schema_version"] == 3
    assert record["run"]["agent"] == BUILD_AGENT[agent]


def test_the_record_has_none_of_the_old_fields(tmp_path):
    record = run_main(agent_trial(tmp_path, "claude"))
    assert "idle_s" not in record["totals"]
    assert "unit" not in record["steps"] and "orchestrator_output" not in record["steps"]
    assert "stopped" not in record["subagents"] and "children" not in record["subagents"]
    for step in record["steps"]["workflows"][0]["steps"]:
        assert "output" not in step and "children" not in step


def test_oracle_run_has_no_agent_measures(tmp_path):
    record = run_main(agent_trial(tmp_path, "oracle"))
    reason = "no agent log (oracle or missing)"
    # wall time alone comes back, from the span of the logs' workflow runs
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": SPEC_WALL_S, **SPEC_LOG_TOTALS}
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": None}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}
    assert record["run"]["model"] is None
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": reason}]
        + [{"field": "subagents.briefs", "reason": reason}]
    )


# --- 2. the check score -------------------------------------------------------


def test_score_checks_overall_and_per_group():
    checks, groups = score_checks(JUNIT)
    assert checks == CHECKS
    assert groups == GROUPS


def test_score_checks_failure_and_error_both_count_as_not_passed():
    junit = """<testsuites><testsuite name="pytest">
      <testcase classname="tests.test_flow.TestA" name="f"><failure message="x"/></testcase>
      <testcase classname="tests.test_flow.TestA" name="e"><error message="x"/></testcase>
      <testcase classname="tests.test_flow.TestA" name="p"/>
      <testcase classname="tests.test_flow.TestA" name="s"><skipped/></testcase>
    </testsuite></testsuites>"""
    checks, groups = score_checks(junit)
    assert checks == {"passed": 1, "total": 3, "score": 1 / 3}
    assert groups == [{"name": "TestA", "passed": 1, "total": 3, "score": 1 / 3}]


def claude_trial(tmp_path, **kw) -> Paths:
    paths = trial(tmp_path, **kw)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    return paths


def test_record_carries_the_check_score(tmp_path):
    record = run_main(claude_trial(tmp_path))
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}


@pytest.mark.parametrize(
    "junit, reason_start",
    [
        (None, "no junit.xml from pytest"),
        ("<testsuites><testsuite>", "malformed junit.xml: "),
    ],
    ids=["missing", "malformed"],
)
def test_missing_or_malformed_junit_leaves_checks_unknown(tmp_path, junit, reason_start):
    record = run_main(claude_trial(tmp_path, junit=junit))
    assert record["result"] == {
        "reward": 1,
        "checks": {"passed": None, "total": None, "score": None},
        "groups": None,
    }
    assert unknown_fields(record) == ["result.checks", "result.groups"]
    assert reason_of(record, "result.checks").startswith(reason_start)
    # the rest of the record is untouched
    assert record["totals"] == {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}


def test_junit_with_no_checks_has_no_score(tmp_path):
    record = run_main(claude_trial(tmp_path, junit='<testsuites><testsuite name="pytest" tests="0"/></testsuites>'))
    assert record["result"]["checks"] == {"passed": 0, "total": 0, "score": None}
    assert record["result"]["groups"] == []
    assert record["unknown"] == [{"field": "result.checks.score", "reason": "pytest ran no checks"}]


# --- 3. a Claude spec run -----------------------------------------------------


def test_claude_spec_run_record_is_complete(tmp_path):
    record = run_main(claude_trial(tmp_path))
    assert record == {
        "schema_version": 3,
        "run": {
            "suite": "plan-workflow",
            "scenario": "default",
            "agent": "claude",
            "model": SONNET,
            "spektacular_version": "0.42.0",
            "commit": "abc1234",
            "dirty": False,
        },
        "result": {"reward": 1, "checks": CHECKS, "groups": GROUPS},
        "totals": {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS},
        "steps": {"workflows": [SPEC_WORKFLOW]},
        # the run started no child, so its briefs are an empty list, not unknown
        "subagents": {**SPEC_HELPERS, "briefs": []},
        "unknown": [],
    }


def test_the_recorder_asks_stats_for_the_trials_logs(tmp_path, stats):
    paths = claude_trial(tmp_path)
    run_main(paths)
    asked = {tuple(argv[2:-4]) for argv in stats.calls}
    assert asked == {("list",), ("run", "--run", fx.TRACE_S), ("spans",), ("commands",)}
    assert all(argv[-4:] == ["--dir", str(paths.log_dir), "--format", "json"] for argv in stats.calls)


def test_old_debug_dir_flag_still_names_the_logs(tmp_path):
    record = run_main(claude_trial(tmp_path), log_flag="--debug-dir")
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}


def test_summary_line_names_steps_and_helpers(tmp_path, capsys):
    run_main(claude_trial(tmp_path))
    assert capsys.readouterr().out == (
        "metrics: claude wall 6.7m, turns 12, cost $1.50, checks 4/6, 3 steps, 1 helpers, 0 unknown\n"
    )


def test_summary_line_says_steps_and_helpers_are_unknown_without_logs(tmp_path, capsys):
    paths = claude_trial(tmp_path)
    paths.log_dir.rmdir()
    run_main(paths)
    assert "unknown steps, unknown helpers, 7 unknown" in capsys.readouterr().out


# --- criterion 1: steps and their split come from the run report --------------


def test_steps_equal_the_run_report_and_each_split_adds_up(tmp_path):
    record = run_main(claude_trial(tmp_path))
    report = fx.spec_report()
    (workflow,) = record["steps"]["workflows"]
    assert [s["step"] for s in workflow["steps"]] == [r["step"] for r in report["steps"]]
    for step, row in zip(workflow["steps"], report["steps"]):
        assert round(step["duration_s"] * 1000) == row["duration_ms"]
        assert round(step["helper_wait_s"] * 1000) == row["helper_wait_ms"]
        assert round(step["user_wait_s"] * 1000) == row["user_wait_ms"]
        assert (step["entries"], step["commands"], step["incomplete"]) == (row["entries"], row["commands"], row["incomplete"])
        assert step["working_s"] + step["helper_wait_s"] + step["user_wait_s"] == step["duration_s"]
    # the trial has both a helper and a user wait to split out
    by_step = {s["step"]: s for s in workflow["steps"]}
    assert by_step["overview"]["user_wait_s"] == 120.0 and by_step["overview"]["working_s"] == 174.0
    assert by_step["discovery"]["helper_wait_s"] == 180.0 and by_step["discovery"]["working_s"] == 234.0
    assert workflow["wall_s"] * 1000 == report["totals"]["wall_ms"]


def test_steps_of_the_golden_report_in_seconds():
    # The Go golden report: analyze is entered twice, its second entry cut
    # off, and both its helpers count; verify waits two minutes on the user.
    assert metrics.step_rows(golden_report()) == [
        {"step": "setup", "started_at": "2026-10-25T09:01:00Z", "entries": 1, "duration_s": 60.0, "working_s": 60.0,
         "helper_wait_s": 0.0, "user_wait_s": 0.0, "commands": 1, "helpers": 0, "incomplete": 0},
        {"step": "analyze", "started_at": "2026-10-25T09:02:00Z", "entries": 2, "duration_s": 960.0, "working_s": 780.0,
         "helper_wait_s": 180.0, "user_wait_s": 0.0, "commands": 2, "helpers": 2, "incomplete": 1},
        {"step": "verify", "started_at": "2026-10-25T09:08:00Z", "entries": 1, "duration_s": 720.0, "working_s": 600.0,
         "helper_wait_s": 0.0, "user_wait_s": 120.0, "commands": 2, "helpers": 0, "incomplete": 0},
        {"step": "finished", "started_at": "2026-10-25T09:30:00Z", "entries": 1, "duration_s": 0.0, "working_s": 0.0,
         "helper_wait_s": 0.0, "user_wait_s": 0.0, "commands": 0, "helpers": 0, "incomplete": 0},
    ]  # fmt: skip


def test_working_time_is_floored_at_zero_when_waits_overlap(tmp_path, stats):
    # A helper running while the user is asked counts in both waits, which
    # can then exceed the step: working time is 0, never negative.
    overview = stats.answers["run"][fx.TRACE_S]["steps"][0]
    overview.update(helper_wait_ms=200000, user_wait_ms=150000)  # 350 s of waits in a 294 s step
    record = run_main(claude_trial(tmp_path))
    step = record["steps"]["workflows"][0]["steps"][0]
    assert (step["duration_s"], step["working_s"], step["helper_wait_s"], step["user_wait_s"]) == (294.0, 0.0, 200.0, 150.0)


def test_working_time_keeps_milliseconds():
    report = {"steps": [{"step": "s", "entries": 1, "duration_ms": 1234, "helper_wait_ms": 200, "user_wait_ms": 34,
                         "commands": 0, "incomplete": 0}]}  # fmt: skip
    (row,) = metrics.step_rows(report)
    assert (row["duration_s"], row["working_s"], row["helper_wait_s"], row["user_wait_s"]) == (1.234, 1.0, 0.2, 0.034)
    assert row["started_at"] is None  # the report gave no entry for it
    assert row["helpers"] == 0


# --- criterion 2: every helper with its spec and step ----------------------------


def test_every_helper_is_listed_with_its_spec_and_step(tmp_path):
    record = run_main(claude_trial(tmp_path))
    assert record["subagents"]["helpers"] == [SPEC_HELPER]
    # the user wait is a span too, but no helper
    assert record["subagents"]["started"] == 1


def test_each_step_counts_the_helpers_started_in_it(tmp_path, stats):
    stats.answers = two_run_answers()
    record = run_main(claude_trial(tmp_path))
    counts = {w["name"]: [(s["step"], s["helpers"]) for s in w["steps"]] for w in record["steps"]["workflows"]}
    assert counts == {
        GOLDEN_NAME: [("setup", 0), ("analyze", 2), ("verify", 0), ("finished", 0)],
        fx.GAMMA: [("overview", 0), ("discovery", 1), ("finished", 0)],
    }
    # every step's count is a number, never unknown
    assert all(type(s["helpers"]) is int for w in record["steps"]["workflows"] for s in w["steps"])


# The epic trial's helpers, in start order.
EPIC_HELPERS = [
    # launched the child that started alpha's run
    {"span_id": "oa", "parent": None, "name": f"plan {fx.ALPHA}", "spec": fx.ALPHA, "kind": None, "step": None,
     "task_id": None, "outside_run": True, "started_at": "2026-10-25T10:01:00Z", "ended_at": "2026-10-25T10:20:00Z",
     "duration_s": 1140.0, "end_reason": "stopped", "status": "ok", "complete": True},
    # launched the child that started beta's run
    {"span_id": "ob1", "parent": None, "name": f"plan {fx.BETA}", "spec": fx.BETA, "kind": None, "step": None,
     "task_id": None, "outside_run": True, "started_at": "2026-10-25T10:01:30Z", "ended_at": "2026-10-25T10:09:00Z",
     "duration_s": 450.0, "end_reason": "stopped", "status": "ok", "complete": True},
    # launched no child: alpha's first run started while it ran
    {"span_id": "oc", "parent": None, "name": "explore", "spec": fx.ALPHA, "kind": None, "step": None,
     "task_id": None, "outside_run": True, "started_at": "2026-10-25T10:01:48Z", "ended_at": "2026-10-25T10:02:12Z",
     "duration_s": 24.0, "end_reason": "stopped", "status": "ok", "complete": True},
    # started inside alpha's run, in discovery
    {"span_id": "ha1", "parent": "sa1", "name": "research", "spec": fx.ALPHA, "kind": "plan", "step": "discovery",
     "task_id": None, "outside_run": False, "started_at": "2026-10-25T10:03:00Z", "ended_at": "2026-10-25T10:05:00Z",
     "duration_s": 120.0, "end_reason": "stopped", "status": "ok", "complete": True},
    # launched the child that resumed beta, and never stopped
    {"span_id": "ob2", "parent": None, "name": f"plan {fx.BETA} (resume)", "spec": fx.BETA, "kind": None, "step": None,
     "task_id": None, "outside_run": True, "started_at": "2026-10-25T10:10:30Z", "ended_at": "2026-10-25T10:10:30Z",
     "duration_s": 0.0, "end_reason": "open", "status": None, "complete": False},
    # after both lanes: no spec at all
    {"span_id": "od", "parent": None, "name": "tidy", "spec": None, "kind": None, "step": None,
     "task_id": None, "outside_run": True, "started_at": "2026-10-25T10:21:00Z", "ended_at": "2026-10-25T10:21:30Z",
     "duration_s": 30.0, "end_reason": "stopped", "status": "ok", "complete": True},
]  # fmt: skip


def test_an_orchestrators_helpers_take_the_lane_of_their_child(tmp_path, stats):
    stats.answers = epic_answers()
    record = run_main(claude_trial(tmp_path))
    assert record["subagents"]["helpers"] == EPIC_HELPERS
    assert {k: record["subagents"][k] for k in HELPER_COUNT_KEYS if k != "helpers"} == {
        "started": 6, "completed": 5, "failed": 0, "unfinished": 1,
        "durations_s": [1140.0, 450.0, 24.0, 120.0, 30.0],  # the open one has none
    }  # fmt: skip
    assert [(w["name"], [(s["step"], s["helpers"]) for s in w["steps"]]) for w in record["steps"]["workflows"]] == [
        (fx.ALPHA, [("discovery", 1), ("architecture", 0), ("finished", 0)]),
        (fx.BETA, [("discovery", 0), ("architecture", 0), ("finished", 0)]),
    ]
    # an orchestrator's helper is in no step, and that is not unknown
    assert record["unknown"] == []


def test_a_failed_helper_is_counted_failed(tmp_path, stats):
    (helper,) = [s for s in stats.answers["spans"]["spans"] if s["span_id"] == "hs1"]
    helper.update(status="error", end_reason="abandoned")
    record = run_main(claude_trial(tmp_path))
    sub = record["subagents"]
    assert (sub["started"], sub["completed"], sub["failed"], sub["unfinished"]) == (1, 0, 1, 0)
    assert sub["durations_s"] == [180.0]  # it did stop
    assert (sub["helpers"][0]["status"], sub["helpers"][0]["end_reason"]) == ("error", "abandoned")


# --- criterion 3: total user wait, helper wait and gaps ---------------------------


def test_totals_sum_the_waits_and_gaps_of_every_run(tmp_path, stats):
    stats.answers = two_run_answers()
    record = run_main(claude_trial(tmp_path))
    workflows = record["steps"]["workflows"]
    # in start order, though `stats list` gave the newer run first
    assert [w["run_id"] for w in workflows] == [GOLDEN_RUN, fx.TRACE_S]
    assert {k: workflows[0][k] for k in WORKFLOW_KEYS if k != "steps"} == {
        "run_id": GOLDEN_RUN, "kind": "implement", "name": GOLDEN_NAME, "status": "finished",
        "wall_s": 1800.0, "user_wait_s": 120.0, "helper_wait_s": 180.0, "gap_s": 900.0, "gaps": 2,
    }  # fmt: skip
    assert {k: record["totals"][k] for k in LOG_TOTALS_KEYS} == {
        "user_wait_s": 120.0 + 120.0,
        "helper_wait_s": 180.0 + 180.0,
        "gap_s": 900.0 + 0.0,
        "gaps": 2 + 0,
        "gap_threshold_s": 300.0,
        "trace_calls": 4,
        "trace_calls_per_task": 2.0,  # the golden run's two tasks
    }
    # the single longest gap is gone
    assert "idle_s" not in record["totals"]
    sub = record["subagents"]
    assert (sub["started"], sub["completed"], sub["failed"], sub["unfinished"]) == (3, 2, 0, 1)
    assert sub["durations_s"] == [180.0, 180.0]


def test_trace_calls_count_only_trace_commands(tmp_path, stats):
    commands = stats.answers["commands"]["commands"]
    assert sum(1 for c in commands if c["args"][0] == "trace") == 4
    commands.append({**commands[0], "args": ["status", "trace"], "command": "status"})
    record = run_main(claude_trial(tmp_path))
    assert (record["totals"]["trace_calls"], record["totals"]["trace_calls_per_task"]) == (4, None)


# --- criterion 4: no logs, nothing filled in from the transcript ----------------


def break_logs(how: str, paths: Paths, stats: Stats, monkeypatch) -> str:
    """Breaks the trial's logs one way and returns the reason the record gives."""
    if how == "logs-removed":
        paths.log_dir.rmdir()
        return f"no Spektacular logs at {paths.log_dir}"
    if how == "no-binary":
        monkeypatch.setattr(metrics.runlog, "RunLog", REAL_RUNLOG)
        monkeypatch.delenv("SPEKTACULAR_BIN", raising=False)
        monkeypatch.setenv("PATH", "")
        return "no spektacular binary on PATH; set SPEKTACULAR_BIN to one that has `stats`"
    if how == "cannot-run":
        stats.fail = FileNotFoundError(2, "No such file or directory")
        return "cannot run spektacular: [Errno 2] No such file or directory; set SPEKTACULAR_BIN to a binary that has `stats`"
    if how == "timeout":
        stats.fail = subprocess.TimeoutExpired(["spektacular"], 60)
        return "`spektacular stats list` took longer than 60s"
    if how == "no-stats-command":
        stats.fail = 'Error: unknown command "stats" for "spektacular"'
        return "`spektacular stats list` gave no JSON; use a spektacular build that has `stats list`"
    if how == "unexpected-json":
        stats.fail = [1, 2]
        return "`spektacular stats list` gave unexpected JSON"
    if how == "refused":
        stats.fail = {"error": True, "code": "stats_log_dir_not_found", "message": "the logs directory does not exist"}
        return "stats list: stats_log_dir_not_found: the logs directory does not exist"
    if how == "spans-refused":
        # the runs and reports read, the helpers not: nothing is kept
        stats.fail, stats.fail_on = {"error": True, "code": "bad_record", "message": "unreadable span"}, "spans"
        return "stats spans: bad_record: unreadable span"
    if how == "no-runs":
        stats.answers["list"]["runs"] = []
        return "the Spektacular logs hold no workflow run"
    raise AssertionError(how)


BREAKS = ["logs-removed", "no-binary", "cannot-run", "timeout", "no-stats-command", "unexpected-json", "refused",
          "spans-refused", "no-runs"]  # fmt: skip


def assert_logs_unknown(record: dict, reason: str) -> None:
    assert record["steps"] == {"workflows": None}
    assert {k: record["subagents"][k] for k in HELPER_COUNT_KEYS} == dict.fromkeys(HELPER_COUNT_KEYS)
    assert {k: record["totals"][k] for k in LOG_TOTALS_KEYS} == dict.fromkeys(LOG_TOTALS_KEYS)
    assert [u for u in record["unknown"] if u["field"] in LOG_FIELDS] == [{"field": f, "reason": reason} for f in LOG_FIELDS]


@pytest.mark.parametrize("how", BREAKS)
def test_logs_that_cannot_answer_leave_every_log_measure_unknown(tmp_path, stats, monkeypatch, how):
    # The transcript reports three sub-agents spawned and holds their
    # launches; none of that stands in for the logs' helpers.
    paths = agent_trial(tmp_path, "claude")
    reason = break_logs(how, paths, stats, monkeypatch)
    record = run_main(paths)  # returns 0 and leaves the reward untouched
    assert_logs_unknown(record, reason)
    assert record["unknown"] == [{"field": f, "reason": reason} for f in LOG_FIELDS]
    # the transcript's own measures are all there
    assert {k: record["totals"][k] for k in TOTALS_KEYS} == CLAUDE_TOTALS
    assert record["subagents"]["briefs"] == CLAUDE_TWO_LANE_BRIEFS
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}
    assert shape(record) == EXPECTED_SHAPE


@pytest.mark.parametrize("agent", ["claude", "bob", "codex", "codex-rollout", "oracle"])
def test_logs_removed_leave_the_log_measures_unknown_for_every_agent(tmp_path, agent):
    paths = agent_trial(tmp_path, agent)
    paths.log_dir.rmdir()
    record = run_main(paths)
    assert_logs_unknown(record, f"no Spektacular logs at {paths.log_dir}")
    assert record["result"]["reward"] == 1


def test_bob_without_logs_or_shell_logs_leaves_wall_time_unknown(tmp_path):
    paths = agent_trial(tmp_path, "bob")
    paths.log_dir.rmdir()
    record = run_main(paths)
    assert record["totals"]["wall_time_s"] is None
    assert reason_of(record, "totals.wall_time_s") == "bob trajectory has no step timestamps"


def test_a_recorder_error_in_the_log_reader_costs_only_the_log_fields(tmp_path, monkeypatch):
    def boom(report):
        raise RuntimeError("bad step row")

    monkeypatch.setattr(metrics, "step_rows", boom)
    paths = agent_trial(tmp_path, "claude")
    record = run_main(paths)  # returns 0 and leaves the reward untouched
    assert record["steps"] == {"workflows": None}
    assert {k: record["subagents"][k] for k in HELPER_COUNT_KEYS} == dict.fromkeys(HELPER_COUNT_KEYS)
    assert {k: record["totals"][k] for k in LOG_TOTALS_KEYS} == dict.fromkeys(LOG_TOTALS_KEYS)
    assert reason_of(record, "steps") == "recorder error: bad step row"
    # everything from the transcript, the junit and the reward survives
    assert {k: record["totals"][k] for k in TOTALS_KEYS} == CLAUDE_TOTALS
    assert record["subagents"]["briefs"] == CLAUDE_TWO_LANE_BRIEFS
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}


def test_a_recorder_error_in_the_log_reader_lists_every_null_log_field(tmp_path, monkeypatch):
    def boom(report):
        raise RuntimeError("bad step row")

    monkeypatch.setattr(metrics, "step_rows", boom)
    record = run_main(agent_trial(tmp_path, "claude"))
    assert set(LOG_FIELDS) <= set(unknown_fields(record))


# --- criterion 6: the same logs, the same steps and helpers for every agent ------


def log_sections(record: dict) -> dict:
    """Everything the record takes from Spektacular's logs."""
    return {
        "steps": record["steps"],
        "subagents": {k: record["subagents"][k] for k in HELPER_COUNT_KEYS},
        "totals": {k: record["totals"][k] for k in LOG_TOTALS_KEYS},
        "unknown": [u for u in record["unknown"] if u["field"] in LOG_FIELDS],
    }


def without_transcript_fields(record: dict) -> dict:
    """The record less what the agent's log gives: transcript totals, the
    agent and model, briefs, and the unknowns those carry."""
    out = copy.deepcopy(record)
    for name in TOTALS_KEYS:
        out["totals"].pop(name)
    out["run"].pop("agent")
    out["run"].pop("model")
    out["subagents"].pop("briefs")
    out["unknown"] = [u for u in out["unknown"] if u["field"] in LOG_FIELDS]
    return out


@pytest.mark.parametrize("answers", [spec_answers, epic_answers], ids=["spec", "epic"])
def test_the_same_logs_give_the_same_steps_and_helpers_for_every_agent(tmp_path, stats, answers):
    records = {}
    for agent in AGENT_LOGS:
        stats.answers = answers()
        paths = agent_trial(tmp_path, agent)
        if agent == "bob":
            write_bob_logs(paths.bob_logs)
        records[agent] = run_main(paths)
    claude = records["claude"]
    assert claude["steps"]["workflows"] and claude["subagents"]["helpers"]
    for agent, record in records.items():
        assert record["steps"] == claude["steps"], agent
        assert record["subagents"]["helpers"] == claude["subagents"]["helpers"], agent
        assert log_sections(record) == log_sections(claude), agent
        assert shape(record) == shape(claude) and nested_keys(record) == nested_keys(claude), agent
        # they differ only in the transcript's totals and briefs
        assert without_transcript_fields(record) == without_transcript_fields(claude), agent
    # and they do differ there
    assert len({json.dumps(r["subagents"]["briefs"]) for r in records.values()}) >= 3
    assert len({r["totals"]["wall_time_s"] for r in records.values()}) >= 3


# --- briefs: what each helper was told, from the transcript ---------------------


def test_claude_briefs_are_every_child_with_its_launch(tmp_path):
    record = run_main(agent_trial(tmp_path, "claude"))
    assert record["subagents"]["briefs"] == CLAUDE_TWO_LANE_BRIEFS
    assert [list(b) for b in record["subagents"]["briefs"]] == [BRIEF_KEYS] * 3
    # the helpers still come from the logs, not from the transcript's three
    assert record["subagents"]["started"] == 1
    assert record["unknown"] == []


def test_claude_briefs_of_children_that_streamed_nothing_have_unknown_models(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_goal_check_log())
    record = run_main(paths)
    briefs = record["subagents"]["briefs"]
    assert [(b["id"], b["parent"], b["models"]) for b in briefs] == [
        ("toolu_IA", None, [HAIKU]),
        ("toolu_IB", None, [HAIKU]),
        ("toolu_GA", "toolu_IA", None),
        ("toolu_GB", "toolu_IB", None),
    ]
    streamed_none = "claude streamed none of the child's messages"
    assert record["unknown"] == [
        {"field": "subagents.briefs[toolu_GA].models", "reason": streamed_none},
        {"field": "subagents.briefs[toolu_GB].models", "reason": streamed_none},
    ]


def test_bob_briefs_carry_kind_description_and_prompt(tmp_path):
    record = run_main(agent_trial(tmp_path, "bob"))
    assert record["subagents"]["briefs"] == [
        {"id": "b1", "parent": None, "kind": "explore", "description": "map the repo", "models": None,
         "prompt": "Map the repo and list its specs.", "prompt_chars": 32},
        {"id": "b3", "parent": None, "kind": None, "description": "write tests", "models": None,
         "prompt": None, "prompt_chars": None},
    ]  # fmt: skip
    no_prompt = "the bob sub-agent call named no prompt"
    assert [u for u in record["unknown"] if u["field"].startswith("subagents.")] == [
        {"field": "subagents.briefs.models", "reason": "bob logs no sub-agent models"},
        {"field": "subagents.briefs[b3].kind", "reason": "the bob sub-agent call named no kind"},
        {"field": "subagents.briefs[b3].prompt", "reason": no_prompt},
        {"field": "subagents.briefs[b3].prompt_chars", "reason": no_prompt},
    ]


@pytest.mark.parametrize("agent", ["codex", "codex-rollout"])
def test_codex_briefs_are_unknown(tmp_path, agent):
    record = run_main(agent_trial(tmp_path, agent))
    assert record["subagents"]["briefs"] is None
    assert reason_of(record, "subagents.briefs") == "codex reports no sub-agents"
    # its helpers come from the logs all the same
    assert {k: record["subagents"][k] for k in HELPER_COUNT_KEYS} == SPEC_HELPERS


def test_raising_child_reader_costs_only_the_briefs(tmp_path, monkeypatch):
    def boom(objs):
        raise RuntimeError("bad child event")

    monkeypatch.setattr(metrics.tx, "claude_children", boom)
    record = run_main(claude_trial(tmp_path))  # returns 0 and leaves the reward untouched
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}
    assert record["totals"] == {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS}
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": None}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}
    assert record["unknown"] == [{"field": "subagents.briefs", "reason": "recorder error: bad child event"}]


# --- a missing or broken input costs only its own fields -------------------------


def test_missing_agent_dir_still_gives_a_record(tmp_path):
    paths = trial(tmp_path)
    paths.agent_dir.rmdir()
    record = run_main(paths)
    reason = "no agent log (oracle or missing)"
    assert record["totals"]["turns"] is None
    assert record["totals"]["cost_usd"] is None
    assert reason_of(record, "totals.turns") == reason
    assert reason_of(record, "subagents.briefs") == reason
    assert record["result"]["checks"] == CHECKS
    assert record["subagents"]["helpers"] == [SPEC_HELPER]


def test_missing_build_info_leaves_run_fields_unknown(tmp_path):
    record = run_main(claude_trial(tmp_path, build=None))
    reason = "no build.json (suite not run through its make target)"
    assert record["run"] == {
        "suite": None,
        "scenario": None,
        "agent": "claude",  # from the agent log
        "model": SONNET,
        "spektacular_version": None,
        "commit": None,
        "dirty": None,
    }
    assert record["unknown"] == [
        {"field": f"run.{n}", "reason": reason}
        for n in ("suite", "spektacular_version", "commit", "dirty")
    ]  # the agent is not unknown: the log named it
    assert record["totals"] == {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS}


@pytest.mark.parametrize(
    "raw, reason_start",
    [
        ("{not json", "unreadable build.json: "),
        ('["plan-workflow"]', "build.json is not an object"),
    ],
    ids=["malformed", "not-an-object"],
)
def test_broken_build_info_leaves_run_fields_unknown(tmp_path, raw, reason_start):
    record = run_main(claude_trial(tmp_path, build=raw))
    assert record["run"]["suite"] is None and record["run"]["commit"] is None
    assert unknown_fields(record) == ["run.suite", "run.spektacular_version", "run.commit", "run.dirty"]
    assert reason_of(record, "run.suite").startswith(reason_start)


def test_build_info_missing_some_fields_lists_only_those(tmp_path):
    record = run_main(claude_trial(tmp_path, build={"suite": "plan-workflow", "agent": "claude", "dirty": "yes"}))
    assert record["run"]["suite"] == "plan-workflow"
    assert record["run"]["dirty"] is None  # not a bool
    assert record["unknown"] == [
        {"field": f"run.{n}", "reason": "missing from build.json"}
        for n in ("spektacular_version", "commit", "dirty")
    ]


def test_malformed_lines_in_claude_log_are_skipped(tmp_path):
    paths = trial(tmp_path)
    lines = [json.dumps(o) for o in claude_plan_log()]
    lines.insert(3, "{this is not json")
    lines.insert(5, '"a bare string"')
    lines.append('{"type": "assistant", "timestamp": "2026-10-23T10:0')  # cut off mid-write
    (paths.agent_dir / "claude-code.txt").write_text("\n".join(lines) + "\n")
    record = run_main(paths)
    assert record["totals"] == {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS}
    assert record["unknown"] == []


def test_claude_log_of_only_malformed_lines_leaves_totals_unknown(tmp_path):
    paths = trial(tmp_path)
    (paths.agent_dir / "claude-code.txt").write_text("{garbage\nnot json\n")
    record = run_main(paths)
    reason = "no result event in claude-code.txt"
    # wall time falls back to the span of the logs' runs; the rest is unknown
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": SPEC_WALL_S, **SPEC_LOG_TOTALS}
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": "no result or init event in claude-code.txt"}]
    )
    assert record["subagents"]["briefs"] == []
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 file")
def test_unreadable_claude_log_still_gives_a_record(tmp_path):
    paths = trial(tmp_path)
    log = paths.agent_dir / "claude-code.txt"
    write_jsonl(log, claude_plan_log())
    log.chmod(0o000)
    try:
        record = run_main(paths)
    finally:
        log.chmod(0o644)
    fields = unknown_fields(record)
    assert fields[0] == "totals"
    assert reason_of(record, "totals").startswith("recorder error: ")
    assert [f for f in fields if f.startswith("totals.")] == [f"totals.{n}" for n in TOTALS_KEYS if n != "wall_time_s"]
    assert reason_of(record, "totals.turns") == "recorder error reading the agent log"
    assert reason_of(record, "subagents.briefs") == "recorder error reading the agent log"
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": SPEC_WALL_S, **SPEC_LOG_TOTALS}
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": None}
    assert record["result"]["checks"] == CHECKS


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 file")
def test_unreadable_junit_leaves_checks_unknown(tmp_path):
    paths = claude_trial(tmp_path)
    paths.junit.chmod(0o000)
    try:
        record = run_main(paths)
    finally:
        paths.junit.chmod(0o644)
    assert unknown_fields(record) == ["result.checks", "result.groups"]
    assert reason_of(record, "result.checks").startswith("unreadable junit.xml: ")


def test_missing_reward_is_unknown_and_not_created(tmp_path):
    paths = claude_trial(tmp_path, reward=None)
    record = run_main(paths)
    assert record["result"]["reward"] is None
    assert unknown_fields(record) == ["result.reward"]
    assert not paths.reward.exists()


def test_every_input_missing_still_gives_a_record(tmp_path):
    paths = trial(tmp_path, build=None, junit=None)
    paths.agent_dir.rmdir()
    paths.log_dir.rmdir()
    record = run_main(paths)
    assert shape(record) == EXPECTED_SHAPE
    assert record["totals"] == dict.fromkeys(RECORD_TOTALS_KEYS)
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert record["steps"] == {"workflows": None}
    assert record["result"]["reward"] == 1
    fields = unknown_fields(record)
    for name in ("run.suite", "totals.wall_time_s", "result.checks", "subagents.briefs", *LOG_FIELDS):
        assert name in fields


# --- Bob ----------------------------------------------------------------------


def bob_trial(tmp_path) -> Paths:
    paths = trial(tmp_path, agent="bob")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(bob_trajectory()))
    return paths


def test_bob_run_wall_time_from_shell_logs(tmp_path):
    paths = bob_trial(tmp_path)
    write_bob_logs(paths.bob_logs)
    record = run_main(paths)
    assert record["run"]["agent"] == "bob"
    assert record["totals"] == {
        **dict.fromkeys(TOTALS_KEYS),
        "wall_time_s": 782.0,  # 09:59:58.552 to 10:13:00.552
        "turns": 7,
        **SPEC_LOG_TOTALS,
    }
    assert record["unknown"] == [
        {"field": "totals.model_time_s", "reason": "bob reports no model time"},
        {"field": "totals.cost_usd", "reason": "bob reports no cost"},
        {"field": "totals.input_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.output_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.cache_read_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.cache_write_tokens", "reason": "bob reports no cache writes"},
        {"field": "run.model", "reason": "bob reports no model"},
    ]
    # no sub-agent call: no briefs, and its helpers come from the logs
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": []}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}


def test_bob_run_wall_time_from_spektacular_logs_without_shell_logs(tmp_path):
    record = run_main(bob_trial(tmp_path))
    assert record["totals"]["wall_time_s"] == SPEC_WALL_S  # 10:00:00 to 10:12:06
    assert "totals.wall_time_s" not in unknown_fields(record)


# --- Codex --------------------------------------------------------------------


def test_codex_atif_run(tmp_path):
    record = run_main(agent_trial(tmp_path, "codex"))
    assert record["run"]["agent"] == "codex"
    assert record["run"]["model"] == "gpt-5"
    assert record["totals"] == {
        **dict.fromkeys(TOTALS_KEYS),
        "wall_time_s": 390.0,  # 10:00:00 to 10:06:30
        "turns": 3,
        "input_tokens": 2000,  # 5000 prompt tokens, 3000 of them cached
        "output_tokens": 700,
        "cache_read_tokens": 3000,
        **SPEC_LOG_TOTALS,
    }
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": None}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}
    assert record["unknown"] == [
        {"field": "totals.model_time_s", "reason": "codex reports no model time"},
        {"field": "totals.cost_usd", "reason": "codex reports no cost"},
        {"field": "totals.cache_write_tokens", "reason": "codex reports no cache writes"},
        {"field": "subagents.briefs", "reason": "codex reports no sub-agents"},
    ]


def test_codex_rollout_run(tmp_path):
    record = run_main(agent_trial(tmp_path, "codex-rollout"))
    assert record["totals"]["turns"] == 1
    assert record["totals"]["wall_time_s"] == 120.0
    assert record["subagents"] == {**SPEC_HELPERS, "briefs": None}
    assert record["steps"] == {"workflows": [SPEC_WORKFLOW]}


# --- agent mismatch -----------------------------------------------------------


def test_agent_log_not_matching_build_info_is_flagged(tmp_path):
    record = run_main(claude_trial(tmp_path, agent="codex"))
    assert record["run"]["agent"] == "codex"
    assert record["unknown"] == [
        {"field": "run.agent", "reason": "the agent log is claude's but build.json names codex"}
    ]
    assert record["totals"] == {**CLAUDE_TOTALS, **SPEC_LOG_TOTALS}


# --- writing the record -------------------------------------------------------


def test_write_record_is_world_readable_json(tmp_path):
    paths = claude_trial(tmp_path)
    record = build_record(paths)
    out = tmp_path / "nested" / "dir" / "metrics.json"
    old = os.umask(0o077)
    try:
        write_record(record, out)
    finally:
        os.umask(old)
    assert out.stat().st_mode & 0o777 == 0o644
    assert json.loads(out.read_text()) == record
    assert sorted(p.name for p in out.parent.iterdir()) == ["metrics.json"]  # no temp file left


def test_main_with_unwritable_out_still_returns_zero(tmp_path, capsys):
    paths = claude_trial(tmp_path)
    blocker = tmp_path / "a-file"
    blocker.write_text("")
    paths.out = blocker / "metrics.json"  # its parent is a regular file
    before = reward_state(paths)
    assert main(args_of(paths)) == 0
    assert reward_state(paths) == before
    assert not paths.out.exists()
    assert "metrics: no record written" in capsys.readouterr().err


def test_main_with_bad_arguments_returns_zero(tmp_path, capsys):
    assert main(["--no-such-option"]) == 0
    capsys.readouterr()
