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
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from metrics import Paths, build_record, lane_for, main, score_checks, write_record  # noqa: E402

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


def plan_steps(outputs):
    """The single plan lane's expected steps, with the given outputs."""
    return [
        {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 60.0, "output": outputs[0]},
        {"step": "discovery", "started_at": "2026-10-23T10:01:00Z", "duration_s": 120.0, "output": outputs[1]},
        {"step": "architecture", "started_at": "2026-10-23T10:03:00Z", "duration_s": 180.0, "output": outputs[2]},
        {"step": "finished", "started_at": "2026-10-23T10:06:00Z", "duration_s": 0.5, "output": outputs[3]},
    ]


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


def launch(tid, subagent_type, prompt):
    return {"type": "tool_use", "id": tid, "name": "Agent", "input": {"subagent_type": subagent_type, "prompt": prompt}}


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
LEN_LAUNCH_A = 73  # {"subagent_type": "general-purpose", "prompt": "plan 20261001-user-auth"}
LEN_LAUNCH_B = 77  # {"subagent_type": "general-purpose", "prompt": "plan 20261001-user-auth-api"}
LEN_LAUNCH_N = 53  # {"subagent_type": "Explore", "prompt": "look around"}
LEN_LS = 17  # {"command": "ls"}
# {"command": "spektacular plan goto --data '{\"step\":\"<step>\",\"name\":\"x\"}'"}
LEN_GOTO_DISCOVERY = 85
LEN_GOTO_ARCHITECTURE = 88
LEN_GOTO_FINISHED = 84


