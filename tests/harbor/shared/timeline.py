#!/usr/bin/env python3
"""Prints where an orchestrated epic run spent its time.

Reads Spektacular's own logs (.spektacular/logs/), only through `spektacular
stats` (runlog.py), and reports, for each spec's lane, when its child started
and finished and its slowest step, then the hand-off gaps that separate a
slow child from a slow orchestrator. The orchestrator starts a `sub-agent`
span outside any run when it launches a child, and stops it when it handles
the child's hand-back (spec 000074); each child is matched to the span open
when its first command ran.

- spawned after: from the start of the child's span to the child's first
  command. Long means the child took a long time to start working.
- picked up after: from the child's last command to the end of its span,
  when the orchestrator stopped it on handling the hand-back; for a span
  never stopped, to the orchestrator's next recorded command or span. It
  still holds the child composing its hand-back, which the logs cannot see,
  so read it against the same gap for another agent.
- merged after: from the child's last command to the orchestrator's
  `epic merge` of its spec, for an implement run.

`status` is never logged, so these gaps no longer start or end at the
orchestrator's `status` polls, and are not comparable with timelines from
before spec 000076. A child launched under no span shows `no span`.

A lane started more than once had its child resumed: the report names the
step each earlier child stopped at. A child cut off by its agent stops at
whatever step it had reached, so stops scattered across steps point at a
limit on the sub-agent rather than at the workflow. When Bob's own logs are
present, the report also counts the times Bob ended a task for reaching its
turn limit (`session.maxTurns`).

It reports and never fails. Run it inside the container, or locally on a
job's collected artifacts, with a `spektacular` that has `stats` on PATH or
named by SPEKTACULAR_BIN:

    python3 timeline.py <trial>/artifacts/spektacular/logs [<trial>/artifacts/agent-logs/bob]
"""

from __future__ import annotations

import sys
from datetime import datetime, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from runlog import Command, Lane, RunLog, Span, Unavailable, event_at, parse_time  # noqa: E402

DEFAULT_DIR = Path("/app/.spektacular/logs")
DEFAULT_BOB_LOGS = Path("/root/.bob/logs")
BOB_TURN_LIMIT = "Reached maxTurns limit"


def bob_turn_limit_hits(log_dir: Path) -> int | None:
    """How many times Bob logged ending a task at its turn limit, or None
    when there are no Bob logs to read."""
    if not log_dir.is_dir():
        return None
    hits = 0
    for path in log_dir.rglob("*.log"):
        try:
            hits += path.read_text(errors="replace").count(BOB_TURN_LIMIT)
        except OSError:
            continue
    return hits


def span(delta: timedelta | None) -> str:
    if delta is None:
        return "-"
    seconds = int(delta.total_seconds())
    sign = "-" if seconds < 0 else ""
    m, s = divmod(abs(seconds), 60)
    return f"{sign}{m}m{s:02d}s"


def offset(origin: datetime, at: datetime | None) -> str:
    return "-" if at is None else "+" + span(at - origin)


def slowest_step(log: RunLog, lane: Lane) -> tuple[str, timedelta] | None:
    """The step with the longest duration over the lane's runs."""
    worst = None
    for run in lane.runs:
        for row in log.report(run["run_id"]).get("steps") or []:
            took = timedelta(milliseconds=row["duration_ms"])
            if worst is None or took > worst[1]:
                worst = (row["step"], took)
    return worst


def label(event: Command | Span) -> str:
    if isinstance(event, Command):
        return " ".join(event.args)[:48]
    return f"span {event.name}"[:48]


