"""Self-check for the trimmed reference runs under data/ and trim_reference.py.

Four real harbor trials from 2026-10-23 are checked in, trimmed so that no
conversation is kept: m70-epic (Sonnet 4.6, epic workflow) and s55-epic,
s55-plan, s55-implement (Sonnet 5.5). The cases here pin what the recorder
reads from them, the child agents of the 5.5 epic run and every run's
tokens, models and cost, with values worked out by reading the fixtures by
hand. Their session logs predated Spektacular's run logs and are not kept,
so every record's steps, helpers and waits are unknown, never filled in from
the transcript.
They also prove that each run is compact and blanked, that trim_reference.py
regenerates every run byte for byte, that a trim never changes a value the
recorder computes, and that the script refuses to write a trim that would.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_reference_runs.py
"""

import json
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import metrics  # noqa: E402
import runlog  # noqa: E402
import transcripts  # noqa: E402
import trim_reference  # noqa: E402

DATA = HERE / "data"
RUNS = ["m70-epic", "s55-epic", "s55-plan", "s55-implement"]
NONE = Path("/nonexistent")

# Each run keeps character counts, so its size follows the run's length; the
# largest (m70-epic) is about 1.16 MB and all four about 2.27 MB.
MAX_RUN_BYTES = 1_500_000
MAX_ALL_BYTES = 3_000_000

BUILD = {
    "m70-epic": {"suite": "epic-workflow", "agent": "claude", "model": "claude-sonnet-4-6"},
    "s55-epic": {"suite": "epic-workflow", "agent": "claude", "model": "claude-sonnet-5-5"},
    "s55-plan": {"suite": "plan-workflow", "agent": "claude", "model": "claude-sonnet-5-5"},
    "s55-implement": {"suite": "implement-workflow", "agent": "claude", "model": "claude-sonnet-5-5"},
}

# The three children of the 5.5 epic run, in launch order, read by hand from
# its Agent calls, task_started and task_notification events.
S55_CHILDREN = [
    {
        "id": "toolu_01LsCrL1DDkTZGc6kMPVwn5D",
        "spec": "20260101000001-storage",
        "prompt_chars": 1723,
        "duration_s": 109.605,
        "background": True,
        "hand_back": "DONE: 20260101000001-storage",
    },
    {
        "id": "toolu_01Fcx3c9hqXdQYxakexacWST",
        "spec": "20260101000003-help",
        "prompt_chars": 1705,
        "duration_s": 111.142,
        "background": True,
        "hand_back": "DONE: 20260101000003-help",
    },
    {
        "id": "toolu_011gDQrdkhaFHQxGGBxQvxzo",
        "spec": "20260101000002-commands",
        "prompt_chars": 1759,
        "duration_s": 145.246,
        "background": False,
        "hand_back": "DONE: 20260101000002-commands",
    },
]
# A spektacular binary that is never there, so a record's log sections are
# unknown for the same reason whatever is on PATH.
NO_BINARY = "/nonexistent/spektacular"
# Every log-sourced field of the record, by hand from metrics.py.
LOG_FIELDS = [
    "steps", "subagents.started", "subagents.helpers",
    "totals.user_wait_s", "totals.helper_wait_s", "totals.gap_s", "totals.trace_calls",
]
LOG_TOTALS = ["trace_calls", "trace_calls_per_task", "user_wait_s", "helper_wait_s", "gap_s", "gaps", "gap_threshold_s"]
HELPER_COUNTS = ["started", "completed", "failed", "unfinished", "durations_s", "helpers"]


