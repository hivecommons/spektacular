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
    Message,
    SubagentLaunch,
    Subagents,
    Totals,
    Unknown,
    acp_subagent_launches,
    atif_messages,
    atif_subagents,
    atif_summary,
    claude_launch_root,
    claude_main_models,
    claude_messages,
    claude_subagent_launches,
    claude_subagents,
    claude_summary,
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


def notification(tool_use_id, status, duration_ms=None):
    note = {"type": "system", "subtype": "task_notification", "tool_use_id": tool_use_id, "status": status}
    if duration_ms is not None:
        note["usage"] = {"total_tokens": 1000, "tool_uses": 3, "duration_ms": duration_ms}
    return note


def stats(spawned, completed, failed, killed_parent=0, killed_user=0, killed_system=0):
    return {
        "spawned": spawned,
        "completed": completed,
        "failed": failed,
        "killed": {"parent": killed_parent, "user": killed_user, "system": killed_system},
    }


def result(num_turns, duration_ms, duration_api_ms, cost, usage, subagent_stats=None):
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
