"""Self-check for the run-summary readers in transcripts.py.

Each agent leaves a different record of a run: Claude's stream-json
transcript with its `result` events, the ATIF trajectory harbor writes for
Bob and Codex, and Codex's native rollout. The readers turn each into the
same totals, messages and sub-agent counts, and must report whatever a log
does not supply as None with an Unknown, never as zero. Each reader is
exercised here on a small hand-built log with hand-worked expected values,
and the readers the existing verifiers already use are pinned on the same
input so that the new code cannot have changed them.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider tests/harbor/shared/tests/test_transcripts_summary.py
"""

import json
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from transcripts import (  # noqa: E402
    Child,
    Message,
    SubagentLaunch,
    Subagents,
    Totals,
    Turn,
    Unknown,
    Wakeup,
    acp_subagent_launches,
    atif_children,
    atif_messages,
    atif_subagents,
    atif_summary,
    claude_children,
    claude_launch_root,
    claude_main_models,
    claude_messages,
    claude_subagent_launches,
    claude_subagents,
    claude_summary,
    claude_turns,
    claude_wakeups,
    codex_rollout_summary,
    goto_step_of,
    parse_timestamp,
)

SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"

# A goto as agents really run it: JSON data in single quotes, stderr folded in.
GOTO_DISCOVERY = """spektacular plan goto --data '{"step":"discovery","name":"x"}' 2>&1"""
GOTO_VERIFICATION = """spektacular plan goto --data '{"step":"verification","name":"x"}'"""
GOTO_HELP = "spektacular spec goto --help"

# len(json.dumps(input)) of each tool input used below, worked out by hand
# from the escaped JSON text so the expected character counts stay fixed.
LEN_GOTO_DISCOVERY = 90  # {"command": "spektacular plan goto --data '{\"step\"...}' 2>&1"}
LEN_GOTO_VERIFICATION = 88
LEN_GOTO_HELP = 43  # {"command": "spektacular spec goto --help"}
LEN_FIND_SPECS = 60  # {"subagent_type": "general-purpose", "prompt": "find specs"}
LEN_LS = 17  # {"command": "ls"}
LEN_EXPLORE = 50  # {"name": "explore", "description": "map the repo"}
LEN_EMPTY = 2  # {}


def ts(clock: str) -> str:
    """A fixed synthetic ISO timestamp on the test day, as Claude writes it."""
    return f"2026-10-23T{clock}Z"


def at(hour: int, minute: int, second: int) -> datetime:
    return datetime(2026, 10, 23, hour, minute, second, tzinfo=timezone.utc)


def init():
    return {"type": "system", "subtype": "init", "model": SONNET}


def assistant(clock, *content, parent=None, model=SONNET):
    return {
        "type": "assistant",
        "timestamp": ts(clock),
        "parent_tool_use_id": parent,
        "message": {"model": model, "role": "assistant", "content": list(content)},
    }


def user_result(clock, tool_use_id):
    return {
        "type": "user",
        "timestamp": ts(clock),
        "parent_tool_use_id": None,
        "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": tool_use_id, "content": "ok"}]},
    }


def tool_use(tid, name, inp):
    return {"type": "tool_use", "id": tid, "name": name, "input": inp}


def launch(tid, subagent_type, prompt):
    return tool_use(tid, "Agent", {"subagent_type": subagent_type, "prompt": prompt})


def notification(tool_use_id, status, duration_ms=None, summary=None):
    note = {"type": "system", "subtype": "task_notification", "tool_use_id": tool_use_id, "status": status}
    if duration_ms is not None:
        note["usage"] = {"total_tokens": 1000, "tool_uses": 3, "duration_ms": duration_ms}
    if summary is not None:
        note["summary"] = summary
    return note


def stats(spawned, completed, failed, killed_parent=0, killed_user=0, killed_system=0):
    return {
        "spawned": spawned,
        "completed": completed,
        "failed": failed,
        "killed": {"parent": killed_parent, "user": killed_user, "system": killed_system},
    }


def result(num_turns, duration_ms, duration_api_ms, cost, usage, subagent_stats=None, origin=None):
    out = {
        "type": "result",
        "subtype": "success",
        "num_turns": num_turns,
        "duration_ms": duration_ms,
        "duration_api_ms": duration_api_ms,
        "total_cost_usd": cost,
        "modelUsage": {
            model: {
                "inputTokens": i,
                "outputTokens": o,
                "cacheReadInputTokens": r,
                "cacheCreationInputTokens": w,
                "costUSD": 0.0,
            }
            for model, (i, o, r, w) in usage.items()
        },
    }
    if subagent_stats is not None:
        out["subagent_stats"] = subagent_stats
    if origin is not None:
        out["origin"] = {"kind": origin}
    return out


def claude_one_result():
    """A Claude run with one `result` event and one foreground sub-agent."""
    return [
        init(),
        assistant(
            "10:00:00",
            {"type": "thinking", "thinking": "plan it"},  # 7 chars
            {"type": "text", "text": "Starting."},  # 9 chars
            tool_use("tu_goto", "Bash", {"command": GOTO_DISCOVERY}),
        ),
        user_result("10:00:01", "tu_goto"),
        assistant("10:00:05", launch("toolu_A", "general-purpose", "find specs")),
        assistant(
            "10:00:06",
            {"type": "text", "text": "looking"},  # 7 chars
            tool_use("tu_help", "Bash", {"command": GOTO_HELP}),
            parent="toolu_A",
            model=HAIKU,
        ),
        notification("toolu_A", "completed", 14000),
        user_result("10:00:20", "toolu_A"),
        result(
            num_turns=7,
            duration_ms=61000,
            duration_api_ms=42500,
            cost=0.25,
            usage={SONNET: (100, 2000, 30000, 4000), HAIKU: (20, 500, 6000, 1000)},
            subagent_stats=stats(spawned=1, completed=1, failed=0),
        ),
    ]