# Token, model and cost figures of each reference run, as the harness
# computed them from the transcript alone before spec 000076 changed any
# transcript reader. Pinned before 000076; must not change. The models used
# are the run's: every model its last result event reports usage for, which
# the recorder joins into `model`.
PINNED_MODELS_USED = {
    "m70-epic": ["claude-sonnet-4-6"],
    "s55-epic": ["claude-sonnet-5-5"],
    "s55-plan": ["claude-sonnet-5-5"],
    "s55-implement": ["claude-sonnet-5-5"],
    "probe-background-child": ["claude-sonnet-5-5"],
}
PINNED_TOTALS = {
    "m70-epic": {
        "model": "claude-sonnet-4-6", "input_tokens": 220, "output_tokens": 76691,
        "cache_read_tokens": 8874067, "cache_write_tokens": 257220, "cost_usd": 4.855505849999998,
        "turns": 31, "model_time_s": 1384.389, "wall_time_s": 993.223,
    },
    "s55-epic": {
        "model": "claude-sonnet-5-5", "input_tokens": 188, "output_tokens": 43630,
        "cache_read_tokens": 3671430, "cache_write_tokens": 222997, "cost_usd": 1.4066310000000004,
        "turns": 17, "model_time_s": 425.898, "wall_time_s": 1249.433,
    },
    "s55-plan": {
        "model": "claude-sonnet-5-5", "input_tokens": 74, "output_tokens": 19805,
        "cache_read_tokens": 1694392, "cache_write_tokens": 81163, "cost_usd": 0.6781967,
        "turns": 34, "model_time_s": 191.608, "wall_time_s": 223.618,
    },
    "s55-implement": {
        "model": "claude-sonnet-5-5", "input_tokens": 26, "output_tokens": 5045,
        "cache_read_tokens": 378361, "cache_write_tokens": 27792, "cost_usd": 0.1995061,
        "turns": 16, "model_time_s": 61.563, "wall_time_s": 61.84,
    },
    "probe-background-child": {
        "model": "claude-sonnet-5-5", "input_tokens": 8, "output_tokens": 466,
        "cache_read_tokens": 45446, "cache_write_tokens": 8345, "cost_usd": 0.0368961,
        "turns": 2, "model_time_s": 8.269, "wall_time_s": 4.747,
    },
}
# Every checked-in reference run with a transcript: the four trials and the
# background-child probe.
PINNED_RUNS = [*RUNS, "probe-background-child"]


@pytest.fixture(autouse=True)
def no_binary(monkeypatch):
    """Points the run log reader at a binary that is never there."""
    monkeypatch.setenv("SPEKTACULAR_BIN", NO_BINARY)


def record_of(run: Path, log_dir: Path | None = None) -> dict:
    """The record of a reference run, its logs looked for in logs/, where a
    trim copies them; these runs predate Spektacular's run logs and have none."""
    return metrics.build_record(
        metrics.Paths(
            build_info=run / "build.json", agent_dir=run, log_dir=log_dir or run / "logs",
            bob_logs=NONE, junit=NONE, reward=NONE,
        )
    )


def log_unknowns(record: dict) -> dict[str, str]:
    """The record's unknowns for its log-sourced fields, by field."""
    return {u["field"]: u["reason"] for u in record["unknown"] if u["field"] in LOG_FIELDS}


def assert_log_sections_unknown(record: dict, reason: str) -> None:
    """Every step, helper and wait measure is null and unknown with reason,
    and none was filled in from the transcript."""
    assert record["steps"] == {"workflows": None}
    assert {k: record["subagents"][k] for k in HELPER_COUNTS} == dict.fromkeys(HELPER_COUNTS)
    assert {k: record["totals"][k] for k in LOG_TOTALS} == dict.fromkeys(LOG_TOTALS)
    assert log_unknowns(record) == dict.fromkeys(LOG_FIELDS, reason)


def transcript(run: Path) -> list[dict]:
    return [json.loads(line) for line in (run / "claude-code.txt").read_text().splitlines() if line.strip()]


def launches(objs: list[dict]) -> dict[str, dict]:
    """Every Agent call in a transcript, by tool_use id."""
    return {
        b["id"]: b
        for o in objs
        if o.get("type") == "assistant"
        for b in o["message"]["content"]
        if b.get("type") == "tool_use" and b.get("name") == "Agent"
    }


def tree(root: Path) -> dict[str, bytes]:
    return {str(p.relative_to(root)): p.read_bytes() for p in sorted(root.rglob("*")) if p.is_file()}


class NoStats:
    """A run log reader that can never answer, for the same fixed reason
    wherever the logs are, so a trim's before and after records compare
    equal whatever `spektacular` is or is not on PATH."""

    REASON = "stats unavailable in the trim self-checks"

    def __init__(self, log_dir, *args, **kwargs):
        self.log_dir = log_dir

    def __getattr__(self, name):
        def unavailable(*args, **kwargs):
            raise runlog.Unavailable(self.REASON)

        return unavailable


