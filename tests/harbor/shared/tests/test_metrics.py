"""Self-check for the run metrics recorder in metrics.py.

The recorder reads a finished harbor trial (build info, the agent's own log,
Spektacular's session log, Bob's shell logs, pytest's JUnit XML and the
reward) and writes one metrics record in the same shape for every agent and
suite. A measure a source does not supply is null and is named under
`unknown`; a missing or broken input costs only its own fields; `main` always
returns 0 and never writes the reward. Each case here builds a small trial
tree by hand under tmp_path, with hand-worked expected values, and runs the
recorder over it, mostly through `main` exactly as test.sh does.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_metrics.py
"""

import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import metrics  # noqa: E402
from metrics import Paths, build_record, idle_seconds, lane_for, main, score_checks, write_record  # noqa: E402

NAME = "20260101000000-user-auth"
LANE_A = "20261001-user-auth"
LANE_B = "20261001-user-auth-api"  # contains LANE_A: the longer name must win
SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"

TOP_KEYS = ["schema_version", "run", "result", "totals", "steps", "subagents", "unknown"]
RUN_KEYS = ["suite", "scenario", "agent", "model", "spektacular_version", "commit", "dirty"]
RESULT_KEYS = ["reward", "checks", "groups"]
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
SUBAGENT_KEYS = ["started", "completed", "failed", "stopped", "durations_s"]
# The record's totals and subagents sections add the run's idle time and its
# child list to the counts above.
RECORD_TOTALS_KEYS = [*TOTALS_KEYS, "idle_s", "trace_calls", "trace_calls_per_task"]
# The trace calls (spec 000074) of a run whose session log holds none, and of
# a run with no session log at all.
TRACE_ZERO = {"trace_calls": 0, "trace_calls_per_task": None}
TRACE_NONE = {"trace_calls": None, "trace_calls_per_task": None}
RECORD_SUBAGENT_KEYS = [*SUBAGENT_KEYS, "children"]
STEP_KEYS = ["step", "started_at", "duration_s", "output", "children"]
CHILD_KEYS = [
    "id",
    "parent",
    "kind",
    "description",
    "background",
    "spec",
    "step",
    "launched_at",
    "ended_at",
    "duration_s",
    "ending",
    "hand_back",
    "models",
    "output",
    "prompt",
    "prompt_chars",
]


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


# --- the trial tree -----------------------------------------------------------


def trial(tmp_path, *, agent="claude", build="default", junit=JUNIT, reward="1\n") -> Paths:
    """A trial tree laid out like a collected harbor trial. build is the
    build.json object, raw text, None for no file, or "default"."""
    paths = Paths(
        build_info=tmp_path / "tests" / "build.json",
        agent_dir=tmp_path / "agent",
        debug_dir=tmp_path / "debug",
        bob_logs=tmp_path / "bob-logs",  # absent unless a test writes it
        junit=tmp_path / "verifier" / "junit.xml",
        reward=tmp_path / "verifier" / "reward.txt",
        out=tmp_path / "verifier" / "metrics.json",
    )
    for d in (paths.build_info.parent, paths.agent_dir, paths.debug_dir, paths.junit.parent):
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


def args_of(paths: Paths) -> list[str]:
    return [
        "--build-info", str(paths.build_info),
        "--agent-dir", str(paths.agent_dir),
        "--debug-dir", str(paths.debug_dir),
        "--bob-logs", str(paths.bob_logs),
        "--junit", str(paths.junit),
        "--reward", str(paths.reward),
        "--out", str(paths.out),
    ]  # fmt: skip


def reward_state(paths: Paths):
    if not paths.reward.exists():
        return None
    return paths.reward.read_bytes(), paths.reward.stat().st_mtime_ns


def run_main(paths: Paths) -> dict:
    """Runs the recorder as test.sh does, checks it returned 0 and left the
    reward untouched, and returns the record it wrote."""
    before = reward_state(paths)
    assert main(args_of(paths)) == 0
    assert reward_state(paths) == before
    return json.loads(paths.out.read_text())


def unknown_fields(record) -> list[str]:
    return [u["field"] for u in record["unknown"]]


def reason_of(record, name) -> str:
    return next(u["reason"] for u in record["unknown"] if u["field"] == name)


def write_jsonl(path: Path, objs) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))


# --- the session log ----------------------------------------------------------


def cli_ts(clock: str) -> str:
    """A fixed synthetic timestamp on the test day, as the CLI writes it (ns)."""
    return f"2026-10-23T{clock}.000000000Z"


def state(step, name=NAME, kind="plan"):
    return {"kind": kind, "name": name, "current_step": step}


def goto(step, name=NAME, kind="plan"):
    return [kind, "goto", "--data", json.dumps({"step": step, "name": name})]


def new(name=NAME, kind="plan"):
    return [kind, "new", "--data", json.dumps({"name": name})]


def record(clock, command, before, after, *, duration_ms=0, exit_code=0):
    return {
        "timestamp": cli_ts(clock),
        "command": command,
        "duration_ms": duration_ms,
        "exit_code": exit_code,
        "state_before": before,
        "state_after": after,
    }


def write_log(debug_dir: Path, started: str, records, session, agent="claude") -> Path:
    """One debug-log file named as the CLI names it; started is HHMMSS."""
    path = debug_dir / f"20261023T{started}.903970323Z_{agent}_{session}.jsonl"
    write_jsonl(path, records)
    return path


def write_shared(debug_dir: Path, agent="claude") -> None:
    """The shared log: first call 10:00:20, last 10:07:00."""
    write_log(
        debug_dir,
        "095959",
        [
            record("10:00:20", ["status", NAME], None, None),
            record("10:07:00", ["knowledge", "search", "--data", "{}"], None, None),
        ],
        "no-active-workflow",
        agent,
    )


def write_plan_lane(debug_dir: Path, agent="claude") -> None:
    """One plan lane: overview 60 s, discovery 120 s, architecture 180 s,
    finished 0.5 s (the finishing goto runs 500 ms). With the shared log the
    session spans 10:00:00 to 10:07:00, 420 s."""
    write_log(
        debug_dir,
        "100000",
        [
            record("10:00:00", new(), None, state("overview")),
            record("10:00:40", ["plan", "file", "read", NAME], state("overview"), state("overview"), duration_ms=200),
            record("10:01:00", goto("discovery"), state("overview"), state("discovery")),
            record("10:03:00", goto("architecture"), state("discovery"), state("architecture")),
            record("10:06:00", goto("finished"), state("architecture"), state("finished"), duration_ms=500),
        ],
        f"plan_{NAME}",
        agent,
    )
    write_shared(debug_dir, agent)


