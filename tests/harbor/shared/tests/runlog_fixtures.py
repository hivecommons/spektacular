"""Hand-built Spektacular logs for the run log reader's self-tests.

An epic trial written as the CLI writes it at the default log level
(spec 000073's records, spec 000074's spans): two plan lanes launched
together, one of them resumed by a second child through a refused
`plan new`, and the orchestrator's records outside any run: `epic` and
`trace` commands, and one `sub-agent` span per child, the last never
stopped. `records()` gives the JSON Lines records by file; `stats()` gives
what `spektacular stats` answers for them, so test_runlog.py runs with no
binary, and test_runlog_integration.py proves the two agree. `reports()`
gives each plan run's `stats run` report, as far as the metrics recorder
reads it.

A spec trial too: one spec run whose overview waits on the user and whose
discovery starts a helper, with `spec_records()`, `spec_stats()` and
`spec_report()` the same way, for the metrics recorder's step split.

This file never runs in a harbor container: it lives under shared/tests/.
"""

from __future__ import annotations

import json
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path

T0 = datetime(2026, 10, 25, 10, 0, 0, tzinfo=timezone.utc)
ALPHA = "000001_alpha"
BETA = "000002_beta"
TRACE_A = "aaaaaaaa00000000000000000000000a"
TRACE_B = "bbbbbbbb00000000000000000000000b"
ORCH = {  # the orchestrator's traces outside any run
    "order": "0000000100000000000000000000000f",
    "span_a": "0000000200000000000000000000000f",
    "span_b1": "0000000300000000000000000000000f",
    "span_b2": "0000000400000000000000000000000f",
    "stop_b1": "0000000500000000000000000000000f",
    "stop_a": "0000000600000000000000000000000f",
    "read": "0000000700000000000000000000000f",
    "merge": "0000000800000000000000000000000f",
}


def at(minutes: float) -> datetime:
    return T0 + timedelta(minutes=minutes)


def stamp(t: datetime) -> str:
    return t.strftime("%Y-%m-%dT%H:%M:%S.") + f"{t.microsecond:06d}000Z"


def _attrs(span_type: str, kind: str | None, name: str | None, **extra) -> dict:
    a = {"spektacular.span.type": span_type, "spektacular.agent": "claude"}
    if kind:
        a["spektacular.workflow.kind"] = kind
        a["spektacular.workflow.name"] = name
    a.update({k.replace("_", "."): v for k, v in extra.items() if v is not None})
    return a


_seq = [0]


def command_name(args: list[str]) -> str:
    """The command's name as the CLI records it: its leading words."""
    words = []
    for a in args[:3]:
        if not re.fullmatch(r"[a-z]+", a):
            break
        words.append(a)
    return " ".join(words)


def command(trace, kind, name, args, start, end, *, step=None, step_to=None, parent=None, error=None):
    _seq[0] += 1
    attrs = _attrs("command", kind, name)
    attrs["process.command_args"] = args
    attrs["process.exit_code"] = 1 if error else 0
    if kind and step:
        attrs["spektacular.step"] = step
    if kind and step_to:
        attrs["spektacular.step.to"] = step_to
    rec = {
        "record": "span", "trace_id": trace, "span_id": f"c{_seq[0]:015d}", "name": command_name(args),
        "kind": "internal", "start_time": stamp(at(start)), "end_time": stamp(at(end)),
        "status": {"code": "error", "message": error} if error else {"code": "ok"},
        "attributes": attrs, "resource": {"service.name": "spektacular"},
    }
    if parent:
        rec["parent_span_id"] = parent
    return rec


def span(trace, kind, name, span_id, span_name, span_type, start, end=None, reason="stopped", *, parent=None, step=None):
    attrs = _attrs(span_type, kind, name)
    if step:
        attrs["spektacular.step"] = step
    first = {
        "record": "span_start", "trace_id": trace, "span_id": span_id, "name": span_name, "kind": "internal",
        "start_time": stamp(at(start)), "attributes": attrs, "resource": {"service.name": "spektacular"},
    }
    if parent:
        first["parent_span_id"] = parent
    if end is None:
        return [first]
    last = dict(first, record="span_end", end_time=stamp(at(end)),
                status={"code": "error", "message": reason} if reason in ("abandoned", "interrupted") else {"code": "ok"})
    last["attributes"] = dict(attrs, **{"spektacular.span.end_reason": reason})
    return [first, last]


def goto(step: str, name: str) -> list[str]:
    return ["plan", "goto", "--data", json.dumps({"step": step, "name": name})]


def new(name: str) -> list[str]:
    return ["plan", "new", "--data", json.dumps({"name": name, "orchestrated": True})]