@pytest.fixture
def scratch(tmp_path, monkeypatch) -> Path:
    """Keeps the script's staging directory under tmp_path, and the run log
    reader's answer fixed."""
    staging = tmp_path / "staging"
    staging.mkdir()
    monkeypatch.setattr(tempfile, "tempdir", str(staging))
    monkeypatch.setattr(metrics.runlog, "RunLog", NoStats)
    return staging


# The 5.5 epic-planning run's children.


def test_s55_epic_child_count_matches_the_transcripts_own():
    run = DATA / "s55-epic"
    objs = transcript(run)
    spawned = [o for o in objs if o.get("type") == "result"][-1]["subagent_stats"]["spawned"]
    children, _ = transcripts.claude_children(objs)
    record = record_of(run)
    assert spawned == 3
    assert len(children) == spawned
    assert len(record["subagents"]["briefs"]) == spawned
    # the helper counts come only from Spektacular's logs, which this run lacks
    assert record["subagents"]["started"] is None
    assert "subagents.started" in log_unknowns(record)


def test_s55_epic_contexts_are_the_launch_prompts():
    run = DATA / "s55-epic"
    calls = launches(transcript(run))
    briefs = record_of(run)["subagents"]["briefs"]
    assert [b["id"] for b in briefs] == [c["id"] for c in S55_CHILDREN]
    for brief, want in zip(briefs, S55_CHILDREN):
        prompt = calls[brief["id"]]["input"]["prompt"]
        assert brief["prompt"] == prompt
        assert brief["prompt_chars"] == len(prompt)
        assert brief["prompt_chars"] == want["prompt_chars"]
        assert f"`{want['spec']}`" in prompt
        assert brief["kind"] == "general-purpose"
        assert brief["parent"] is None
        assert brief["models"] == ["claude-sonnet-5-5"]


def test_s55_epic_children_endings_durations_and_background():
    children, _ = transcripts.claude_children(transcript(DATA / "s55-epic"))
    assert [c.duration_s for c in children] == [109.605, 111.142, 145.246]
    for child, want in zip(children, S55_CHILDREN):
        assert child.id == want["id"]
        assert child.ending == "completed"
        assert child.hand_back == want["hand_back"]
        assert child.duration_s == want["duration_s"]
        assert child.background is want["background"]
        assert child.parent is None
        assert child.kind == "general-purpose"
        assert child.models == ["claude-sonnet-5-5"]


# Token, model and cost figures, from the transcripts alone.


def test_every_reference_run_is_pinned():
    assert sorted(PINNED_RUNS) == sorted(p.name for p in DATA.iterdir() if (p / "claude-code.txt").is_file())
    assert set(PINNED_TOTALS) == set(PINNED_MODELS_USED) == set(PINNED_RUNS)


@pytest.mark.parametrize("name", PINNED_RUNS)
def test_reference_totals_are_pinned(name):
    totals = transcripts.claude_summary(transcript(DATA / name))
    for field, want in PINNED_TOTALS[name].items():
        assert getattr(totals, field) == want, field
    assert totals.model.split("+") == PINNED_MODELS_USED[name]
    assert totals.unknowns == []


@pytest.mark.parametrize("name", PINNED_RUNS)
def test_reference_record_totals_match_the_pins_without_spektacular_logs(name):
    record = record_of(DATA / name, log_dir=NONE)
    pinned = PINNED_TOTALS[name]
    assert record["run"]["model"] == pinned["model"]
    for field in transcripts.TOTAL_FIELDS:
        assert record["totals"][field] == pinned[field], field
    assert_log_sections_unknown(record, f"no Spektacular logs at {NONE}")


@pytest.mark.parametrize("name", PINNED_RUNS)
def test_reference_record_has_its_log_sections_unknown_and_its_briefs(name):
    # A reference run holds no Spektacular run logs: every step, helper and
    # wait measure is unknown with the reader's reason, while tokens, models,
    # cost and briefs still come from the transcript.
    run = DATA / name
    record = record_of(run)
    assert_log_sections_unknown(record, f"no Spektacular logs at {run / 'logs'}")
    children, _ = transcripts.claude_children(transcript(run))
    assert [b["id"] for b in record["subagents"]["briefs"]] == [c.id for c in children]
    assert record["schema_version"] == 3