def write_two_lanes(debug_dir: Path, agent="claude") -> None:
    """Lane A 10:00-10:05 and lane B 10:01-10:06:01, running side by side."""
    write_log(
        debug_dir,
        "100000",
        [
            record("10:00:00", new(LANE_A), None, state("overview", LANE_A)),
            record("10:02:00", goto("discovery", LANE_A), state("overview", LANE_A), state("discovery", LANE_A)),
            record("10:05:00", goto("finished", LANE_A), state("discovery", LANE_A), state("finished", LANE_A)),
        ],
        f"plan_{LANE_A}",
        agent,
    )
    write_log(
        debug_dir,
        "100100",
        [
            record("10:01:00", new(LANE_B), None, state("overview", LANE_B)),
            record("10:03:00", goto("discovery", LANE_B), state("overview", LANE_B), state("discovery", LANE_B)),
            record(
                "10:06:00",
                goto("finished", LANE_B),
                state("discovery", LANE_B),
                state("finished", LANE_B),
                duration_ms=1000,
            ),
        ],
        f"plan_{LANE_B}",
        agent,
    )
    write_shared(debug_dir, agent)


def plan_steps(outputs, children):
    """The single plan lane's expected steps, with the given outputs and
    child counts."""
    return [
        {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 60.0, "output": outputs[0], "children": children[0]},
        {"step": "discovery", "started_at": "2026-10-23T10:01:00Z", "duration_s": 120.0, "output": outputs[1], "children": children[1]},
        {"step": "architecture", "started_at": "2026-10-23T10:03:00Z", "duration_s": 180.0, "output": outputs[2], "children": children[2]},
        {"step": "finished", "started_at": "2026-10-23T10:06:00Z", "duration_s": 0.5, "output": outputs[3], "children": children[3]},
    ]


NO_CHILDREN = [0, 0, 0, 0]
CHILDREN_UNKNOWN = [None] * 4
STEPS_CHILDREN_UNKNOWN = "the run's children, or the steps they were started in, are unknown"


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


# The totals the Claude log itself reports; the record adds idle_s.
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


# claude_plan_log over write_plan_lane: its messages (09:59:50, 10:00:30,
# 10:02:00, 10:05:00, 10:06:00.2) and the session log's command starts and
# ends (10:00:00, 10:00:20, 10:00:40, 10:00:40.2, 10:01:00, 10:03:00,
# 10:06:00, 10:06:00.5, 10:07:00) leave 10:03:00 to 10:05:00 as the longest
# quiet stretch.
CLAUDE_PLAN_IDLE_S = 120.0
# The same messages without a session log: 10:02:00 to 10:05:00.
CLAUDE_PLAN_IDLE_NO_SESSION_S = 180.0


def claude_plan_log():
    """A single-agent Claude plan run: one message in each step of the plan
    lane, and one before the lane started."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        assistant("09:59:50", text("hello")),  # before the lane: no step
        assistant("10:00:30", text("a" * 10)),  # overview
        assistant("10:02:00", text("b" * 25)),  # discovery
        assistant("10:05:00", text("c" * 40)),  # architecture
        assistant("10:06:00.200", text("d" * 5)),  # finished, [10:06:00, 10:06:00.5)
        claude_result(),
    ]


# len(json.dumps(input)) of each tool input used below, worked out by hand
# from the JSON text so the expected character counts stay fixed.
# {"subagent_type": "general-purpose", "prompt": "plan 20261001-user-auth", "description": "Plan A"}
LEN_LAUNCH_A = 98
# {"subagent_type": "general-purpose", "prompt": "plan 20261001-user-auth-api", "description": "Plan B"}
LEN_LAUNCH_B = 102
LEN_LAUNCH_N = 83  # {"subagent_type": "Explore", "prompt": "look around", "description": "Look around"}
LEN_LS = 17  # {"command": "ls"}
# {"command": "spektacular plan goto --data '{\"step\":\"<step>\",\"name\":\"x\"}'"}
LEN_GOTO_DISCOVERY = 85
LEN_GOTO_ARCHITECTURE = 88
LEN_GOTO_FINISHED = 84


def claude_two_lane_log():
    """An orchestrator launching one background sub-agent per lane; A's
    sub-agent launches a nested foreground one whose prompt names no spec.
    Each child's start, end and hand-back are logged as real Claude logs them."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        # main agent: both launches, to the orchestrator (98 + 102)
        assistant(
            "10:00:10",
            launch("toolu_A", "general-purpose", f"plan {LANE_A}", "Plan A"),
            launch("toolu_B", "general-purpose", f"plan {LANE_B}", "Plan B"),
        ),
        task_started("toolu_A", "task_a", background=True),
        task_started("toolu_B", "task_b", background=True),
        assistant("10:00:30", text("a" * 20), parent="toolu_A", model=HAIKU),  # A overview
        assistant("10:01:30", text("b" * 50), parent="toolu_B", model=HAIKU),  # B overview
        assistant("10:01:40", text("x" * 11), parent="toolu_X", model=HAIKU),  # no known launch: orchestrator
        assistant("10:02:30", text("a" * 30), parent="toolu_A", model=HAIKU),  # A discovery
        # A's sub-agent launches N: its own message is A's (83 chars)
        assistant("10:03:00", launch("toolu_N", "Explore", "look around", "Look around"), parent="toolu_A", model=HAIKU),
        task_started("toolu_N", "task_n", background=False, depth=2, parent_task="task_a"),
        assistant("10:04:00", text("n" * 40), parent="toolu_N", model=HAIKU),  # N's root is A: A discovery
        assistant("10:05:30", text("m" * 13)),  # main agent: orchestrator
        assistant("10:06:00.500", text("b" * 7), parent="toolu_B", model=HAIKU),  # B finished
        assistant("10:07:00", text("a" * 9), parent="toolu_A", model=HAIKU),  # after A's lane: orchestrator
        task_updated("task_n", "10:04:01"),
        task_notification("toolu_N", "task_n", 61000, "\nFound two specs.\nBoth are plans."),
        # A ends 10:07:05, after its last message; B 10:06:40. Each reports
        # its own run time from launch: 415 s and 390 s.
        task_updated("task_a", "10:07:05"),
        task_notification("toolu_A", "task_a", 415000, "Plan A written."),
        task_updated("task_b", "10:06:40"),
        task_notification("toolu_B", "task_b", 390000, "Plan B written."),
        claude_result(spawned=3, completed=3),
    ]


def plan_goto(step):
    return {"command": f"""spektacular plan goto --data '{{"step":"{step}","name":"x"}}'"""}


def bob_trajectory():
    """A Bob ATIF trajectory: no timestamps, tokens, cost or model."""

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
            agent(2, "Reading."),  # 8, overview
            agent(3, "Moving on.", plan_goto("discovery")),  # 10 + 85, overview, then discovery
            {"step_id": 4, "source": "user", "message": "continue"},
            agent(5, "Exploring the code."),  # 19, discovery
            agent(6, "", plan_goto("architecture")),  # 88, discovery, then architecture
            agent(7, "Designing."),  # 10, architecture
            agent(8, "Done.", plan_goto("finished")),  # 5 + 84, architecture, then finished
            agent(9, "Finished."),  # 9, finished
        ],
        "final_metrics": {"total_steps": 9},
    }