def records() -> dict[str, list[dict]]:
    """The trial's log files, by name."""
    _seq[0] = 0
    a = (TRACE_A, "plan", ALPHA)
    b = (TRACE_B, "plan", BETA)
    alpha = [
        command(*a, new(ALPHA), 2, 2.1, step_to="discovery"),
        *span(*a, "sa1", "discovery", "step", 2.1, 10, "moved"),
        *span(*a, "ha1", "research", "sub-agent", 3, 5, parent="sa1", step="discovery"),
        command(*a, goto("architecture", ALPHA), 10, 10.1, step="discovery", step_to="architecture", parent="sa1"),
        *span(*a, "sa2", "architecture", "step", 10.1, 15.1, "moved"),
        command(*a, ["changelog", "file", "write", ALPHA, "--from", "x.md"], 12, 12.1, step="architecture", parent="sa2"),
        command(*a, ["changelog", "file", "write", ALPHA, "--repo", "docs", "--from", "y.md"], 12.2, 12.3, step="architecture", parent="sa2"),
        command(*a, ["changelog", "file", "write", "--repo=web", ALPHA, "--from", "z.md"], 12.4, 12.5, step="architecture", parent="sa2"),
        command(*a, ["changelog", "file", "write", ALPHA, "--help"], 12.6, 12.7, step="architecture", parent="sa2"),
        command(*a, ["changelog", "file", "write", ALPHA, "--from", "bad.md"], 12.8, 12.9, step="architecture", parent="sa2", error="no such file"),
        command(*a, goto("finished", ALPHA), 15, 15.1, step="architecture", step_to="finished", parent="sa2"),
        *span(*a, "sa3", "finished", "step", 15.1, 15.1, "finished"),
    ]
    beta = [
        command(*b, new(BETA), 2.5, 2.6, step_to="discovery"),
        *span(*b, "sb1", "discovery", "step", 2.6, 6, "moved"),
        command(*b, goto("architecture", BETA), 6, 6.1, step="discovery", step_to="architecture", parent="sb1"),
        *span(*b, "sb2", "architecture", "step", 6.1, 8, "interrupted"),
        command(*b, ["plan", "file", "read", BETA, "plan"], 7.9, 8, step="architecture", parent="sb2"),
        command(*b, new(BETA), 11, 11.1, step="architecture", parent="sb2", error="a plan workflow is in progress"),
        *span(*b, "sb3", "architecture", "step", 11.1, 16.1, "moved"),
        command(*b, goto("finished", BETA), 16, 16.1, step="architecture", step_to="finished", parent="sb3"),
        *span(*b, "sb4", "finished", "step", 16.1, 16.1, "finished"),
    ]
    o = lambda key: (ORCH[key], None, None)  # noqa: E731
    outside = [
        command(*o("order"), ["epic", "order", "--data", '{"name":"e"}'], 0, 0.5),
        *span(*o("span_a"), "oa", f"plan {ALPHA}", "sub-agent", 1, 20),
        *span(*o("span_b1"), "ob1", f"plan {BETA}", "sub-agent", 1.5, 9),
        command(*o("stop_b1"), ["trace", "stop", f"{ORCH['span_b1']}-ob1"], 9, 9.1),
        *span(*o("span_b2"), "ob2", f"plan {BETA} (resume)", "sub-agent", 10.5),
        command(*o("read"), ["plan", "file", "read", ALPHA, "plan"], 17, 17.1),
        command(*o("stop_a"), ["trace", "stop", f"{ORCH['span_a']}-oa"], 20, 20.1),
        command(*o("merge"), ["epic", "merge", "--data", json.dumps({"spec": ALPHA})], 22, 22.5),
    ]
    return {
        f"20261025T100200.000000000Z_claude_plan_{ALPHA}.jsonl": alpha,
        f"20261025T100230.000000000Z_claude_plan_{BETA}.jsonl": beta,
        "standalone_20261025.jsonl": outside,
    }


def write(log_dir: Path) -> Path:
    log_dir.mkdir(parents=True, exist_ok=True)
    for name, recs in records().items():
        (log_dir / name).write_text("".join(json.dumps(r) + "\n" for r in recs))
    return log_dir


# What `spektacular stats` answers for these records, written by hand from
# the contract in internal/runstats/listings.go. Only the fields the reader
# reads are given for `stats list`.


def _t(value: str) -> datetime:
    return datetime.fromisoformat(value[:26] + "+00:00")