def claude_split(with_stats=True):
    """A Claude run split by background sub-agents into two `result` events.

    Three top-level launches (A, B, C) and one nested launch (N, from inside
    A). A completes with a duration, B is killed without one, C fails with
    one, and N never reports. Timestamps run from 10:00:00 to 10:03:30.
    """
    s1 = stats(spawned=2, completed=0, failed=0) if with_stats else None
    s2 = stats(spawned=4, completed=2, failed=1, killed_parent=1) if with_stats else None
    return [
        init(),
        assistant(
            "10:00:00",
            launch("toolu_A", "general-purpose", "find specs"),
            launch("toolu_B", "Explore", "dig deeper"),
        ),
        assistant("10:00:30", launch("toolu_N", "Explore", "nested look"), parent="toolu_A", model=HAIKU),
        assistant("10:00:40", {"type": "text", "text": "nested"}, parent="toolu_N", model=HAIKU),
        result(4, 5000, 3000, 0.1, {SONNET: (50, 800, 9000, 2000)}, s1),
        notification("toolu_A", "completed", 14000),
        user_result("10:01:00", "toolu_A"),
        assistant("10:02:00", launch("toolu_C", "general-purpose", "write report")),
        notification("toolu_B", "killed"),
        notification("toolu_C", "failed", 2500),
        user_result("10:03:30", "toolu_C"),
        result(3, 8000, 90000, 0.3, {SONNET: (300, 4000, 50000, 7000)}, s2),
    ]


def bob_trajectory():
    """A Bob ATIF trajectory: no timestamps, no tokens, no cost, no model."""
    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "bob", "version": "2.0.6"},
        "steps": [
            {"step_id": 1, "source": "user", "message": "Create a plan"},
            {
                "step_id": 2,
                "source": "agent",
                "message": "I will start.",  # 13 chars
                "reasoning_content": "think",  # 5 chars
                "tool_calls": [{"tool_call_id": "t1", "function_name": "execute", "arguments": {"command": GOTO_VERIFICATION}}],
            },
            {
                "step_id": 3,
                "source": "agent",
                "message": "",
                "tool_calls": [
                    {
                        "tool_call_id": "t2",
                        "function_name": "Running subagent: map the repo",
                        "arguments": {"name": "explore", "description": "map the repo"},
                    }
                ],
            },
            {"step_id": 4, "source": "user", "message": "continue"},
            {
                "step_id": 5,
                "source": "agent",
                "message": "Done.",  # 5 chars
                "tool_calls": [{"tool_call_id": "t3", "function_name": "Running subagent: write tests", "arguments": {}}],
            },
        ],
        "final_metrics": {"total_steps": 5},
    }


def codex_trajectory():
    """A Codex ATIF trajectory: timestamps, token totals, a model, no cost."""
    return {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "codex", "model_name": "gpt-5"},
        "steps": [
            {"step_id": 1, "source": "user", "timestamp": ts("10:00:00"), "message": "go"},
            {
                "step_id": 2,
                "source": "agent",
                "timestamp": ts("10:00:04"),
                "message": [{"type": "text", "text": "Listing."}],  # 8 chars
                "tool_calls": [{"tool_call_id": "c1", "function_name": "exec_command", "arguments": {"command": "ls"}}],
            },
            {
                "step_id": 3,
                "source": "agent",
                "timestamp": ts("10:01:30"),
                "message": "Finished.",  # 9 chars
                "reasoning_content": "ok",  # 2 chars
            },
        ],
        "final_metrics": {
            "total_prompt_tokens": 5000,
            "total_completion_tokens": 700,
            "total_cached_tokens": 3000,
            "total_steps": 3,
        },
    }


# Child agents, turns and wake-ups. Claude's system events carry no timestamp,
# as in real logs; task_updated carries only its task id and patch.

LEN_NESTED_LOOK = 53  # {"subagent_type": "Explore", "prompt": "nested look"}

# A multi-line prompt with indentation and a trailing newline, which a child's
# context must keep exactly as launched: 77 characters.
PROMPT_A = "Find every spec under .spektacular/specs.\n\n  Report the names, one per line.\n"
LEN_PROMPT_A = 77

# A foreground child's report as the harness frames it in the launch's result.
FRAMED_HAND_BACK = (
    "[Subagent hand-back] The agent finished its work. The report follows:\n"
    "  line1\n"
    "  line2"
)

# A sleeping shell command longer than the 80 characters a wake-up keeps.
LONG_SLEEP = """sleep 300 && spektacular implement status --data '{"name":"000071_harbor-child-agents"}' 2>&1"""
LONG_SLEEP_80 = """sleep 300 && spektacular implement status --data '{"name":"000071_harbor-child-a"""


def ms(hour: int, minute: int, second: int) -> int:
    """The epoch milliseconds task_updated writes for a time on the test day."""
    return int(at(hour, minute, second).timestamp() * 1000)


def agent_launch(tid, subagent_type, prompt, description=None, background=None):
    inp = {"subagent_type": subagent_type, "prompt": prompt}
    if description is not None:
        inp["description"] = description
    if background is not None:
        inp["run_in_background"] = background
    return tool_use(tid, "Agent", inp)