BOB_OUTPUTS = [
    8 + 10 + LEN_GOTO_DISCOVERY,  # 103
    19 + LEN_GOTO_ARCHITECTURE,  # 107
    10 + 5 + LEN_GOTO_FINISHED,  # 99
    9,
]


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
                "message": [{"type": "text", "text": "Listing."}],  # 8 + 17, overview
                "tool_calls": [{"tool_call_id": "c1", "function_name": "exec_command", "arguments": {"command": "ls"}}],
            },
            {
                "step_id": 3,
                "source": "agent",
                "timestamp": claude_ts("10:02:30"),
                "message": "Reading the code.",  # 17, discovery
            },
            {
                "step_id": 4,
                "source": "agent",
                "timestamp": claude_ts("10:06:30"),
                "message": "Finished.",  # after the lane's last command: no step
            },
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


# --- 1. every agent, the same shape -------------------------------------------


def shape(record):
    return (
        list(record),
        list(record["run"]),
        list(record["result"]),
        list(record["totals"]),
        list(record["subagents"]),
    )


def step_keys(record):
    return [list(s) for w in record["steps"]["workflows"] for s in w["steps"]]


def test_every_agent_gives_the_same_shape(tmp_path):
    records = {}

    paths = trial(tmp_path / "claude", agent="claude")
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    records["claude"] = run_main(paths)

    paths = trial(tmp_path / "codex", agent="codex")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(codex_trajectory()))
    write_plan_lane(paths.debug_dir, "codex")
    records["codex"] = run_main(paths)

    paths = trial(tmp_path / "codex-rollout", agent="codex")
    write_codex_rollout(paths.agent_dir)
    write_plan_lane(paths.debug_dir, "codex")
    records["codex-rollout"] = run_main(paths)

    paths = trial(tmp_path / "bob", agent="bob")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(bob_trajectory()))
    write_plan_lane(paths.debug_dir, "bob")
    records["bob"] = run_main(paths)

    paths = trial(tmp_path / "oracle", agent="oracle")
    write_plan_lane(paths.debug_dir, "oracle")
    records["oracle"] = run_main(paths)

    expected = (TOP_KEYS, RUN_KEYS, RESULT_KEYS, RECORD_TOTALS_KEYS, RECORD_SUBAGENT_KEYS)
    assert {agent: shape(r) for agent, r in records.items()} == dict.fromkeys(records, expected)
    # every step entry, for every agent, carries the same keys, children last
    assert {agent: step_keys(r) for agent, r in records.items()} == dict.fromkeys(records, [STEP_KEYS] * 4)
    assert [r["run"]["agent"] for r in records.values()] == ["claude", "codex", "codex", "bob", "oracle"]
    assert all(r["schema_version"] == 2 for r in records.values())


def test_oracle_run_has_no_agent_measures(tmp_path):
    paths = trial(tmp_path, agent="oracle")
    write_plan_lane(paths.debug_dir, "oracle")
    record = run_main(paths)
    reason = "no agent log (oracle or missing)"
    # wall time alone comes back, from the session log's span
    assert record["totals"] == {**dict.fromkeys(RECORD_TOTALS_KEYS), "wall_time_s": 420.0, **TRACE_ZERO}
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert record["run"]["model"] is None
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": reason}]
        + [{"field": f"subagents.{n}", "reason": reason} for n in SUBAGENT_KEYS]
        + [{"field": "steps.output", "reason": "no messages read from the agent's log"}]
        + [{"field": "subagents.children", "reason": reason}]
        + [{"field": "steps.children", "reason": STEPS_CHILDREN_UNKNOWN}]
        + [{"field": "totals.idle_s", "reason": "the agent's log carries no activity times"}]
    )
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([None] * 4, CHILDREN_UNKNOWN)


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


def test_record_carries_the_check_score(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
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
    paths = trial(tmp_path, junit=junit)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["result"] == {
        "reward": 1,
        "checks": {"passed": None, "total": None, "score": None},
        "groups": None,
    }
    assert unknown_fields(record) == ["result.checks", "result.groups"]
    assert reason_of(record, "result.checks").startswith(reason_start)
    # the rest of the record is untouched
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO}


def test_junit_with_no_checks_has_no_score(tmp_path):
    paths = trial(tmp_path, junit='<testsuites><testsuite name="pytest" tests="0"/></testsuites>')
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["result"]["checks"] == {"passed": 0, "total": 0, "score": None}
    assert record["result"]["groups"] == []
    assert record["unknown"] == [{"field": "result.checks.score", "reason": "pytest ran no checks"}]


# --- 3 and 5. a Claude single-lane plan run -----------------------------------


def test_claude_plan_run_record_is_complete(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record == {
        "schema_version": 2,
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
        "totals": {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO},
        "steps": {
            "unit": "characters",
            # no children: every step counts zero, not null
            "workflows": [{"kind": "plan", "name": NAME, "steps": plan_steps([10, 25, 40, 5], NO_CHILDREN)}],
        },
        "subagents": {"started": 0, "completed": 0, "failed": 0, "stopped": 0, "durations_s": [], "children": []},
        "unknown": [],
    }


def test_plan_run_step_durations_fit_in_the_wall_time(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    steps = record["steps"]["workflows"][0]["steps"]
    assert [s["step"] for s in steps] == ["overview", "discovery", "architecture", "finished"]
    assert all(isinstance(s["duration_s"], float) and isinstance(s["output"], int) for s in steps)
    assert sum(s["duration_s"] for s in steps) == 360.5
    assert sum(s["duration_s"] for s in steps) <= record["totals"]["wall_time_s"] == 400.0


# --- 4. a missing or broken input costs only its own fields -------------------


def test_missing_agent_dir_still_gives_a_record(tmp_path):
    paths = trial(tmp_path)
    paths.agent_dir.rmdir()
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    reason = "no agent log (oracle or missing)"
    assert record["totals"]["turns"] is None
    assert record["totals"]["cost_usd"] is None
    assert reason_of(record, "totals.turns") == reason
    assert reason_of(record, "subagents.started") == reason
    assert record["result"]["checks"] == CHECKS


def test_missing_build_info_leaves_run_fields_unknown(tmp_path):
    paths = trial(tmp_path, build=None)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
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
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO}


@pytest.mark.parametrize(
    "raw, reason_start",
    [
        ("{not json", "unreadable build.json: "),
        ('["plan-workflow"]', "build.json is not an object"),
    ],
    ids=["malformed", "not-an-object"],
)
def test_broken_build_info_leaves_run_fields_unknown(tmp_path, raw, reason_start):
    paths = trial(tmp_path, build=raw)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["run"]["suite"] is None and record["run"]["commit"] is None
    assert unknown_fields(record) == ["run.suite", "run.spektacular_version", "run.commit", "run.dirty"]
    assert reason_of(record, "run.suite").startswith(reason_start)


def test_build_info_missing_some_fields_lists_only_those(tmp_path):
    paths = trial(tmp_path, build={"suite": "plan-workflow", "agent": "claude", "dirty": "yes"})
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["run"]["suite"] == "plan-workflow"
    assert record["run"]["dirty"] is None  # not a bool
    assert record["unknown"] == [
        {"field": f"run.{n}", "reason": "missing from build.json"}
        for n in ("spektacular_version", "commit", "dirty")
    ]


def test_malformed_lines_in_claude_log_are_skipped(tmp_path):
    paths = trial(tmp_path)
    objs = claude_plan_log()
    lines = [json.dumps(o) for o in objs]
    lines.insert(3, "{this is not json")
    lines.insert(5, '"a bare string"')
    lines.append('{"type": "assistant", "timestamp": "2026-10-23T10:0')  # cut off mid-write
    (paths.agent_dir / "claude-code.txt").write_text("\n".join(lines) + "\n")
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO}
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([10, 25, 40, 5], NO_CHILDREN)
    assert record["unknown"] == []