def _cmd_row(r: dict) -> dict:
    a = r["attributes"]
    in_run = "spektacular.workflow.kind" in a
    return {
        "run_id": r["trace_id"] if in_run else None,
        "kind": a.get("spektacular.workflow.kind"), "name": a.get("spektacular.workflow.name"),
        "agent": "claude", "span_id": r["span_id"], "parent_span_id": r.get("parent_span_id"),
        "command": r["name"], "args": a["process.command_args"],
        "step": a.get("spektacular.step"), "step_to": a.get("spektacular.step.to"),
        "started_at": r["start_time"], "ended_at": r["end_time"],
        "duration_ms": round((_t(r["end_time"]) - _t(r["start_time"])).total_seconds() * 1000),
        "exit_code": a["process.exit_code"],
        "status": r["status"]["code"], "message": r["status"].get("message"),
    }


def _span_row(first: dict, last: dict | None) -> dict:
    a = first["attributes"]
    in_run = "spektacular.workflow.kind" in a
    return {
        "span_id": first["span_id"], "parent_span_id": first.get("parent_span_id"), "name": first["name"],
        "step": a.get("spektacular.step"), "task_id": None,
        "started_at": first["start_time"], "ended_at": (last or first)["end_time" if last else "start_time"],
        "end_reason": last["attributes"]["spektacular.span.end_reason"] if last else "open",
        "status": last["status"]["code"] if last else None, "message": None, "complete": last is not None,
        "type": a["spektacular.span.type"], "trace_id": first["trace_id"],
        "run_id": first["trace_id"] if in_run else None,
        "kind": a.get("spektacular.workflow.kind"), "workflow": a.get("spektacular.workflow.name"),
    }


def stats() -> dict[str, dict]:
    """The `stats list`, `stats commands` and `stats spans` answers."""
    runs = [
        {"run_id": TRACE_A, "kind": "plan", "name": ALPHA, "agent": "claude", "started_at": stamp(at(2)), "ended_at": stamp(at(15.1)), "status": "finished"},
        {"run_id": TRACE_B, "kind": "plan", "name": BETA, "agent": "claude", "started_at": stamp(at(2.5)), "ended_at": stamp(at(16.1)), "status": "finished"},
    ]
    return _answers(records(), runs)


def _answers(files: dict[str, list[dict]], runs: list[dict]) -> dict[str, dict]:
    every = [r for recs in files.values() for r in recs]
    cmds = sorted((_cmd_row(r) for r in every if r["record"] == "span"), key=lambda c: (c["started_at"], c["span_id"]))
    starts = {(r["trace_id"], r["span_id"]): r for r in every if r["record"] == "span_start"}
    ends = {(r["trace_id"], r["span_id"]): r for r in every if r["record"] == "span_end"}
    spans = sorted(
        (_span_row(s, ends.get(k)) for k, s in starts.items() if s["attributes"]["spektacular.span.type"] in ("sub-agent", "user-wait", "work")),
        key=lambda s: (s["started_at"], s["span_id"]),
    )
    source = {"error": False, "log_dir": "/logs", "skipped_lines": 0}
    return {
        "list": {**source, "runs": runs},
        "commands": {**source, "commands": cmds},
        "spans": {**source, "spans": spans},
    }


# Each plan run's `stats run` report, written by hand from the records above
# and the contract in internal/runstats/report.go: only the fields the
# metrics recorder reads. Go writes a whole-second time without a fraction.


def iso(minutes: float) -> str:
    t = at(minutes)
    return t.strftime("%Y-%m-%dT%H:%M:%S") + (f".{t.microsecond:06d}".rstrip("0") if t.microsecond else "") + "Z"


def ms(minutes: float) -> int:
    return round(minutes * 60_000)


def step_row(step, entries, minutes, commands, helper=0.0, user=0.0, incomplete=0) -> dict:
    return {
        "step": step, "entries": entries, "duration_ms": ms(minutes), "commands": commands,
        "helper_wait_ms": ms(helper), "user_wait_ms": ms(user), "incomplete": incomplete,
    }


def report_totals(wall, user=0.0, helper=0.0, gap=0.0, gaps=0) -> dict:
    return {"wall_ms": ms(wall), "user_wait_ms": ms(user), "helper_wait_ms": ms(helper), "gap_ms": ms(gap), "gaps_over_threshold": gaps}