def launch_result(clock, tool_use_id, text, is_error=False, tool_use_result=None, parent=None):
    block = {"type": "tool_result", "tool_use_id": tool_use_id, "content": [{"type": "text", "text": text}]}
    if is_error:
        block["is_error"] = True
    out = {
        "type": "user",
        "timestamp": ts(clock),
        "parent_tool_use_id": parent,
        "message": {"role": "user", "content": [block]},
    }
    if tool_use_result is not None:
        out["tool_use_result"] = tool_use_result
    return out


def task_started(tool_use_id, task_id, background, depth, parent_task_id=None, task_type="local_agent", prompt=None):
    out = {
        "type": "system",
        "subtype": "task_started",
        "task_id": task_id,
        "tool_use_id": tool_use_id,
        "task_type": task_type,
        "is_backgrounded": background,
        "depth": depth,
    }
    if parent_task_id is not None:
        out["parent_task_id"] = parent_task_id
    if prompt is not None:
        out["prompt"] = prompt
    return out


def task_updated(task_id, status, end_ms, error=None):
    patch = {"status": status, "end_time": end_ms}
    if error is not None:
        patch["error"] = error
    return {"type": "system", "subtype": "task_updated", "task_id": task_id, "patch": patch}


def bash(tid, command, background=False):
    inp = {"command": command}
    if background:
        inp["run_in_background"] = True
    return tool_use(tid, "Bash", inp)


def schedule_wakeup(tid, delay_seconds):
    return tool_use(tid, "ScheduleWakeup", {"delaySeconds": delay_seconds, "reason": "wait for the child"})


def claude_children_log():
    """A Claude run with top-level and nested children.

    At 10:00:00 the main agent launches A (background), R (refused: no
    task_started) and F (foreground). A's launch returns at once as async and
    A never reports back. A starts the nested N, which completes after 5s of
    its own run time. F's framed report comes back as its launch's result.
    At 10:01:00 a background shell task starts alongside K (background,
    killed) and X (foreground, fails without a duration).
    """
    return [
        init(),
        assistant(
            "10:00:00",
            agent_launch("toolu_A", "general-purpose", PROMPT_A, description="Find specs", background=True),
            agent_launch("toolu_R", "no-such-agent", "never runs", description="Refused"),
            agent_launch("toolu_F", "Explore", "look around", description="Look around"),
        ),
        launch_result("10:00:01", "toolu_R", "Agent type 'no-such-agent' not found", is_error=True),
        task_started("toolu_A", "task_a", background=True, depth=1, prompt=PROMPT_A),
        launch_result(
            "10:00:01",
            "toolu_A",
            "Async agent launched successfully.",
            tool_use_result={"isAsync": True, "status": "async_launched", "agentId": "task_a"},
        ),
        task_started("toolu_F", "task_f", background=False, depth=1),
        assistant(
            "10:00:10",
            {"type": "text", "text": "nested"},  # 6 chars
            launch("toolu_N", "Explore", "nested look"),
            parent="toolu_A",
            model=HAIKU,
        ),
        task_started("toolu_N", "task_n", background=False, depth=2, parent_task_id="task_a"),
        notification("toolu_N", "completed", 5000, summary="\n  Nested done.  \nsecond line"),
        task_updated("task_n", "completed", ms(10, 0, 20)),
        launch_result("10:00:30", "toolu_F", FRAMED_HAND_BACK, tool_use_result={"status": "completed"}),
        task_updated("task_f", "completed", ms(10, 0, 30)),
        assistant(
            "10:01:00",
            bash("tu_bg", "npm test", background=True),
            agent_launch("toolu_K", "general-purpose", "watch it", description="Watch", background=True),
            agent_launch("toolu_X", "general-purpose", "break it", description="Break"),
        ),
        task_started("tu_bg", "task_bg", background=True, depth=1, task_type="local_bash"),
        task_started("toolu_K", "task_k", background=True, depth=1),
        task_started("toolu_X", "task_x", background=False, depth=1),
        notification("toolu_X", "failed", summary="Error: boom"),
        task_updated("task_x", "failed", ms(10, 1, 5), error="boom"),
        notification("toolu_K", "killed"),
        task_updated("task_k", "killed", ms(10, 2, 0)),
        result(6, 130000, 60000, 0.2, {SONNET: (10, 20, 30, 40)}),
    ]


def write_rollout(path: Path, lines: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(line) + "\n" for line in lines))


def token_count(clock, input_tokens, cached, output):
    return {
        "timestamp": ts(clock),
        "type": "event_msg",
        "payload": {
            "type": "token_count",
            "info": {"total_token_usage": {"input_tokens": input_tokens, "cached_input_tokens": cached, "output_tokens": output}},
        },
    }


ALL_TOTALS = (
    "wall_time_s",
    "model_time_s",
    "turns",
    "cost_usd",
    "input_tokens",
    "output_tokens",
    "cache_read_tokens",
    "cache_write_tokens",
)


# parse_timestamp and goto_step_of


def test_parse_timestamp_reads_iso_with_z_and_rejects_the_rest():
    assert parse_timestamp("2026-10-23T10:00:05Z") == at(10, 0, 5)
    assert parse_timestamp("2026-10-23T10:00:05.250Z") == datetime(2026, 10, 23, 10, 0, 5, 250000, tzinfo=timezone.utc)
    assert parse_timestamp(None) is None
    assert parse_timestamp("") is None
    assert parse_timestamp("yesterday") is None