def test_claude_log_of_only_malformed_lines_leaves_totals_unknown(tmp_path):
    paths = trial(tmp_path)
    (paths.agent_dir / "claude-code.txt").write_text("{garbage\nnot json\n")
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    reason = "no result event in claude-code.txt"
    # wall time falls back to the session log's span; everything else is unknown
    assert record["totals"] == {**dict.fromkeys(RECORD_TOTALS_KEYS), "wall_time_s": 420.0, **TRACE_ZERO}
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": "no result or init event in claude-code.txt"}]
        # no message carries a time, so the session log alone cannot tell idle from thinking
        + [{"field": "totals.idle_s", "reason": "claude's log carries no activity times"}]
    )
    assert record["subagents"]["children"] == []
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([0, 0, 0, 0], NO_CHILDREN)


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 file")
def test_unreadable_claude_log_still_gives_a_record(tmp_path):
    paths = trial(tmp_path)
    log = paths.agent_dir / "claude-code.txt"
    write_jsonl(log, claude_plan_log())
    log.chmod(0o000)
    write_plan_lane(paths.debug_dir)
    try:
        record = run_main(paths)
    finally:
        log.chmod(0o644)
    fields = unknown_fields(record)
    assert fields[0] == "totals"
    assert reason_of(record, "totals").startswith("recorder error: ")
    assert [f for f in fields if f.startswith("totals.")] == [
        *(f"totals.{n}" for n in TOTALS_KEYS if n != "wall_time_s"),
        "totals.idle_s",  # gathered last
    ]
    assert reason_of(record, "totals.turns") == "recorder error reading the agent log"
    assert reason_of(record, "subagents.children") == "recorder error reading the agent log"
    assert reason_of(record, "totals.idle_s") == "the agent's log carries no activity times"
    assert record["totals"] == {**dict.fromkeys(RECORD_TOTALS_KEYS), "wall_time_s": 420.0, **TRACE_ZERO}
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert record["result"]["checks"] == CHECKS


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads a mode-000 file")
def test_unreadable_junit_leaves_checks_unknown(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    paths.junit.chmod(0o000)
    try:
        record = run_main(paths)
    finally:
        paths.junit.chmod(0o644)
    assert unknown_fields(record) == ["result.checks", "result.groups"]
    assert reason_of(record, "result.checks").startswith("unreadable junit.xml: ")


def test_missing_debug_dir_leaves_steps_unknown(tmp_path):
    paths = trial(tmp_path)
    paths.debug_dir.rmdir()
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    record = run_main(paths)
    assert record["steps"] == {"unit": "characters", "workflows": None}
    assert record["unknown"] == [{"field": "steps", "reason": "no session log"}]
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_NO_SESSION_S, **TRACE_NONE}


def test_empty_debug_dir_leaves_steps_unknown(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    record = run_main(paths)
    assert record["steps"] == {"unit": "characters", "workflows": None}
    assert record["unknown"] == [{"field": "steps", "reason": "the session log is empty"}]


def test_broken_session_log_costs_only_the_steps(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    # a record with no timestamp cannot be read back
    write_log(paths.debug_dir, "100000", [{"command": new(), "exit_code": 0}], f"plan_{NAME}")
    record = run_main(paths)
    assert record["steps"] == {"unit": "characters", "workflows": None}
    assert unknown_fields(record) == ["steps"]
    assert reason_of(record, "steps").startswith("recorder error: ")
    # idle time falls back to the messages alone
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_NO_SESSION_S, **TRACE_NONE}
    assert record["result"]["checks"] == CHECKS


def test_missing_reward_is_unknown_and_not_created(tmp_path):
    paths = trial(tmp_path, reward=None)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["result"]["reward"] is None
    assert unknown_fields(record) == ["result.reward"]
    assert not paths.reward.exists()


def test_every_input_missing_still_gives_a_record(tmp_path):
    paths = trial(tmp_path, build=None, junit=None)
    paths.agent_dir.rmdir()
    paths.debug_dir.rmdir()
    record = run_main(paths)
    assert shape(record) == (TOP_KEYS, RUN_KEYS, RESULT_KEYS, RECORD_TOTALS_KEYS, RECORD_SUBAGENT_KEYS)
    assert record["totals"] == dict.fromkeys(RECORD_TOTALS_KEYS)
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert record["result"]["reward"] == 1
    fields = unknown_fields(record)
    for name in ("run.suite", "totals.wall_time_s", "subagents.started", "result.checks", "steps",
                 "subagents.children", "totals.idle_s"):
        assert name in fields


# --- Bob ----------------------------------------------------------------------


def bob_trial(tmp_path) -> Paths:
    paths = trial(tmp_path, agent="bob")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(bob_trajectory()))
    write_plan_lane(paths.debug_dir, "bob")
    return paths