def reports() -> dict[str, dict]:
    """`stats run --run <id>` for each plan run, by run id."""
    return {
        TRACE_A: {
            "gap_threshold_ms": 300000,
            # 10:02:00 to 10:15:06: the run's wall time
            "totals": report_totals(13.1, helper=2),
            "steps": [
                step_row("discovery", 1, 7.9, 1, helper=2),  # 2.1 to 10, research 3 to 5
                step_row("architecture", 1, 5, 6),  # 10.1 to 15.1: five changelog writes and the goto
                step_row("finished", 1, 0, 0),
            ],
            "step_entries": [
                {"step": "discovery", "started_at": iso(2.1)},
                {"step": "architecture", "started_at": iso(10.1)},
                {"step": "finished", "started_at": iso(15.1)},
            ],
            "tasks": [],
            "helpers": [{"span_id": "ha1", "step": "discovery"}],
        },
        TRACE_B: {
            "gap_threshold_ms": 300000,
            "totals": report_totals(13.6),  # 10:02:30 to 10:16:06
            "steps": [
                step_row("discovery", 1, 3.4, 1),  # 2.6 to 6
                step_row("architecture", 2, 1.9 + 5, 3, incomplete=1),  # 6.1 to 8 interrupted, 11.1 to 16.1
                step_row("finished", 1, 0, 0),
            ],
            "step_entries": [
                {"step": "discovery", "started_at": iso(2.6)},
                {"step": "architecture", "started_at": iso(6.1)},
                {"step": "architecture", "started_at": iso(11.1)},
                {"step": "finished", "started_at": iso(16.1)},
            ],
            "tasks": [],
            "helpers": [],
        },
    }


# The spec trial.

TRACE_S = "cccccccc00000000000000000000000c"
GAMMA = "000003_gamma"


def spec_goto(step: str) -> list[str]:
    return ["spec", "goto", "--data", json.dumps({"step": step, "name": GAMMA})]


def spec_records() -> dict[str, list[dict]]:
    """One spec run: overview 0.1 to 5 with the user approving 1 to 3,
    discovery 5.1 to 12 with a helper researching 6 to 9, then finished."""
    _seq[0] = 0
    s = (TRACE_S, "spec", GAMMA)
    run = [
        command(*s, ["spec", "new", "--data", json.dumps({"name": GAMMA})], 0, 0.1, step_to="overview"),
        *span(*s, "ss1", "overview", "step", 0.1, 5, "moved"),
        command(*s, ["trace", "start", "approval", "--type", "user-wait"], 0.9, 1, step="overview", parent="ss1"),
        *span(*s, "us1", "approval", "user-wait", 1, 3, parent="ss1", step="overview"),
        command(*s, ["trace", "stop", f"{TRACE_S}-us1"], 3, 3.1, step="overview", parent="ss1"),
        command(*s, spec_goto("discovery"), 5, 5.1, step="overview", step_to="discovery", parent="ss1"),
        *span(*s, "ss2", "discovery", "step", 5.1, 12, "moved"),
        command(*s, ["trace", "start", "research", "--type", "sub-agent"], 5.9, 6, step="discovery", parent="ss2"),
        *span(*s, "hs1", "research", "sub-agent", 6, 9, parent="ss2", step="discovery"),
        command(*s, ["trace", "stop", f"{TRACE_S}-hs1"], 9, 9.1, step="discovery", parent="ss2"),
        command(*s, spec_goto("finished"), 12, 12.1, step="discovery", step_to="finished", parent="ss2"),
        *span(*s, "ss3", "finished", "step", 12.1, 12.1, "finished"),
    ]
    return {f"20261025T100000.000000000Z_claude_spec_{GAMMA}.jsonl": run}


def write_spec(log_dir: Path) -> Path:
    log_dir.mkdir(parents=True, exist_ok=True)
    for name, recs in spec_records().items():
        (log_dir / name).write_text("".join(json.dumps(r) + "\n" for r in recs))
    return log_dir


def spec_stats() -> dict[str, dict]:
    """The spec trial's `stats list`, `stats commands` and `stats spans` answers."""
    runs = [{
        "run_id": TRACE_S, "kind": "spec", "name": GAMMA, "agent": "claude",
        "started_at": stamp(at(0)), "ended_at": stamp(at(12.1)), "status": "finished",
    }]
    return _answers(spec_records(), runs)


def spec_report() -> dict:
    """`stats run --run TRACE_S`, as far as the recorder reads it."""
    return {
        "gap_threshold_ms": 300000,
        "totals": report_totals(12.1, user=2, helper=3),
        "steps": [
            step_row("overview", 1, 4.9, 3, user=2),  # 0.1 to 5
            step_row("discovery", 1, 6.9, 3, helper=3),  # 5.1 to 12
            step_row("finished", 1, 0, 0),
        ],
        "step_entries": [
            {"step": "overview", "started_at": iso(0.1)},
            {"step": "discovery", "started_at": iso(5.1)},
            {"step": "finished", "started_at": iso(12.1)},
        ],
        "tasks": [],
        "helpers": [{"span_id": "hs1", "step": "discovery"}],
    }