def test_goto_step_of_reads_the_step_from_goto_data():
    assert goto_step_of(GOTO_DISCOVERY) == "discovery"
    assert goto_step_of("""spektacular implement goto --data='{"step":"verify","name":"x"}'""") == "verify"
    assert goto_step_of(GOTO_HELP) is None
    assert goto_step_of("spektacular plan status") is None
    assert goto_step_of("ls -la") is None


# Claude: one result event


def test_claude_one_result_totals_equal_that_result():
    totals = claude_summary(claude_one_result())
    assert totals == Totals(
        wall_time_s=61.0,
        model_time_s=42.5,
        turns=7,
        cost_usd=0.25,
        input_tokens=120,
        output_tokens=2500,
        cache_read_tokens=36000,
        cache_write_tokens=5000,
        model=f"{SONNET}+{HAIKU}",
        unknowns=[],
    )


def test_claude_one_result_missing_cost_is_unknown_not_zero():
    objs = claude_one_result()
    del objs[-1]["total_cost_usd"]
    totals = claude_summary(objs)
    assert totals.cost_usd is None
    assert totals.unknowns == [Unknown("totals.cost_usd", "missing from the last result event in claude-code.txt")]


def test_claude_one_result_subagents_from_stats_and_notification():
    assert claude_subagents(claude_one_result()) == Subagents(
        started=1, completed=1, failed=0, stopped=0, durations_s=[14.0], unknowns=[]
    )


def test_claude_messages_one_per_assistant_object():
    assert claude_messages(claude_one_result()) == [
        Message(at=at(10, 0, 0), chars=7 + 9 + LEN_GOTO_DISCOVERY, parent=None, order=0, goto_step="discovery"),
        Message(at=at(10, 0, 5), chars=LEN_FIND_SPECS, parent=None, order=1, goto_step=None),
        Message(at=at(10, 0, 6), chars=7 + LEN_GOTO_HELP, parent="toolu_A", order=2, goto_step=None),
    ]


# Claude: split by background sub-agents


def test_claude_split_sums_turns_and_takes_the_rest_from_the_last_result():
    totals = claude_summary(claude_split())
    assert totals == Totals(
        wall_time_s=210.0,  # 10:00:00 to 10:03:30
        model_time_s=90.0,
        turns=4 + 3,
        cost_usd=0.3,
        input_tokens=300,
        output_tokens=4000,
        cache_read_tokens=50000,
        cache_write_tokens=7000,
        model=SONNET,
        unknowns=[],
    )


def test_claude_split_subagents_from_last_stats_with_unknown_durations():
    assert claude_subagents(claude_split()) == Subagents(
        started=4,
        completed=2,
        failed=1,
        stopped=1,
        durations_s=[14.0, 2.5],
        unknowns=[Unknown("subagents.durations_s", "2 sub-agents never reported a duration")],
    )


def test_claude_subagents_without_stats_counts_launches_and_notifications():
    assert claude_subagents(claude_split(with_stats=False)) == Subagents(
        started=3,  # top-level launches A, B, C; the nested N is not the main agent's
        completed=1,
        failed=1,
        stopped=1,  # B's "killed"
        durations_s=[14.0, 2.5],
        unknowns=[Unknown("subagents.durations_s", "1 sub-agents never reported a duration")],
    )


def test_claude_launch_root_resolves_nested_launch_to_top_level():
    assert claude_launch_root(claude_split()) == {"toolu_A": "toolu_A", "toolu_N": "toolu_A"}


# Claude: no result event


def test_claude_no_result_leaves_every_total_unknown():
    totals = claude_summary([init(), assistant("10:00:00", {"type": "text", "text": "hi"})])
    assert totals == Totals(
        model=SONNET,
        unknowns=[Unknown(f"totals.{name}", "no result event in claude-code.txt") for name in ALL_TOTALS],
    )


def test_claude_no_result_and_no_init_also_leaves_model_unknown():
    totals = claude_summary([assistant("10:00:00", {"type": "text", "text": "hi"})])
    assert totals.model is None
    assert totals.unknowns[-1] == Unknown("run.model", "no result or init event in claude-code.txt")
    assert len(totals.unknowns) == len(ALL_TOTALS) + 1


# The readers existing verifiers use, pinned on the same input


def test_existing_claude_readers_unchanged_on_one_result():
    objs = claude_one_result()
    assert claude_main_models(objs) == {SONNET}
    assert claude_subagent_launches(objs) == [
        SubagentLaunch(tool_use_id="toolu_A", subagent_type="general-purpose", prompt="find specs", models={HAIKU}, returned=True)
    ]


def test_existing_claude_readers_unchanged_on_split():
    objs = claude_split()
    assert claude_main_models(objs) == {SONNET}
    assert claude_subagent_launches(objs) == [
        SubagentLaunch(tool_use_id="toolu_A", subagent_type="general-purpose", prompt="find specs", models={HAIKU}, returned=True),
        SubagentLaunch(tool_use_id="toolu_B", subagent_type="Explore", prompt="dig deeper", models=set(), returned=False),
        SubagentLaunch(tool_use_id="toolu_C", subagent_type="general-purpose", prompt="write report", models=set(), returned=True),
    ]


def test_existing_acp_reader_unchanged_on_bob():
    assert acp_subagent_launches(bob_trajectory()["steps"]) == [
        SubagentLaunch(
            tool_use_id="t2",
            subagent_type="",
            prompt='map the repo\n{"name": "explore", "description": "map the repo"}',
            returned=True,
        ),
        SubagentLaunch(tool_use_id="t3", subagent_type="", prompt="write tests\n{}", returned=True),
    ]