def test_bob_run_wall_time_from_shell_logs(tmp_path):
    paths = bob_trial(tmp_path)
    write_bob_logs(paths.bob_logs)
    record = run_main(paths)
    assert record["run"]["agent"] == "bob"
    assert record["totals"] == {
        **dict.fromkeys(RECORD_TOTALS_KEYS),
        "wall_time_s": 782.0,  # 09:59:58.552 to 10:13:00.552
        "turns": 7,
        # the shell logs' times stand in for its messages': the longest
        # quiet stretch is the session log's last command, 10:07:00, to the
        # shell log's 10:12:30.052
        "idle_s": 330.052,
        **TRACE_ZERO,
    }
    assert record["unknown"] == [
        {"field": "totals.model_time_s", "reason": "bob reports no model time"},
        {"field": "totals.cost_usd", "reason": "bob reports no cost"},
        {"field": "totals.input_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.output_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.cache_read_tokens", "reason": "bob reports no token counts"},
        {"field": "totals.cache_write_tokens", "reason": "bob reports no cache writes"},
        {"field": "run.model", "reason": "bob reports no model"},
        {"field": "subagents.completed", "reason": "bob reports no sub-agent outcomes"},
        {"field": "subagents.failed", "reason": "bob reports no sub-agent outcomes"},
        {"field": "subagents.stopped", "reason": "bob reports no sub-agent outcomes"},
        {"field": "subagents.durations_s", "reason": "bob reports no sub-agent outcomes"},
    ]
    assert record["subagents"] == {**dict.fromkeys(RECORD_SUBAGENT_KEYS), "started": 0, "children": []}


def test_bob_run_wall_time_from_session_log_without_shell_logs(tmp_path):
    paths = bob_trial(tmp_path)
    record = run_main(paths)
    assert record["totals"]["wall_time_s"] == 420.0  # 10:00:00 to 10:07:00
    assert "totals.wall_time_s" not in unknown_fields(record)


def test_bob_run_without_any_timed_log_leaves_wall_time_unknown(tmp_path):
    paths = trial(tmp_path, agent="bob")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(bob_trajectory()))
    record = run_main(paths)
    assert record["totals"]["wall_time_s"] is None
    assert reason_of(record, "totals.wall_time_s") == "bob trajectory has no step timestamps"


def test_bob_characters_follow_its_gotos(tmp_path):
    paths = bob_trial(tmp_path)
    record = run_main(paths)
    assert record["steps"] == {
        "unit": "characters",
        "workflows": [{"kind": "plan", "name": NAME, "steps": plan_steps(BOB_OUTPUTS, NO_CHILDREN)}],
    }
    assert not [f for f in unknown_fields(record) if f.startswith("steps")]


# --- Codex --------------------------------------------------------------------


def test_codex_atif_run(tmp_path):
    paths = trial(tmp_path, agent="codex")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(codex_trajectory()))
    write_plan_lane(paths.debug_dir, "codex")
    record = run_main(paths)
    assert record["run"]["agent"] == "codex"
    assert record["run"]["model"] == "gpt-5"
    assert record["totals"] == {
        **dict.fromkeys(RECORD_TOTALS_KEYS),
        "wall_time_s": 390.0,  # 10:00:00 to 10:06:30
        "turns": 3,
        "input_tokens": 2000,  # 5000 prompt tokens, 3000 of them cached
        "output_tokens": 700,
        "cache_read_tokens": 3000,
        # its messages (10:00:30, 10:02:30, 10:06:30) among the session log's
        # commands: 10:03:00 to 10:06:00 is the longest quiet stretch
        "idle_s": 180.0,
        **TRACE_ZERO,
    }
    # the child list is unknown, not empty, and no step counts zero children
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert record["unknown"] == [
        {"field": "totals.model_time_s", "reason": "codex reports no model time"},
        {"field": "totals.cost_usd", "reason": "codex reports no cost"},
        {"field": "totals.cache_write_tokens", "reason": "codex reports no cache writes"},
    ] + [{"field": f"subagents.{n}", "reason": "codex reports no sub-agents"} for n in SUBAGENT_KEYS] + [
        {"field": "subagents.children", "reason": "codex reports no sub-agents"},
        {"field": "steps.children", "reason": STEPS_CHILDREN_UNKNOWN},
    ]
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([8 + LEN_LS, 17, 0, 0], CHILDREN_UNKNOWN)


def test_codex_rollout_run_has_no_step_outputs(tmp_path):
    paths = trial(tmp_path, agent="codex")
    write_codex_rollout(paths.agent_dir)
    write_plan_lane(paths.debug_dir, "codex")
    record = run_main(paths)
    assert record["totals"]["turns"] == 1
    assert record["totals"]["wall_time_s"] == 120.0
    assert record["subagents"] == dict.fromkeys(RECORD_SUBAGENT_KEYS)
    assert reason_of(record, "subagents.started") == "codex reports no sub-agents"
    assert reason_of(record, "subagents.children") == "codex reports no sub-agents"
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([None] * 4, CHILDREN_UNKNOWN)
    assert reason_of(record, "steps.output") == "no messages read from codex's log"
    assert reason_of(record, "steps.children") == STEPS_CHILDREN_UNKNOWN
    assert record["totals"]["idle_s"] is None
    assert reason_of(record, "totals.idle_s") == "codex's log carries no activity times"


# --- two lanes ----------------------------------------------------------------


def test_lane_for_takes_the_spec_the_prompt_names_first():
    # an epic sub-agent's prompt names its own spec, then the ones it builds on
    names = ["20260201000003-commands", "20260201000004-export"]
    prompt = "You are implementing spec `20260201000004-export`. It depends on `20260201000003-commands`."
    assert lane_for(prompt, names) == "20260201000004-export"


def test_lane_for_takes_the_longer_name_where_one_contains_another():
    assert lane_for(f"plan {LANE_B}", [LANE_A, LANE_B]) == LANE_B
    assert lane_for(f"plan {LANE_A}", [LANE_A, LANE_B]) == LANE_A
    assert lane_for("look around", [LANE_A, LANE_B]) is None


def test_two_lane_claude_run_splits_characters_by_lane(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_two_lane_log())
    write_two_lanes(paths.debug_dir)
    record = run_main(paths)
    assert record["steps"] == {
        "unit": "characters",
        "workflows": [
            {
                "kind": "plan",
                "name": LANE_A,
                "steps": [
                    # A was launched here, at 10:00:10
                    {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 120.0, "output": 20, "children": 1},
                    {
                        "step": "discovery",
                        "started_at": "2026-10-23T10:02:00Z",
                        "duration_s": 180.0,
                        "output": 30 + LEN_LAUNCH_N + 40,
                        "children": 1,  # N, nested, at 10:03:00
                    },
                    {"step": "finished", "started_at": "2026-10-23T10:05:00Z", "duration_s": 0.0, "output": 0, "children": 0},
                ],
            },
            {
                "kind": "plan",
                "name": LANE_B,
                # B was launched at 10:00:10, before its lane's first command
                "steps": [
                    {"step": "overview", "started_at": "2026-10-23T10:01:00Z", "duration_s": 120.0, "output": 50, "children": 0},
                    {"step": "discovery", "started_at": "2026-10-23T10:03:00Z", "duration_s": 180.0, "output": 0, "children": 0},
                    {"step": "finished", "started_at": "2026-10-23T10:06:00Z", "duration_s": 1.0, "output": 7, "children": 0},
                ],
            },
        ],
        # the launches, the main agent's text, the unknown launch's and the
        # message after lane A had finished
        "orchestrator_output": LEN_LAUNCH_A + LEN_LAUNCH_B + 13 + 11 + 9,
    }
    assert record["unknown"] == []