# Compact and free of conversation text.


@pytest.mark.parametrize("name", RUNS)
def test_reference_run_has_its_three_parts_and_no_old_debug_log(name):
    run = DATA / name
    assert sorted(p.name for p in run.iterdir()) == ["README", "build.json", "claude-code.txt"]
    assert json.loads((run / "build.json").read_text()) == BUILD[name]
    readme = (run / "README").read_text()
    assert readme.startswith(f"{name}: trimmed from harbor job ")
    assert "is not kept" in readme and "no logs/" in readme


@pytest.mark.parametrize("name", RUNS)
def test_reference_run_is_compact(name):
    assert sum(len(b) for b in tree(DATA / name).values()) < MAX_RUN_BYTES


def test_reference_runs_together_are_compact():
    assert sum(len(b) for name in RUNS for b in tree(DATA / name).values()) < MAX_ALL_BYTES


@pytest.mark.parametrize("name", RUNS)
def test_reference_run_keeps_no_message_text(name):
    objs = transcript(DATA / name)
    blocks = [
        b
        for o in objs
        if o.get("type") in ("assistant", "user")
        for b in (o["message"]["content"] if isinstance(o["message"]["content"], list) else [])
    ]
    texts = [b["text"] for b in blocks if b["type"] == "text"]
    thoughts = [b["thinking"] for b in blocks if b["type"] == "thinking"]
    results = [b["content"] for b in blocks if b["type"] == "tool_result"]
    plain = [o["message"]["content"] for o in objs if o.get("type") in ("assistant", "user") and isinstance(o["message"]["content"], str)]
    assert texts and results
    for s in texts + thoughts + results + plain:
        assert isinstance(s, str) and set(s) <= {"x"}
    assert all(o["result"] == "" for o in objs if o.get("type") == "result")


@pytest.mark.parametrize("name", RUNS)
def test_reference_run_keeps_only_each_hand_backs_first_line(name):
    # The record reads a hand-back's first line; the rest of the child's report
    # is blanked line by line, and its progress notes entirely.
    objs = transcript(DATA / name)
    for o in objs:
        if o.get("subtype") == "task_notification":
            first, _, rest = o["summary"].lstrip("\n").partition("\n")
            assert first.strip() and set(rest) <= {"x", "\n"}
        if o.get("subtype") == "task_progress" and "description" in o:
            assert set(o["description"]) <= {"x"}


def test_s55_epic_hand_backs_keep_their_done_line():
    firsts = [o["summary"].split("\n", 1)[0] for o in transcript(DATA / "s55-epic") if o.get("subtype") == "task_notification"]
    assert firsts == ["DONE: 20260101000001-storage", "DONE: 20260101000003-help", "DONE: 20260101000002-commands"]


@pytest.mark.parametrize("name", RUNS)
def test_reference_run_blanks_tool_input_but_launches_and_commands(name):
    for o in transcript(DATA / name):
        if o.get("type") != "assistant":
            continue
        for b in o["message"]["content"]:
            if b.get("type") != "tool_use" or b["name"] in ("Agent", "Task", "ScheduleWakeup", "CronCreate"):
                continue
            for key, value in b["input"].items():
                if b["name"] == "Bash" and key in ("command", "run_in_background"):
                    continue
                if isinstance(value, str):
                    assert set(value) <= {"x"}, (b["name"], key)


# Regenerable, idempotent, and never changing a computed value.


@pytest.mark.parametrize("name", RUNS)
def test_trimming_a_reference_run_again_gives_the_same_bytes(name, tmp_path, scratch):
    out = tmp_path / name
    assert trim_reference.main([str(DATA / name), name, "--out", str(out)]) == 0
    assert tree(out) == tree(DATA / name)
    assert list(scratch.iterdir()) == []


@pytest.mark.parametrize("name", RUNS)
def test_trimming_never_changes_a_measured_value(name, tmp_path, scratch):
    out = tmp_path / name
    assert trim_reference.main([str(DATA / name), name, "--out", str(out)]) == 0
    assert trim_reference.measured(out, out / "logs") == trim_reference.measured(DATA / name, DATA / name / "logs")