# ATIF: Bob


def test_bob_summary_has_turns_and_everything_else_unknown():
    assert atif_summary(bob_trajectory()) == Totals(
        turns=3,
        unknowns=[
            Unknown("totals.wall_time_s", "bob trajectory has no step timestamps"),
            Unknown("totals.model_time_s", "bob reports no model time"),
            Unknown("totals.cost_usd", "bob reports no cost"),
            Unknown("totals.input_tokens", "bob reports no token counts"),
            Unknown("totals.output_tokens", "bob reports no token counts"),
            Unknown("totals.cache_read_tokens", "bob reports no token counts"),
            Unknown("totals.cache_write_tokens", "bob reports no cache writes"),
            Unknown("run.model", "bob reports no model"),
        ],
    )


def test_bob_messages_are_agent_steps_without_timestamps():
    # order is the step's index in the trajectory, user steps included.
    assert atif_messages(bob_trajectory()) == [
        Message(at=None, chars=13 + 5 + LEN_GOTO_VERIFICATION, parent=None, order=1, goto_step="verification"),
        Message(at=None, chars=LEN_EXPLORE, parent=None, order=2, goto_step=None),
        Message(at=None, chars=5 + LEN_EMPTY, parent=None, order=4, goto_step=None),
    ]


def test_bob_subagents_counts_launches_and_leaves_outcomes_unknown():
    reason = "bob reports no sub-agent outcomes"
    assert atif_subagents(bob_trajectory()) == Subagents(
        started=2,
        unknowns=[
            Unknown("subagents.completed", reason),
            Unknown("subagents.failed", reason),
            Unknown("subagents.stopped", reason),
            Unknown("subagents.durations_s", reason),
        ],
    )


# ATIF: Codex


def test_codex_summary_has_turns_tokens_and_wall_time():
    assert atif_summary(codex_trajectory()) == Totals(
        wall_time_s=90.0,  # 10:00:00 to 10:01:30
        turns=2,
        input_tokens=2000,  # 5000 prompt tokens, 3000 of them cached
        output_tokens=700,
        cache_read_tokens=3000,
        model="gpt-5",
        unknowns=[
            Unknown("totals.model_time_s", "codex reports no model time"),
            Unknown("totals.cost_usd", "codex reports no cost"),
            Unknown("totals.cache_write_tokens", "codex reports no cache writes"),
        ],
    )


def test_codex_messages_have_timestamps_and_list_text():
    assert atif_messages(codex_trajectory()) == [
        Message(at=at(10, 0, 4), chars=8 + LEN_LS, parent=None, order=1, goto_step=None),
        Message(at=at(10, 1, 30), chars=9 + 2, parent=None, order=2, goto_step=None),
    ]


def test_codex_subagents_all_unknown():
    reason = "codex reports no sub-agents"
    assert atif_subagents(codex_trajectory()) == Subagents(
        unknowns=[
            Unknown("subagents.started", reason),
            Unknown("subagents.completed", reason),
            Unknown("subagents.failed", reason),
            Unknown("subagents.stopped", reason),
            Unknown("subagents.durations_s", reason),
        ],
    )


# Codex native rollout


def test_codex_rollout_summary_reads_the_newest_rollout(tmp_path):
    sessions = tmp_path / "sessions"
    # An older rollout with different numbers, which must be ignored.
    write_rollout(
        sessions / "2026" / "10" / "22" / "rollout-2026-10-22T09-00-00-old.jsonl",
        [{"timestamp": "2026-10-22T09:00:00Z", "type": "turn_context", "payload": {"model": "gpt-4"}}, token_count("09:00:01", 9, 9, 9)],
    )
    write_rollout(
        sessions / "2026" / "10" / "23" / "rollout-2026-10-23T10-00-00-abc.jsonl",
        [
            {"timestamp": ts("10:00:00"), "type": "session_meta", "payload": {}},
            {"timestamp": ts("10:00:01"), "type": "turn_context", "payload": {"model": "gpt-5"}},
            # A rate-limit update carries no usage and is not a model call.
            {"timestamp": ts("10:00:05"), "type": "event_msg", "payload": {"type": "token_count", "info": None}},
            token_count("10:00:10", 400, 100, 50),
            token_count("10:02:00", 1000, 600, 200),
        ],
    )
    assert codex_rollout_summary(sessions) == Totals(
        wall_time_s=120.0,  # 10:00:00 to 10:02:00
        turns=2,
        input_tokens=400,  # 1000 input, 600 of them cached
        output_tokens=200,
        cache_read_tokens=600,
        model="gpt-5",
        unknowns=[
            Unknown("totals.cost_usd", "codex reports no cost"),
            Unknown("totals.model_time_s", "codex reports no model time"),
            Unknown("totals.cache_write_tokens", "codex reports no cache writes"),
        ],
    )


def test_codex_rollout_summary_empty_sessions_dir_is_all_unknown(tmp_path):
    sessions = tmp_path / "sessions"
    sessions.mkdir()
    reason = "no codex rollout under sessions/"
    expected = Totals(
        unknowns=[Unknown(f"totals.{name}", reason) for name in ALL_TOTALS] + [Unknown("run.model", reason)]
    )
    assert codex_rollout_summary(sessions) == expected
    assert codex_rollout_summary(tmp_path / "missing") == expected


# Claude: child agents

NO_END = "claude logged no end time for the child"
NO_DURATION = "claude reported no duration for the child"
NO_DURATION_FAILED = "claude reported no duration for a failed child"
NO_MESSAGES = "claude streamed none of the child's messages"
EMPTY_HAND_BACK = "the child's hand-back was empty"
NO_DESCRIPTION = "the launch named no description"