def test_two_lane_atif_run_leaves_outputs_unknown(tmp_path):
    paths = trial(tmp_path, agent="codex")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(codex_trajectory()))
    write_two_lanes(paths.debug_dir, "codex")
    record = run_main(paths)
    workflows = record["steps"]["workflows"]
    assert [w["name"] for w in workflows] == [LANE_A, LANE_B]
    assert [[(s["step"], s["duration_s"], s["output"]) for s in w["steps"]] for w in workflows] == [
        [("overview", 120.0, None), ("discovery", 180.0, None), ("finished", 0.0, None)],
        [("overview", 120.0, None), ("discovery", 180.0, None), ("finished", 1.0, None)],
    ]
    assert "orchestrator_output" not in record["steps"]
    assert reason_of(record, "steps.output") == "codex output cannot be split between concurrent lanes"


# --- child agents -------------------------------------------------------------


def test_two_lane_claude_run_records_every_child_in_its_spec_and_step(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_two_lane_log())
    write_two_lanes(paths.debug_dir)
    record = run_main(paths)
    assert record["subagents"]["children"] == [
        {
            "id": "toolu_A",
            "parent": None,
            "kind": "general-purpose",
            "description": "Plan A",
            "background": True,
            "spec": LANE_A,  # its own prompt names lane A
            "step": "overview",  # 10:00:10, in lane A's overview
            "launched_at": "2026-10-23T10:00:10Z",
            "ended_at": "2026-10-23T10:07:05Z",
            "duration_s": 415.0,
            "ending": "completed",
            "hand_back": "Plan A written.",
            "models": [HAIKU],
            "output": 20 + 30 + LEN_LAUNCH_N + 9,  # its own messages, the launch of N among them
            "prompt": f"plan {LANE_A}",
            "prompt_chars": 23,
        },
        {
            "id": "toolu_B",
            "parent": None,
            "kind": "general-purpose",
            "description": "Plan B",
            "background": True,
            "spec": LANE_B,  # the longer name wins over lane A's, which it contains
            "step": None,  # 10:00:10, before lane B's first command at 10:01:00
            "launched_at": "2026-10-23T10:00:10Z",
            "ended_at": "2026-10-23T10:06:40Z",
            "duration_s": 390.0,
            "ending": "completed",
            "hand_back": "Plan B written.",
            "models": [HAIKU],
            "output": 50 + 7,
            "prompt": f"plan {LANE_B}",
            "prompt_chars": 27,
        },
        {
            "id": "toolu_N",
            "parent": "toolu_A",
            "kind": "Explore",
            "description": "Look around",
            "background": False,
            "spec": LANE_A,  # its prompt names no spec: its parent's
            "step": "discovery",  # 10:03:00, in lane A's discovery
            "launched_at": "2026-10-23T10:03:00Z",
            "ended_at": "2026-10-23T10:04:01Z",
            "duration_s": 61.0,
            "ending": "completed",
            "hand_back": "Found two specs.",  # the summary's first non-empty line
            "models": [HAIKU],
            "output": 40,
            "prompt": "look around",
            "prompt_chars": 11,
        },
    ]
    assert [list(c) for c in record["subagents"]["children"]] == [CHILD_KEYS] * 3
    # a child started outside any step has no step, and that is not unknown
    assert record["unknown"] == []


def test_two_lane_claude_run_idle_time_spans_messages_children_and_commands(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_two_lane_log())
    write_two_lanes(paths.debug_dir)
    record = run_main(paths)
    # Messages 10:00:10 ... 10:07:00, children's starts and ends (10:00:10,
    # 10:03:00, 10:04:01, 10:06:40, 10:07:05) and the commands (10:00:00,
    # 10:00:20, 10:01:00, 10:02:00, 10:03:00, 10:05:00, 10:06:00, 10:06:01,
    # 10:07:00): the longest quiet stretch is 10:03:00 to N's message at 10:04:00.
    assert record["totals"]["idle_s"] == 60.0


def write_two_implement_lanes(debug_dir: Path) -> None:
    """Two implement lanes side by side. A: read_plan 10:00, implement 10:01,
    check_goal 10:03-10:05, finished 10:05. B: read_plan 10:00:30, implement
    10:02, check_goal 10:04-10:06, finished 10:06."""
    for started, name, clocks in (
        ("100000", LANE_A, ("10:00:00", "10:01:00", "10:03:00", "10:05:00")),
        ("100030", LANE_B, ("10:00:30", "10:02:00", "10:04:00", "10:06:00")),
    ):
        steps = ["read_plan", "implement", "check_goal", "finished"]
        records = [record(clocks[0], new(name, "implement"), None, state(steps[0], name, "implement"))]
        for clock, before, after in zip(clocks[1:], steps, steps[1:]):
            records.append(record(clock, goto(after, name, "implement"), state(before, name, "implement"), state(after, name, "implement")))
        write_log(debug_dir, started, records, f"implement_{name}")


def claude_goal_check_log():
    """An orchestrator launching one implementer per spec; each implementer
    launches a goal-check reviewer, whose prompt names no spec, during its
    check_goal step. The reviewers stream no messages of their own."""
    review = "Check whether the goal is met."
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        assistant(
            "10:00:05",
            launch("toolu_IA", "general-purpose", f"Implement spec {LANE_A}.", "Implement A"),
            # names its own spec first, then the one it builds on
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


def test_goal_reviewers_are_recorded_against_their_parents_spec_and_the_goal_check_step(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_goal_check_log())
    write_two_implement_lanes(paths.debug_dir)
    record = run_main(paths)
    children = record["subagents"]["children"]
    assert [(c["id"], c["parent"], c["spec"], c["step"]) for c in children] == [
        ("toolu_IA", None, LANE_A, "read_plan"),  # 10:00:05, in A's read_plan
        ("toolu_IB", None, LANE_B, None),  # 10:00:05, before B's first command
        ("toolu_GA", "toolu_IA", LANE_A, "check_goal"),  # 10:03:30
        ("toolu_GB", "toolu_IB", LANE_B, "check_goal"),  # 10:04:30
    ]
    assert [(w["name"], [(s["step"], s["children"]) for s in w["steps"]]) for w in record["steps"]["workflows"]] == [
        (LANE_A, [("read_plan", 1), ("implement", 0), ("check_goal", 1), ("finished", 0)]),
        (LANE_B, [("read_plan", 0), ("implement", 0), ("check_goal", 1), ("finished", 0)]),
    ]
    streamed_none = "claude streamed none of the child's messages"
    assert record["unknown"] == [
        {"field": "subagents.children[toolu_GA].models", "reason": streamed_none},
        {"field": "subagents.children[toolu_GA].output", "reason": streamed_none},
        {"field": "subagents.children[toolu_GB].models", "reason": streamed_none},
        {"field": "subagents.children[toolu_GB].output", "reason": streamed_none},
    ]
    assert [(c["models"], c["output"]) for c in children[2:]] == [(None, None), (None, None)]


def test_claude_children_without_a_session_log_have_no_spec_or_step(tmp_path):
    paths = trial(tmp_path)
    paths.debug_dir.rmdir()
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_two_lane_log())
    record = run_main(paths)
    children = record["subagents"]["children"]
    assert [(c["id"], c["spec"], c["step"]) for c in children] == [
        ("toolu_A", None, None),
        ("toolu_B", None, None),
        ("toolu_N", None, None),
    ]
    reason = "no session log to place children in"
    assert record["unknown"] == [
        {"field": "steps", "reason": "no session log"},
        {"field": "subagents.children.spec", "reason": reason},
        {"field": "subagents.children.step", "reason": reason},
    ]