# The ten-minute probe: one background child in a headless session, stopped
# when the session closed about 600 s after the main agent ended its turn.
# It never called spektacular, so it has no logs/.

PROBE = "probe-background-child"
PROBE_CHILD = "toolu_01Sqm7X3YSdf3jFnjDYNJhVv"
PROBE_LAUNCHED = datetime(2026, 10, 24, 10, 38, 47, 522000, tzinfo=timezone.utc)
PROBE_STOPPED = datetime(2026, 10, 24, 10, 48, 49, 532000, tzinfo=timezone.utc)  # task_updated end_time
PROBE_MAIN_LAST = datetime(2026, 10, 24, 10, 38, 49, 380000, tzinfo=timezone.utc)
PROBE_OUTCOME = (
    "stopped 602 s 2026-10-24 claude-code 2.1.310: headless (claude --print) background child killed"
    " ~600 s after the main agent ended its turn; no completion notice started a new turn\n"
)


def test_probe_has_its_parts_and_no_logs():
    run = DATA / PROBE
    assert sorted(p.name for p in run.iterdir()) == ["OUTCOME", "README", "build.json", "claude-code.txt"]
    assert json.loads((run / "build.json").read_text()) == {"suite": "background-child", "agent": "claude", "model": "claude-sonnet-5-5"}
    assert (run / "README").read_text().startswith(f"{PROBE}: trimmed from harbor job ")


def test_probe_child_ran_in_the_background_past_ten_minutes_and_was_stopped():
    children, _ = transcripts.claude_children(transcript(DATA / PROBE))
    assert len(children) == 1
    child = children[0]
    assert child.id == PROBE_CHILD
    assert child.background is True
    assert child.ending == "stopped"
    assert child.hand_back is None
    assert child.duration_s is None
    assert child.launched_at == PROBE_LAUNCHED
    assert child.ended_at == PROBE_STOPPED
    assert (child.ended_at - child.launched_at).total_seconds() == 602.01
    assert (child.ended_at - PROBE_MAIN_LAST).total_seconds() > 600


def test_probe_has_one_turn_ending_at_the_main_agents_last_message_and_no_wakeups():
    objs = transcript(DATA / PROBE)
    assert transcripts.claude_turns(objs) == [transcripts.Turn(ended_at=PROBE_MAIN_LAST, woken_by="start")]
    assert transcripts.claude_wakeups(objs) == []


def test_probe_outcome_names_the_ending_duration_date_and_version():
    text = (DATA / PROBE / "OUTCOME").read_text()
    assert text == PROBE_OUTCOME
    assert text.partition(":")[0].split() == ["stopped", "602", "s", "2026-10-24", "claude-code", "2.1.310"]
    init = transcript(DATA / PROBE)[0]
    assert init["subtype"] == "init" and init["claude_code_version"] == "2.1.310"


def test_trimming_the_probe_again_gives_the_same_bytes_and_values(tmp_path, scratch):
    out = tmp_path / PROBE
    assert trim_reference.main([str(DATA / PROBE), PROBE, "--out", str(out)]) == 0
    assert not (out / "logs").exists()
    assert tree(out) == {k: v for k, v in tree(DATA / PROBE).items() if k != "OUTCOME"}
    assert trim_reference.measured(out, out / "logs") == trim_reference.measured(DATA / PROBE, DATA / PROBE / "logs")
    assert list(scratch.iterdir()) == []


def test_trim_refuses_an_existing_output(tmp_path, scratch):
    out = tmp_path / "s55-implement"
    out.mkdir()
    assert trim_reference.main([str(DATA / "s55-implement"), "s55-implement", "--out", str(out)]) == 1
    assert list(out.iterdir()) == []


# A small collected trial, laid out as harbor leaves it.

CHILD = "toolu_synthetic_child"
PROMPT = "Plan the spec `20260101000000-user-auth`. Hand back DONE first."