def child_unknown(tid, name, reason):
    return Unknown(f"subagents.children[{tid}].{name}", reason)


CHILD_A = Child(
    id="toolu_A",
    parent=None,
    kind="general-purpose",
    description="Find specs",
    background=True,
    launched_at=at(10, 0, 0),
    ended_at=None,  # its launch returned at 10:00:01, but it never ended
    duration_s=None,
    ending="no_hand_back",
    hand_back=None,
    models=[HAIKU],
    output=6 + LEN_NESTED_LOOK,
    prompt=PROMPT_A,
)
CHILD_F = Child(
    id="toolu_F",
    parent=None,
    kind="Explore",
    description="Look around",
    background=False,
    launched_at=at(10, 0, 0),
    ended_at=at(10, 0, 30),
    duration_s=None,
    ending="completed",
    hand_back="line1",
    models=None,
    output=None,
    prompt="look around",
)
CHILD_N = Child(
    id="toolu_N",
    parent="toolu_A",
    kind="Explore",
    description=None,
    background=False,
    launched_at=at(10, 0, 10),
    ended_at=at(10, 0, 20),
    duration_s=5.0,  # its own report, not the 10s between launch and end
    ending="completed",
    hand_back="Nested done.",
    models=None,
    output=None,
    prompt="nested look",
)
CHILD_K = Child(
    id="toolu_K",
    parent=None,
    kind="general-purpose",
    description="Watch",
    background=True,
    launched_at=at(10, 1, 0),
    ended_at=at(10, 2, 0),
    duration_s=None,
    ending="stopped",
    hand_back=None,
    models=None,
    output=None,
    prompt="watch it",
)
CHILD_X = Child(
    id="toolu_X",
    parent=None,
    kind="general-purpose",
    description="Break",
    background=False,
    launched_at=at(10, 1, 0),
    ended_at=at(10, 1, 5),
    duration_s=None,
    ending="failed",
    hand_back="Error: boom",
    models=None,
    output=None,
    prompt="break it",
)


def children_by_id(objs):
    children, _ = claude_children(objs)
    return {c.id: c for c in children}


def test_claude_children_one_per_started_child_in_launch_order():
    children, unknowns = claude_children(claude_children_log())
    assert children == [CHILD_A, CHILD_F, CHILD_N, CHILD_K, CHILD_X]
    assert unknowns == [
        child_unknown("toolu_A", "ended_at", NO_END),
        child_unknown("toolu_A", "duration_s", NO_DURATION),
        child_unknown("toolu_F", "duration_s", NO_DURATION),
        child_unknown("toolu_F", "models", NO_MESSAGES),
        child_unknown("toolu_F", "output", NO_MESSAGES),
        child_unknown("toolu_N", "models", NO_MESSAGES),
        child_unknown("toolu_N", "output", NO_MESSAGES),
        child_unknown("toolu_N", "description", NO_DESCRIPTION),
        child_unknown("toolu_K", "duration_s", NO_DURATION),
        child_unknown("toolu_K", "models", NO_MESSAGES),
        child_unknown("toolu_K", "output", NO_MESSAGES),
        child_unknown("toolu_X", "duration_s", NO_DURATION_FAILED),
        child_unknown("toolu_X", "models", NO_MESSAGES),
        child_unknown("toolu_X", "output", NO_MESSAGES),
    ]


def test_claude_children_leave_out_refused_launches_and_shell_tasks():
    ids = set(children_by_id(claude_children_log()))
    assert "toolu_R" not in ids  # refused: an error result and no task_started
    assert "tu_bg" not in ids  # a background shell task, task_type local_bash
    assert ids == {"toolu_A", "toolu_F", "toolu_N", "toolu_K", "toolu_X"}


def test_claude_nested_child_names_the_child_that_started_it():
    children = children_by_id(claude_children_log())
    assert children["toolu_N"].parent == "toolu_A"
    assert [c.parent for c in children.values() if c.id != "toolu_N"] == [None, None, None, None]


def test_claude_child_context_is_exactly_its_launch_prompt():
    child = children_by_id(claude_children_log())["toolu_A"]
    assert child.prompt == PROMPT_A
    assert len(child.prompt) == LEN_PROMPT_A


def test_claude_child_duration_is_its_own_reported_run_time():
    assert children_by_id(claude_children_log())["toolu_N"].duration_s == 5.0


def test_claude_background_child_not_ended_when_its_launch_returns():
    child = children_by_id(claude_children_log())["toolu_A"]
    assert child.background is True
    assert child.ending == "no_hand_back"
    assert child.ended_at is None


def test_claude_failed_child_without_duration_is_unknown_not_zero():
    children, unknowns = claude_children(claude_children_log())
    child = {c.id: c for c in children}["toolu_X"]
    assert child.ending == "failed"
    assert child.duration_s is None
    assert child_unknown("toolu_X", "duration_s", NO_DURATION_FAILED) in unknowns


def test_claude_killed_child_is_stopped():
    assert children_by_id(claude_children_log())["toolu_K"].ending == "stopped"


def test_claude_foreground_hand_back_is_unframed_first_line():
    assert children_by_id(claude_children_log())["toolu_F"].hand_back == "line1"


