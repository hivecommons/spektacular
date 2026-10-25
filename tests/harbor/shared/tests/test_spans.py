"""Self-check for spans.py, the reader of the span_start and span_end
records Spektacular writes to its run logs (spec 000074).

Each case writes a small log directory by hand, in the shapes the CLI
writes, and checks the spans read back against hand-written values.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_spans.py
"""

import json
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from spans import SUB_AGENT, USER_WAIT, of_type, read_spans, sub_agents, user_waits  # noqa: E402

TRACE = "4bf92f3577b34da6a3ce929d0e0e4736"
OTHER = "11111111111111111111111111111111"
LANE = "20261023T100000.000000000Z_claude_plan_20260101000000-user-auth"


def at(hour, minute, second=0):
    return datetime(2026, 10, 23, hour, minute, second, tzinfo=timezone.utc)


def edge(kind, span_id, name, typ, start, end=None, *, parent=None, trace=TRACE, reason=None, status=None, step=None):
    rec = {
        "record": kind,
        "trace_id": trace,
        "span_id": span_id,
        "name": name,
        "kind": "internal",
        "start_time": f"2026-10-23T{start}.000000000Z",
        "attributes": {"spektacular.span.type": typ},
        "resource": {"service.name": "spektacular"},
    }
    if parent:
        rec["parent_span_id"] = parent
    if step:
        rec["attributes"]["spektacular.step"] = step
    if kind == "span_end":
        rec["end_time"] = f"2026-10-23T{end}.000000000Z"
        rec["status"] = status or {"code": "ok"}
        rec["attributes"]["spektacular.span.end_reason"] = reason or "moved"
    return rec


def command(clock, args, parent):
    return {
        "record": "span",
        "trace_id": TRACE,
        "span_id": "c" * 16,
        "parent_span_id": parent,
        "name": " ".join(args[:2]),
        "kind": "internal",
        "start_time": f"2026-10-23T{clock}.000000000Z",
        "end_time": f"2026-10-23T{clock}.500000000Z",
        "status": {"code": "ok"},
        "attributes": {"spektacular.span.type": "command", "process.command_args": args},
    }


def write(path: Path, records, tail=""):
    path.write_text("".join(json.dumps(r, separators=(",", ":")) + "\n" for r in records) + tail)


def test_starts_and_ends_pair_across_lane_and_day_files(tmp_path):
    write(
        tmp_path / f"{LANE}.jsonl",
        [
            edge("span_start", "a" * 16, "overview", "step", "10:00:00", step="overview"),
            command("10:00:10", ["trace", "start", "test-writer"], "a" * 16),
            edge("span_start", "b" * 16, "test-writer", SUB_AGENT, "10:00:20", parent="a" * 16, step="overview"),
            edge("span_start", "d" * 16, "ask scope", USER_WAIT, "10:01:00", parent="a" * 16),
            edge("span_end", "b" * 16, "test-writer", SUB_AGENT, "10:00:20", "10:02:00", parent="a" * 16, reason="stopped", step="overview"),
            edge("span_end", "a" * 16, "overview", "step", "10:00:00", "10:03:00"),
        ],
        tail='{"record":"span_start","trace_id":"4bf9',  # a partly written last line
    )
    write(
        tmp_path / "standalone_20261023.jsonl",
        [
            edge("span_start", "e" * 16, "child-storage", SUB_AGENT, "09:59:00", trace=OTHER),
            edge("span_end", "e" * 16, "child-storage", SUB_AGENT, "09:59:00", "10:05:00", trace=OTHER,
                 reason="stopped", status={"code": "error", "message": "child failed"}),
        ],
    )
    spans = read_spans(tmp_path)
    by_id = {s.span_id: s for s in spans}
    assert sorted(by_id) == ["a" * 16, "b" * 16, "d" * 16, "e" * 16]

    helper = by_id["b" * 16]
    assert (helper.name, helper.type, helper.parent, helper.step) == ("test-writer", SUB_AGENT, "a" * 16, "overview")
    assert (helper.start, helper.end, helper.status, helper.end_reason) == (at(10, 0, 20), at(10, 2), "ok", "stopped")
    assert helper.lane == LANE and not helper.open

    wait = by_id["d" * 16]
    assert wait.open and wait.end is None and wait.status is None

    child = by_id["e" * 16]
    assert (child.trace_id, child.status, child.lane) == (OTHER, "error", "standalone_20261023")

    assert [s.span_id for s in sub_agents(spans)] == ["b" * 16, "e" * 16]
    assert [s.span_id for s in user_waits(spans)] == ["d" * 16]
    assert [s.span_id for s in of_type(spans, "step")] == ["a" * 16]


def test_an_end_without_its_start_is_a_complete_span(tmp_path):
    write(tmp_path / f"{LANE}.jsonl", [edge("span_end", "f" * 16, "helper", SUB_AGENT, "10:00:00", "10:01:00", reason="abandoned",
                                            status={"code": "error", "message": "abandoned"})])
    [span] = read_spans(tmp_path)
    assert (span.start, span.end, span.end_reason, span.status) == (at(10, 0), at(10, 1), "abandoned", "error")


def test_the_same_span_id_in_two_traces_is_two_spans(tmp_path):
    write(tmp_path / "standalone_20261023.jsonl", [
        edge("span_start", "a" * 16, "one", SUB_AGENT, "10:00:00"),
        edge("span_start", "a" * 16, "two", SUB_AGENT, "10:00:01", trace=OTHER),
    ])
    assert sorted(s.name for s in read_spans(tmp_path)) == ["one", "two"]


def test_a_missing_log_directory_has_no_spans(tmp_path):
    assert read_spans(tmp_path / "nope") == []