def synthetic_trial(root: Path) -> Path:
    agent = root / "agent"
    agent.mkdir(parents=True)
    (root / "artifacts" / "spektacular").mkdir(parents=True)
    (root / "config.json").write_text(
        json.dumps({"task": {"path": "tasks/plan-workflow-claude"}, "agent": {"name": "claude-code", "model_name": "claude-sonnet-5-5"}})
    )
    objs = [
        {"type": "system", "subtype": "init", "model": "claude-sonnet-5-5", "session_id": "s1", "tools": ["Agent"]},
        {
            "type": "assistant",
            "timestamp": "2026-10-23T10:00:00.000Z",
            "parent_tool_use_id": None,
            "message": {
                "model": "claude-sonnet-5-5",
                "role": "assistant",
                "content": [
                    {"type": "text", "text": "I will hand this to a child."},
                    {"type": "tool_use", "id": CHILD, "name": "Agent", "input": {"subagent_type": "general-purpose", "description": "Plan it", "prompt": PROMPT}},
                ],
            },
        },
        {
            "type": "system", "subtype": "task_started", "tool_use_id": CHILD, "task_id": "t1",
            "task_type": "local_agent", "is_backgrounded": False, "spawn_depth": 1,
        },
        {
            "type": "assistant",
            "timestamp": "2026-10-23T10:00:10.000Z",
            "parent_tool_use_id": CHILD,
            "message": {"model": "claude-sonnet-5-5", "role": "assistant", "content": [{"type": "text", "text": "Planning now."}]},
        },
        {
            "type": "system", "subtype": "task_notification", "tool_use_id": CHILD, "task_id": "t1",
            "status": "completed", "summary": "DONE: 20260101000000-user-auth", "usage": {"duration_ms": 20000},
        },
        {
            "type": "user",
            "timestamp": "2026-10-23T10:00:20.000Z",
            "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": CHILD, "content": "DONE: 20260101000000-user-auth"}]},
        },
        {
            "type": "result", "subtype": "success", "num_turns": 2, "duration_ms": 30000, "duration_api_ms": 20000,
            "total_cost_usd": 0.1, "result": "All planned.",
            "subagent_stats": {"spawned": 1, "completed": 1, "failed": 0},
        },
    ]
    (agent / "claude-code.txt").write_text("".join(json.dumps(o) + "\n" for o in objs))
    return root


def test_trimming_a_collected_trial_blanks_its_text(tmp_path, scratch):
    trial = synthetic_trial(tmp_path / "job" / "trial")
    out = tmp_path / "out" / "synthetic"
    assert trim_reference.main([str(trial), "synthetic", "--out", str(out)]) == 0
    assert json.loads((out / "build.json").read_text()) == {"suite": "plan-workflow", "agent": "claude", "model": "claude-sonnet-5-5"}
    assert (out / "README").read_text() == "synthetic: trimmed from harbor job job, trial trial, by trim_reference.py\n"
    objs = transcript(out)
    assert objs[1]["message"]["content"][0] == {"type": "text", "text": "x" * 28}
    assert objs[1]["message"]["content"][1]["input"]["prompt"] == PROMPT
    assert objs[3]["message"]["content"] == [{"type": "text", "text": "x" * 13}]
    assert objs[5]["message"]["content"][0]["content"] == "x" * 30
    assert objs[6]["result"] == ""
    brief = trim_reference.measured(out, out / "logs")["subagents"]["briefs"][0]
    assert brief == {
        "id": CHILD, "parent": None, "kind": "general-purpose", "description": "Plan it",
        "models": ["claude-sonnet-5-5"], "prompt": PROMPT, "prompt_chars": 63,
    }
    assert list(scratch.iterdir()) == []


def test_trim_writes_nothing_when_the_trim_would_change_a_value(tmp_path, scratch, monkeypatch, capsys):
    trial = synthetic_trial(tmp_path / "job" / "trial")
    real = trim_reference.trim_transcript

    def corrupting(objs):
        trimmed = real(objs)
        for o in trimmed:
            if o.get("type") == "assistant" and o.get("parent_tool_use_id") == CHILD:
                o["message"]["model"] = "claude-haiku-4-5"  # changes the child's brief
        return trimmed

    monkeypatch.setattr(trim_reference, "trim_transcript", corrupting)
    out = tmp_path / "out" / "synthetic"
    assert trim_reference.main([str(trial), "synthetic", "--out", str(out)]) == 1
    assert not out.exists()
    assert not out.parent.exists()
    assert list(scratch.iterdir()) == []
    assert "subagents" in capsys.readouterr().err