def claude_two_lane_log():
    """An orchestrator launching one sub-agent per lane; A's sub-agent
    launches a nested one."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        # main agent: both launches, to the orchestrator (73 + 77)
        assistant(
            "10:00:10",
            launch("toolu_A", "general-purpose", f"plan {LANE_A}"),
            launch("toolu_B", "general-purpose", f"plan {LANE_B}"),
        ),
        assistant("10:00:30", text("a" * 20), parent="toolu_A", model=HAIKU),  # A overview
        assistant("10:01:30", text("b" * 50), parent="toolu_B", model=HAIKU),  # B overview
        assistant("10:01:40", text("x" * 11), parent="toolu_X", model=HAIKU),  # no known launch: orchestrator
        assistant("10:02:30", text("a" * 30), parent="toolu_A", model=HAIKU),  # A discovery
        # A's sub-agent launches N: its own message is A's (53 chars)
        assistant("10:03:00", launch("toolu_N", "Explore", "look around"), parent="toolu_A", model=HAIKU),
        assistant("10:04:00", text("n" * 40), parent="toolu_N", model=HAIKU),  # N's root is A: A discovery
        assistant("10:05:30", text("m" * 13)),  # main agent: orchestrator
        assistant("10:06:00.500", text("b" * 7), parent="toolu_B", model=HAIKU),  # B finished
        assistant("10:07:00", text("a" * 9), parent="toolu_A", model=HAIKU),  # after A's lane: orchestrator
        {"type": "system", "subtype": "task_notification", "tool_use_id": "toolu_N", "status": "completed", "usage": {"duration_ms": 61000}},
        {"type": "system", "subtype": "task_notification", "tool_use_id": "toolu_A", "status": "completed", "usage": {"duration_ms": 300000}},
        {"type": "system", "subtype": "task_notification", "tool_use_id": "toolu_B", "status": "completed", "usage": {"duration_ms": 330000}},
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

    expected = (TOP_KEYS, RUN_KEYS, RESULT_KEYS, TOTALS_KEYS, SUBAGENT_KEYS)
    assert {agent: shape(r) for agent, r in records.items()} == dict.fromkeys(records, expected)
    assert [r["run"]["agent"] for r in records.values()] == ["claude", "codex", "codex", "bob", "oracle"]
    assert all(r["schema_version"] == 1 for r in records.values())


def test_oracle_run_has_no_agent_measures(tmp_path):
    paths = trial(tmp_path, agent="oracle")
    write_plan_lane(paths.debug_dir, "oracle")
    record = run_main(paths)
    reason = "no agent log (oracle or missing)"
    # wall time alone comes back, from the session log's span
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": 420.0}
    assert record["subagents"] == dict.fromkeys(SUBAGENT_KEYS)
    assert record["run"]["model"] is None
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": reason}]
        + [{"field": f"subagents.{n}", "reason": reason} for n in SUBAGENT_KEYS]
        + [{"field": "steps.output", "reason": "no messages read from the agent's log"}]
    )
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([None] * 4)


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
    assert record["totals"] == CLAUDE_TOTALS


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
        "schema_version": 1,
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
        "totals": CLAUDE_TOTALS,
        "steps": {
            "unit": "characters",
            "workflows": [{"kind": "plan", "name": NAME, "steps": plan_steps([10, 25, 40, 5])}],
        },
        "subagents": {"started": 0, "completed": 0, "failed": 0, "stopped": 0, "durations_s": []},
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
    assert record["totals"] == CLAUDE_TOTALS


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
    assert record["totals"] == CLAUDE_TOTALS
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([10, 25, 40, 5])
    assert record["unknown"] == []


def test_claude_log_of_only_malformed_lines_leaves_totals_unknown(tmp_path):
    paths = trial(tmp_path)
    (paths.agent_dir / "claude-code.txt").write_text("{garbage\nnot json\n")
    write_plan_lane(paths.debug_dir)
    record = run_main(paths)
    reason = "no result event in claude-code.txt"
    # wall time falls back to the session log's span; everything else is unknown
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": 420.0}
    assert record["unknown"] == (
        [{"field": f"totals.{n}", "reason": reason} for n in TOTALS_KEYS if n != "wall_time_s"]
        + [{"field": "run.model", "reason": "no result or init event in claude-code.txt"}]
    )
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([0, 0, 0, 0])


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
    assert [f for f in fields if f.startswith("totals.")] == [f"totals.{n}" for n in TOTALS_KEYS if n != "wall_time_s"]
    assert reason_of(record, "totals.turns") == "recorder error reading the agent log"
    assert record["totals"] == {**dict.fromkeys(TOTALS_KEYS), "wall_time_s": 420.0}
    assert record["subagents"] == dict.fromkeys(SUBAGENT_KEYS)
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
    assert record["totals"] == CLAUDE_TOTALS


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
    assert record["totals"] == CLAUDE_TOTALS
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
    assert shape(record) == (TOP_KEYS, RUN_KEYS, RESULT_KEYS, TOTALS_KEYS, SUBAGENT_KEYS)
    assert record["totals"] == dict.fromkeys(TOTALS_KEYS)
    assert record["result"]["reward"] == 1
    fields = unknown_fields(record)
    for name in ("run.suite", "totals.wall_time_s", "subagents.started", "result.checks", "steps"):
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
        **dict.fromkeys(TOTALS_KEYS),
        "wall_time_s": 782.0,  # 09:59:58.552 to 10:13:00.552
        "turns": 7,
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
    assert record["subagents"] == {**dict.fromkeys(SUBAGENT_KEYS), "started": 0}


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
        "workflows": [{"kind": "plan", "name": NAME, "steps": plan_steps(BOB_OUTPUTS)}],
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
        **dict.fromkeys(TOTALS_KEYS),
        "wall_time_s": 390.0,  # 10:00:00 to 10:06:30
        "turns": 3,
        "input_tokens": 2000,  # 5000 prompt tokens, 3000 of them cached
        "output_tokens": 700,
        "cache_read_tokens": 3000,
    }
    assert record["subagents"] == dict.fromkeys(SUBAGENT_KEYS)
    assert record["unknown"] == [
        {"field": "totals.model_time_s", "reason": "codex reports no model time"},
        {"field": "totals.cost_usd", "reason": "codex reports no cost"},
        {"field": "totals.cache_write_tokens", "reason": "codex reports no cache writes"},
    ] + [{"field": f"subagents.{n}", "reason": "codex reports no sub-agents"} for n in SUBAGENT_KEYS]
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([8 + LEN_LS, 17, 0, 0])


def test_codex_rollout_run_has_no_step_outputs(tmp_path):
    paths = trial(tmp_path, agent="codex")
    write_codex_rollout(paths.agent_dir)
    write_plan_lane(paths.debug_dir, "codex")
    record = run_main(paths)
    assert record["totals"]["turns"] == 1
    assert record["totals"]["wall_time_s"] == 120.0
    assert record["subagents"] == dict.fromkeys(SUBAGENT_KEYS)
    assert reason_of(record, "subagents.started") == "codex reports no sub-agents"
    assert record["steps"]["workflows"][0]["steps"] == plan_steps([None] * 4)
    assert reason_of(record, "steps.output") == "no messages read from codex's log"


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
                    {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 120.0, "output": 20},
                    {
                        "step": "discovery",
                        "started_at": "2026-10-23T10:02:00Z",
                        "duration_s": 180.0,
                        "output": 30 + LEN_LAUNCH_N + 40,
                    },
                    {"step": "finished", "started_at": "2026-10-23T10:05:00Z", "duration_s": 0.0, "output": 0},
                ],
            },
            {
                "kind": "plan",
                "name": LANE_B,
                "steps": [
                    {"step": "overview", "started_at": "2026-10-23T10:01:00Z", "duration_s": 120.0, "output": 50},
                    {"step": "discovery", "started_at": "2026-10-23T10:03:00Z", "duration_s": 180.0, "output": 0},
                    {"step": "finished", "started_at": "2026-10-23T10:06:00Z", "duration_s": 1.0, "output": 7},
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
                {"step": "overview", "started_at": "2026-10-23T10:00:00Z", "duration_s": 240.0, "output": 10 + 25},
                # the 10:05:00 message falls after the lane's last command

                {"step": "finished", "started_at": "2026-10-23T10:04:00Z", "duration_s": 0.0, "output": 0},
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
    assert record["totals"] == CLAUDE_TOTALS


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