def test_claude_foreground_child_without_notification_reads_its_launch_result():
    objs = [
        assistant(
            "10:00:00",
            agent_launch("toolu_G", "general-purpose", "try it", description="Try"),
            agent_launch("toolu_H", "general-purpose", "do it", description="Do"),
        ),
        task_started("toolu_G", "task_g", background=False, depth=1),
        task_started("toolu_H", "task_h", background=False, depth=1),
        launch_result("10:00:20", "toolu_G", "\nboom\nmore", is_error=True),
        launch_result("10:00:30", "toolu_H", "All done.\nDetails", tool_use_result={"status": "completed"}),
    ]
    children = children_by_id(objs)
    assert (children["toolu_G"].ending, children["toolu_G"].hand_back) == ("failed", "boom")
    assert (children["toolu_H"].ending, children["toolu_H"].hand_back) == ("completed", "All done.")


def test_claude_child_parent_from_parent_task_id_and_only_a_known_launch():
    objs = [
        assistant("10:00:00", agent_launch("toolu_P", "general-purpose", "parent")),
        task_started("toolu_P", "task_p", background=False, depth=1),
        # Launched in an object that names no parent: task_started's
        # parent_task_id still names P.
        assistant("10:00:10", agent_launch("toolu_Q", "Explore", "child of P")),
        task_started("toolu_Q", "task_q", background=False, depth=2, parent_task_id="task_p"),
        # Launched in a sub-agent message whose parent is no launch at all.
        assistant("10:00:20", agent_launch("toolu_S", "Explore", "orphan"), parent="toolu_gone"),
        task_started("toolu_S", "task_s", background=False, depth=2),
    ]
    children = children_by_id(objs)
    assert children["toolu_P"].parent is None
    assert children["toolu_Q"].parent == "toolu_P"
    assert children["toolu_S"].parent is None


def test_claude_children_without_task_started_count_every_unrefused_launch():
    # claude_split has no task_started, so every launch counts, nested N
    # included, and background state is unknown agent-wide.
    children, unknowns = claude_children(claude_split())
    base = dict(description=None, background=None, ended_at=None)
    assert children == [
        Child(id="toolu_A", parent=None, kind="general-purpose", launched_at=at(10, 0, 0), duration_s=14.0,
              ending="completed", hand_back=None, models=[HAIKU], output=LEN_NESTED_LOOK, prompt="find specs", **base),
        Child(id="toolu_B", parent=None, kind="Explore", launched_at=at(10, 0, 0), duration_s=None,
              ending="stopped", hand_back=None, models=None, output=None, prompt="dig deeper", **base),
        Child(id="toolu_N", parent="toolu_A", kind="Explore", launched_at=at(10, 0, 30), duration_s=None,
              ending="no_hand_back", hand_back=None, models=[HAIKU], output=6, prompt="nested look", **base),
        Child(id="toolu_C", parent=None, kind="general-purpose", launched_at=at(10, 2, 0), duration_s=2.5,
              ending="failed", hand_back=None, models=None, output=None, prompt="write report", **base),
    ]
    assert unknowns == [
        Unknown("subagents.children.background", "claude logged no task_started events"),
        child_unknown("toolu_A", "ended_at", NO_END),
        child_unknown("toolu_A", "hand_back", EMPTY_HAND_BACK),
        child_unknown("toolu_A", "description", NO_DESCRIPTION),
        child_unknown("toolu_B", "ended_at", NO_END),
        child_unknown("toolu_B", "duration_s", NO_DURATION),
        child_unknown("toolu_B", "models", NO_MESSAGES),
        child_unknown("toolu_B", "output", NO_MESSAGES),
        child_unknown("toolu_B", "description", NO_DESCRIPTION),
        child_unknown("toolu_N", "ended_at", NO_END),
        child_unknown("toolu_N", "duration_s", NO_DURATION),
        child_unknown("toolu_N", "description", NO_DESCRIPTION),
        child_unknown("toolu_C", "ended_at", NO_END),
        child_unknown("toolu_C", "hand_back", EMPTY_HAND_BACK),
        child_unknown("toolu_C", "models", NO_MESSAGES),
        child_unknown("toolu_C", "output", NO_MESSAGES),
        child_unknown("toolu_C", "description", NO_DESCRIPTION),
    ]


def test_claude_children_without_task_started_still_leave_out_refused_launches():
    objs = [
        assistant("10:00:00", launch("toolu_A", "general-purpose", "go"), launch("toolu_R", "nope", "refused")),
        launch_result("10:00:01", "toolu_R", "Agent type 'nope' not found", is_error=True),
    ]
    assert list(children_by_id(objs)) == ["toolu_A"]


# ATIF: child agents


def test_bob_children_carry_kind_description_prompt_and_nothing_else():
    children, unknowns = atif_children(bob_trajectory())
    empty = dict(parent=None, background=None, launched_at=None, ended_at=None, duration_s=None,
                 ending=None, hand_back=None, models=None, output=None)
    assert children == [
        # order is the launching step's index among the dict steps: step_id 3 is index 2, step_id 5 index 4.
        Child(id="t2", kind="explore", description="map the repo", prompt="map the repo", order=2, **empty),
        Child(id="t3", kind=None, description="write tests", prompt=None, order=4, **empty),
    ]
    assert unknowns == [
        Unknown("subagents.children.parent", "bob logs no sub-agent parent"),
        Unknown("subagents.children.background", "bob logs no sub-agent background state"),
        Unknown("subagents.children.launched_at", "bob logs no sub-agent start time"),
        Unknown("subagents.children.ended_at", "bob logs no sub-agent end time"),
        Unknown("subagents.children.duration_s", "bob logs no sub-agent duration"),
        Unknown("subagents.children.ending", "bob logs no sub-agent outcome"),
        Unknown("subagents.children.hand_back", "bob logs no sub-agent hand-back"),
        Unknown("subagents.children.models", "bob logs no sub-agent models"),
        Unknown("subagents.children.output", "bob logs no sub-agent output"),
        Unknown("subagents.children[t3].kind", "the bob sub-agent call named no kind"),
        Unknown("subagents.children[t3].prompt", "the bob sub-agent call named no prompt"),
    ]