# --- idle time ----------------------------------------------------------------


def clock(h, m, s):
    return datetime(2026, 10, 23, h, m, s, tzinfo=timezone.utc)


def test_idle_seconds_is_the_longest_gap_whatever_the_order():
    instants = [clock(10, 2, 5), clock(10, 0, 0), clock(10, 2, 10), clock(10, 0, 5)]
    # 10:00:00, 10:00:05, 10:02:05, 10:02:10: gaps of 5, 120 and 5 s
    assert idle_seconds(instants) == 120.0


def test_idle_seconds_needs_two_instants():
    assert idle_seconds([]) is None
    assert idle_seconds([clock(10, 0, 0)]) is None


def test_run_with_one_timed_message_has_idle_time_unknown(tmp_path):
    paths = trial(tmp_path)
    paths.debug_dir.rmdir()
    write_jsonl(
        paths.agent_dir / "claude-code.txt",
        [{"type": "system", "subtype": "init", "model": SONNET}, assistant("10:00:30", text("hi")), claude_result()],
    )
    record = run_main(paths)
    assert record["totals"]["idle_s"] is None
    assert record["unknown"] == [
        {"field": "steps", "reason": "no session log"},
        {"field": "totals.idle_s", "reason": "too little activity to measure"},
    ]


# --- Bob's children -----------------------------------------------------------


def bob_children_trajectory(second_args=None, first_prompt="Map the repo and list its specs."):
    """A Bob trajectory with two sub-agent calls, one in overview and one in
    discovery once a goto has moved it on. Bob logs no times."""

    def call(tid, title, args):
        return {"tool_call_id": tid, "function_name": f"Running subagent: {title}", "arguments": args}

    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "bob", "version": "2.0.6"},
        "steps": [
            {"step_id": 1, "source": "user", "message": "Create a plan"},
            {"step_id": 2, "source": "agent", "message": "Reading.",
             "tool_calls": [call("b1", "map the repo", {"name": "explore", "description": first_prompt})]},
            {"step_id": 3, "source": "agent", "message": "Moving on.",
             "tool_calls": [{"tool_call_id": "g1", "function_name": "execute", "arguments": plan_goto("discovery")}]},
            {"step_id": 4, "source": "agent", "message": "",
             "tool_calls": [call("b3", "write tests", second_args or {})]},
            {"step_id": 5, "source": "agent", "message": "Done.",
             "tool_calls": [{"tool_call_id": "g2", "function_name": "execute", "arguments": plan_goto("architecture")}]},
        ],
        "final_metrics": {"total_steps": 5},
    }


def child_tail(record):
    """The unknowns this record's child, step-count and idle sections added."""
    return [u for u in record["unknown"] if u["field"].startswith(("subagents.children", "steps.children", "totals.idle_s"))]


BOB_CHILD_GAPS = [
    ("parent", "sub-agent parent"),
    ("background", "sub-agent background state"),
    ("launched_at", "sub-agent start time"),
    ("ended_at", "sub-agent end time"),
    ("duration_s", "sub-agent duration"),
    ("ending", "sub-agent outcome"),
    ("hand_back", "sub-agent hand-back"),
    ("models", "sub-agent models"),
    ("output", "sub-agent output"),
]


def test_bob_children_are_placed_by_its_gotos_and_their_gaps_are_unknown(tmp_path):
    paths = trial(tmp_path, agent="bob")
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(bob_children_trajectory()))
    write_plan_lane(paths.debug_dir, "bob")
    record = run_main(paths)
    unshown = dict.fromkeys(("parent", "background", "launched_at", "ended_at", "duration_s", "ending", "hand_back", "models", "output"))
    assert record["subagents"]["children"] == [
        {"id": "b1", **unshown, "kind": "explore", "description": "map the repo", "spec": NAME, "step": "overview",
         "prompt": "Map the repo and list its specs.", "prompt_chars": 32},
        {"id": "b3", **unshown, "kind": None, "description": "write tests", "spec": NAME, "step": "discovery",
         "prompt": None, "prompt_chars": None},
    ]
    assert [s["children"] for s in record["steps"]["workflows"][0]["steps"]] == [1, 1, 0, 0]
    no_prompt = "the bob sub-agent call named no prompt"
    assert child_tail(record) == [
        *({"field": f"subagents.children.{f}", "reason": f"bob logs no {what}"} for f, what in BOB_CHILD_GAPS),
        {"field": "subagents.children[b3].kind", "reason": "the bob sub-agent call named no kind"},
        {"field": "subagents.children[b3].prompt", "reason": no_prompt},
        {"field": "subagents.children[b3].prompt_chars", "reason": no_prompt},
        # Bob's messages carry no time and no shell logs were left
        {"field": "totals.idle_s", "reason": "bob's log carries no activity times"},
    ]
    assert record["totals"]["idle_s"] is None


def test_bob_children_in_concurrent_lanes_leave_steps_and_counts_unknown(tmp_path):
    paths = trial(tmp_path, agent="bob")
    trajectory = bob_children_trajectory(
        second_args={"name": "general", "description": f"Write tests for {LANE_A}"},
        first_prompt=f"Map the repo for {LANE_B}",
    )
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(trajectory))
    write_two_lanes(paths.debug_dir, "bob")
    record = run_main(paths)
    assert [(c["id"], c["spec"], c["step"]) for c in record["subagents"]["children"]] == [
        ("b1", LANE_B, None),
        ("b3", LANE_A, None),
    ]
    # no child count is written as zero when the children's steps are unknown
    assert [[s["children"] for s in w["steps"]] for w in record["steps"]["workflows"]] == [[None] * 3, [None] * 3]
    assert child_tail(record) == [
        *({"field": f"subagents.children.{f}", "reason": f"bob logs no {what}"} for f, what in BOB_CHILD_GAPS),
        {"field": "subagents.children.step", "reason": "the child has no launch time to place it in a step"},
        {"field": "steps.children", "reason": STEPS_CHILDREN_UNKNOWN},
        {"field": "totals.idle_s", "reason": "bob's log carries no activity times"},
    ]