def report(log: RunLog, bob_logs: Path | None = None) -> str:
    try:
        runs, outside, spans = log.runs(), log.outside(), log.orchestrator_spans()
        lanes = sorted(log.lanes().values(), key=lambda l: l.start.at if l.start else parse_time(l.runs[0]["started_at"]) if l.runs else l.commands[0].at)
    except Unavailable as u:
        return f"== Epic run timeline ==\nNo Spektacular logs under {log.log_dir}: {u.reason}\n"
    if not runs and not outside:
        return f"== Epic run timeline ==\nNo Spektacular logs under {log.log_dir}: they hold no command.\n"

    times = [parse_time(r["started_at"]) for r in runs] + [c.at for c in outside] + [s.start for s in spans]
    ends = [parse_time(r["ended_at"]) for r in runs] + [c.end for c in outside] + [s.end for s in spans]
    origin, last = min(times), max(ends)

    out = ["== Epic run timeline (from Spektacular's logs, through `spektacular stats`) =="]
    out.append(
        f"Run: {origin:%H:%M:%S} to {last:%H:%M:%S} ({span(last - origin)}), "
        f"{len(log.commands())} spektacular calls, {len(spans)} orchestrator helper spans"
    )
    out.append("")
    out.append("Lanes, one per child workflow (times from the first call):")
    out.append(f"  {'lane':<34} {'start':>8} {'finish':>8} {'took':>8} {'steps':>5} {'children':>8}  slowest step")
    for lane in lanes:
        start = lane.start.at if lane.start else None
        finish = lane.finish.end if lane.finish else None
        took = finish - start if start and finish else None
        steps = sum(1 for c in lane.commands if c.goto_step and c.ok)
        slow = slowest_step(log, lane)
        slow_text = f"{slow[0]} {span(slow[1])}" if slow else "-"
        out.append(
            f"  {lane.kind + ' ' + lane.name:<34} {offset(origin, start):>8} "
            f"{offset(origin, finish):>8} {span(took):>8} {steps:>5} "
            f"{lane.children:>8}  {slow_text}"
        )
    unfinished = [l.name for l in lanes if not l.finish]
    if unfinished:
        out.append(f"  never reached finished: {', '.join(unfinished)}")

    out.append("")
    out.append("Hand-offs between the orchestrator and each child:")
    out.append(f"  {'lane':<34} {'spawned after':>14} {'picked up after':>16} {'merged after':>13}  picked up by")
    for lane in lanes:
        spawned_text = picked_text = "no span"
        what = ""
        first = log.child_span(lane.start) if lane.start else None
        if first is not None:
            spawned_text = span(lane.start.at - first.start)
        last_child = log.child_span(lane.child_starts[-1]) if lane.child_starts else None
        if not lane.finish:
            picked_text = "-"
        elif last_child is not None and last_child.complete:
            picked_text, what = span(last_child.end - lane.last.end), "hand-back"
        elif last_child is not None:
            reaction = log.orchestrator_reaction(lane)
            if reaction is None:
                picked_text = "no reaction"
            else:
                picked_text, what = span(event_at(reaction, lane.last.end) - lane.last.end), label(reaction)
        merge = log.merge(lane.name)
        merged = merge.at - lane.finish.end if merge and lane.finish else None
        out.append(
            f"  {lane.kind + ' ' + lane.name:<34} {spawned_text:>14} {picked_text:>16} "
            f"{span(merged):>13}  {what}"
        )

    resumed = [l for l in lanes if l.children > 1]
    out.append("")
    if resumed:
        out.append("Resumed children (each earlier child stopped without finishing):")
        for lane in resumed:
            out.append(
                f"  {lane.kind + ' ' + lane.name:<34} {lane.children - 1} resume(s); "
                f"stopped at: {', '.join(lane.stops)}"
            )
    else:
        out.append("Resumed children: none; every child finished in one go.")

    hits = bob_turn_limit_hits(bob_logs) if bob_logs else None
    if hits is not None:
        out.append(f"Bob tasks ended at the turn limit (\"{BOB_TURN_LIMIT}\" in its logs): {hits}")
    out.append("")
    return "\n".join(out)


def main(argv: list[str]) -> int:
    log_dir = Path(argv[1]) if len(argv) > 1 else DEFAULT_DIR
    bob_logs = Path(argv[2]) if len(argv) > 2 else DEFAULT_BOB_LOGS
    try:
        print(report(RunLog(log_dir), bob_logs))
    except Exception as exc:  # it reports and never fails
        print(f"== Epic run timeline ==\nNo timeline: {exc}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