def test_bob_without_subagent_calls_has_no_children_and_no_unknowns():
    traj = bob_trajectory()
    traj["steps"] = traj["steps"][:2]
    assert atif_children(traj) == ([], [])


def test_codex_children_list_is_unknown():
    assert atif_children(codex_trajectory()) == (None, [Unknown("subagents.children", "codex reports no sub-agents")])


# Claude: turns and wake-ups


def user_prompt(clock, text, origin=None):
    out = {"type": "user", "timestamp": ts(clock), "parent_tool_use_id": None, "message": {"role": "user", "content": text}}
    if origin is not None:
        out["origin"] = {"kind": origin}
    return out


def turn_result(origin=None):
    return result(1, 1000, 500, 0.01, {SONNET: (1, 1, 1, 1)}, origin=origin)


def test_claude_turns_read_their_end_and_what_woke_them():
    objs = [
        init(),
        user_prompt("10:00:00", "go"),
        assistant("10:00:05", agent_launch("toolu_A", "general-purpose", "work", background=True)),
        turn_result(),
        notification("toolu_A", "completed", 60000, summary="done"),
        user_prompt("10:01:00", "<task-notification>done</task-notification>", origin="task-notification"),
        assistant("10:01:05", {"type": "text", "text": "noted"}),
        turn_result(),
        {"type": "command_lifecycle", "wakeup_source": "scheduled_wakeup"},
        user_prompt("10:05:00", "wake up"),
        assistant("10:05:10", {"type": "text", "text": "checking"}),
        turn_result(),
        user_prompt("10:06:00", "more"),
        assistant("10:06:30", {"type": "text", "text": "sure"}),
        turn_result(),
        assistant("10:07:00", {"type": "text", "text": "again"}),
        turn_result(origin="task-notification"),  # the notice named only on the result
    ]
    assert claude_turns(objs) == [
        Turn(ended_at=at(10, 0, 5), woken_by="start"),
        Turn(ended_at=at(10, 1, 5), woken_by="task-notification"),
        Turn(ended_at=at(10, 5, 10), woken_by="wakeup"),
        Turn(ended_at=at(10, 6, 30), woken_by=None),
        Turn(ended_at=at(10, 7, 0), woken_by="task-notification"),
    ]


def late_results_log(wakeup=False):
    """Two turns split by a mid-stream init, with both results written at the
    end. A child's message streams in turn 1 after the main agent's last."""
    return [
        init(),
        user_prompt("10:00:00", "go"),
        assistant("10:00:05", agent_launch("toolu_A", "general-purpose", "work", background=True)),
        assistant("10:00:30", {"type": "text", "text": "working"}, parent="toolu_A"),
        *([{"type": "command_lifecycle", "wakeup_source": "scheduled_wakeup"}] if wakeup else []),
        init(),
        assistant("10:01:05", {"type": "text", "text": "noted"}),
        turn_result(),
        turn_result(origin=None if wakeup else "task-notification"),
    ]


def test_claude_turns_pair_late_results_with_the_stretches_between_inits():
    assert claude_turns(late_results_log()) == [
        Turn(ended_at=at(10, 0, 5), woken_by="start"),
        Turn(ended_at=at(10, 1, 5), woken_by="task-notification"),
    ]


def test_claude_turns_late_results_a_wakeup_before_the_init_wakes_the_next_turn():
    assert claude_turns(late_results_log(wakeup=True)) == [
        Turn(ended_at=at(10, 0, 5), woken_by="start"),
        Turn(ended_at=at(10, 1, 5), woken_by="wakeup"),
    ]


def test_claude_wakeups_read_scheduled_wakeups_and_sleeps_from_the_main_agent():
    objs = [
        assistant("10:00:00", schedule_wakeup("w1", 270)),
        assistant("10:00:10", bash("b1", "sleep 240; spektacular status")),
        assistant("10:00:20", bash("b2", "cd /work && sleep 60 && spektacular implement status", background=True)),
        assistant("10:00:30", bash("b3", "ls asleep.txt"), bash("b4", "echo sleepy")),
        # A child's own sleeps and wake-ups are not the main agent's.
        assistant("10:00:40", bash("b5", "sleep 30"), schedule_wakeup("w2", 60), parent="toolu_A", model=HAIKU),
        assistant("10:00:50", tool_use("c1", "CronCreate", {"cron": "*/5 * * * *", "prompt": "check"})),
        assistant("10:01:00", bash("b6", LONG_SLEEP)),
    ]
    assert claude_wakeups(objs) == [
        Wakeup(at=at(10, 0, 0), tool="ScheduleWakeup", detail="delaySeconds=270"),
        Wakeup(at=at(10, 0, 10), tool="Bash", detail="sleep 240; spektacular status"),
        Wakeup(at=at(10, 0, 20), tool="Bash", detail="cd /work && sleep 60 && spektacular implement status"),
        Wakeup(at=at(10, 0, 50), tool="CronCreate", detail="*/5 * * * *"),
        Wakeup(at=at(10, 1, 0), tool="Bash", detail=LONG_SLEEP_80),
    ]