def test_bob_child_naming_no_spec_in_concurrent_lanes_has_its_step_unknown(tmp_path):
    # Without a launch time it may have been started in either lane's step, so
    # its null step is listed unknown rather than read as "outside any step".
    paths = trial(tmp_path, agent="bob")
    trajectory = bob_children_trajectory(
        second_args={"name": "general", "description": f"Write tests for {LANE_A}"},
        first_prompt="Map the repo",
    )
    (paths.agent_dir / "trajectory.json").write_text(json.dumps(trajectory))
    write_two_lanes(paths.debug_dir, "bob")
    record = run_main(paths)
    assert [(c["id"], c["spec"], c["step"]) for c in record["subagents"]["children"]] == [
        ("b1", None, None),
        ("b3", LANE_A, None),
    ]
    assert {"field": "subagents.children.step", "reason": "the child has no launch time to place it in a step"} in record["unknown"]
    assert {"field": "steps.children", "reason": STEPS_CHILDREN_UNKNOWN} in record["unknown"]


# --- a broken child or idle section costs only itself -------------------------


def test_raising_child_reader_costs_only_the_children(tmp_path, monkeypatch):
    def boom(objs):
        raise RuntimeError("bad child event")

    monkeypatch.setattr(metrics.tx, "claude_children", boom)
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)  # returns 0 and leaves the reward untouched
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO}
    assert record["subagents"] == {"started": 0, "completed": 0, "failed": 0, "stopped": 0, "durations_s": [], "children": None}
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([10, 25, 40, 5], CHILDREN_UNKNOWN)
    assert record["unknown"] == [
        {"field": "subagents.children", "reason": "recorder error: bad child event"},
        {"field": "steps.children", "reason": STEPS_CHILDREN_UNKNOWN},
    ]


def test_raising_idle_computation_costs_only_the_idle_time(tmp_path, monkeypatch):
    def boom(instants):
        raise ValueError("bad instant")

    monkeypatch.setattr(metrics, "idle_seconds", boom)
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["result"] == {"reward": 1, "checks": CHECKS, "groups": GROUPS}
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": None, **TRACE_ZERO}
    assert record["subagents"]["children"] == []
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([10, 25, 40, 5], NO_CHILDREN)
    assert record["unknown"] == [{"field": "totals.idle_s", "reason": "recorder error: bad instant"}]


# --- which logs count as workflows --------------------------------------------


def test_spec_lane_is_a_workflow(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_log(
        paths.debug_dir,
        "100000",
        [
            record("10:00:00", new(kind="spec"), None, state("overview", kind="spec")),
            record("10:04:00", goto("finished", kind="spec"), state("overview", kind="spec"), state("finished", kind="spec")),
        ],
        f"spec_{NAME}",
    )
    record_ = run_main(paths)
    assert record_["steps"]["workflows"] == [
        {
            "kind": "spec",
            "name": NAME,
            "steps": [
                {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 240.0, "output": 10 + 25, "children": 0},
                # the 10:05:00 message falls after the lane's last command

                {"step": "finished", "started_at": "2026-10-23T10:04:00Z", "duration_s": 0.0, "output": 0, "children": 0},
            ],
        }
    ]
    assert record_["unknown"] == []


def test_session_log_without_a_lane_gives_no_workflows(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_shared(paths.debug_dir)
    record = run_main(paths)
    assert record["steps"] == {"unit": "characters", "workflows": []}
    assert not [f for f in unknown_fields(record) if f.startswith("steps")]
    assert record["unknown"] == []


# --- agent mismatch -----------------------------------------------------------


def test_agent_log_not_matching_build_info_is_flagged(tmp_path):
    paths = trial(tmp_path, agent="codex")
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    assert record["run"]["agent"] == "codex"
    assert record["unknown"] == [
        {"field": "run.agent", "reason": "the agent log is claude's but build.json names codex"}
    ]
    assert record["totals"] == {**CLAUDE_TOTALS, "idle_s": CLAUDE_PLAN_IDLE_S, **TRACE_ZERO}


# --- writing the record -------------------------------------------------------


def test_write_record_is_world_readable_json(tmp_path):
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
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
    paths = trial(tmp_path)
    write_jsonl(paths.agent_dir / "claude-code.txt", claude_plan_log())
    write_plan_lane(paths.debug_dir)
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


# --- trace calls (spec 000074) -------------------------------------------------------


def test_trace_calls_are_counted_from_the_session_log_per_task(tmp_path):
    from metrics import read_trace_calls
    from sessionlog import SessionLog

    def rec(kind, clock, **extra):
        r = {"record": kind, "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "span_id": extra.pop("span_id", "c" * 16),
             "name": extra.pop("name", "x"), "kind": "internal", "start_time": f"2026-10-23T{clock}.000000000Z"}
        r.update(extra)
        return r

    def cmd(clock, args):
        return rec("span", clock, end_time=f"2026-10-23T{clock}.100000000Z", status={"code": "ok"},
                   attributes={"spektacular.span.type": "command", "process.command_args": args, "process.exit_code": 0})

    def task(clock, span_id):
        return rec("span_start", clock, span_id=span_id, name="Task", attributes={"spektacular.span.type": "task"})

    lines = [
        cmd("10:00:00", ["implement", "new", "--data", "{}"]),
        task("10:00:01", "a" * 16),
        cmd("10:01:00", ["trace", "start", "test-writer", "--type", "sub-agent"]),
        cmd("10:02:00", ["trace", "stop", "x-y"]),
        task("10:03:00", "b" * 16),
        cmd("10:04:00", ["trace", "start", "reviewer", "--type", "sub-agent"]),
        cmd("10:05:00", ["implement", "goto", "--data", "{}"]),
    ]
    (tmp_path / "20261023T100000.000000000Z_claude_implement_x.jsonl").write_text("".join(json.dumps(r) + "\n" for r in lines))
    totals, unknowns = read_trace_calls(SessionLog(tmp_path), tmp_path)
    assert totals == {"trace_calls": 3, "trace_calls_per_task": 1.5}
    assert unknowns == []


def test_trace_calls_without_tasks_have_no_average(tmp_path):
    from metrics import read_trace_calls
    from sessionlog import SessionLog

    assert read_trace_calls(None, tmp_path) == ({"trace_calls": None, "trace_calls_per_task": None}, [])
    (tmp_path / "standalone_20261023.jsonl").write_text("")
    assert read_trace_calls(SessionLog(tmp_path), tmp_path)[0]["trace_calls_per_task"] is None
