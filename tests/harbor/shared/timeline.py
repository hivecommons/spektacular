#!/usr/bin/env python3
"""Prints where an orchestrated epic run spent its time.

Reads Spektacular's session log (.spektacular/logs/) and reports, for each spec's lane,
when its child started and finished and its slowest step, then the two
hand-off gaps that separate a slow child from a slow orchestrator:

- spawned after: from the orchestrator's last `status <epic>` poll before
  the child's first command, to that command. Long means the child took a
  long time to start working.
- picked up after: from the child's last command to the orchestrator's
  first reaction: an epic status poll, an `epic` command, or a command
  naming another spec (see SessionLog.orchestrator_reaction). Long means the
  orchestrator was slow to pick up a finished child. This gap also holds the
  child composing its hand-back, which the log cannot see, so read it
  against the same gap for another agent.
- merged after: from the child's last command to the orchestrator's
  `epic merge` of its spec, for an implement run.

A lane started more than once had its child resumed: the report names the
step each earlier child stopped at. A child cut off by its agent stops at
whatever step it had reached, so stops scattered across steps point at a
limit on the sub-agent rather than at the workflow. When Bob's own logs are
present, the report also counts the times Bob ended a task for reaching its
turn limit (`session.maxTurns`).

It reports and never fails. Run it inside the container, or locally on a
job's collected artifacts:

    python3 timeline.py <trial>/artifacts/spektacular/logs [<trial>/artifacts/agent-logs/bob]
"""

from __future__ import annotations

import sys
from datetime import datetime, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from sessionlog import Event, Lane, SessionLog  # noqa: E402

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


def slowest_step(lane: Lane) -> tuple[str, timedelta] | None:
    """The longest gap between two of the lane's commands, and its step."""
    worst = None
    for prev, cur in zip(lane.events, lane.events[1:]):
        gap = cur.at - prev.end
        if worst is None or gap > worst[1]:
            worst = (prev.step_after or "?", gap)
    return worst


def last_before(polls: list[Event], at: datetime) -> Event | None:
    before = [p for p in polls if p.at <= at]
    return before[-1] if before else None


def report(log: SessionLog, bob_logs: Path | None = None) -> str:
    if log.empty:
        return (
            f"== Epic run timeline ==\nNo session log under {log.debug_dir}: "
            "the seeded project's log.level: verbose did not reach the agent, "
            "or it ran no spektacular command.\n"
        )

    events = log.all_events()
    origin, last = events[0].at, events[-1].end
    polls = log.epic_polls()
    lanes = sorted(log.lanes.values(), key=lambda l: l.start.at if l.start else last)

    out = ["== Epic run timeline (from the spektacular session log) =="]
    out.append(
        f"Run: {origin:%H:%M:%S} to {last:%H:%M:%S} ({span(last - origin)}), "
        f"{len(events)} spektacular calls, {len(polls)} epic status polls"
    )
    out.append("")
    out.append("Lanes, one per child workflow (times from the first call):")
    header = f"  {'lane':<34} {'start':>8} {'finish':>8} {'took':>8} {'steps':>5} {'children':>8}  slowest step"
    out.append(header)
    for lane in lanes:
        start = lane.start.at if lane.start else None
        finish = lane.finish.end if lane.finish else None
        took = finish - start if start and finish else None
        steps = sum(1 for e in lane.events if e.goto_step and e.ok)
        slow = slowest_step(lane)
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
    out.append(
        f"  {'lane':<34} {'spawned after':>14} {'picked up after':>16} {'merged after':>13}  first reaction"
    )
    for lane in lanes:
        spawned = picked = merged = None
        reaction = log.orchestrator_reaction(lane)
        merge = log.merge(lane.name)
        if lane.start:
            poll = last_before(polls, lane.start.at)
            spawned = lane.start.at - poll.end if poll else None
        if reaction:
            picked = reaction.at - lane.finish.end
        if merge and lane.finish:
            merged = merge.at - lane.finish.end
        picked_text = span(picked) if reaction else ("no reaction" if lane.finish else "-")
        what = " ".join(reaction.command)[:48] if reaction else ""
        out.append(
            f"  {lane.kind + ' ' + lane.name:<34} {span(spawned):>14} {picked_text:>16} "
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
    debug_dir = Path(argv[1]) if len(argv) > 1 else DEFAULT_DIR
    bob_logs = Path(argv[2]) if len(argv) > 2 else DEFAULT_BOB_LOGS
    print(report(SessionLog(debug_dir), bob_logs))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
