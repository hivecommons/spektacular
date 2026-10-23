"""Reads Spektacular's debug session log back into lanes and shared events.

Every `spektacular` call is one JSON line under `.spektacular/debug/`. A file
is named `<started>_<agent>_<session>.jsonl`, where the session is
`<kind>_<name>` for an orchestrated plan or implement lane and
`no-active-workflow` for everything else. An orchestrated lane's file holds
its `new` and every `goto`. The orchestrator's `status <epic>` polls, and a
child's other calls, land in the shared file.

The log is written by the CLI, not read from an agent transcript, so
everything here reads the same whichever agent drove the run.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from pathlib import Path

LANE_KINDS = ("plan", "implement")


def lane_file(kinds: tuple[str, ...] = LANE_KINDS) -> re.Pattern:
    """Matches a lane file of one of the given workflow kinds."""
    return re.compile(
        r"^\d{8}T\d{6}\.\d+Z_[^_]+_(?P<kind>" + "|".join(kinds) + r")_(?P<name>.+)\.jsonl$"
    )


LANE_FILE = lane_file()

# Go writes nanoseconds; datetime holds microseconds.
_FRACTION = re.compile(r"^(?P<head>.*\.\d{6})\d*(?P<tail>.*)$")


def parse_time(value: str) -> datetime:
    m = _FRACTION.match(value)
    return datetime.fromisoformat(m["head"] + m["tail"] if m else value)


@dataclass
class Event:
    at: datetime
    command: list[str]
    duration_ms: int
    exit_code: int
    state_before: dict | None
    state_after: dict | None
    response: str = ""

    @property
    def end(self) -> datetime:
        return self.at + timedelta(milliseconds=self.duration_ms)

    @property
    def ok(self) -> bool:
        return self.exit_code == 0

    def data(self) -> dict:
        """The command's --data JSON, or {} when it has none."""
        args = self.command
        for i, arg in enumerate(args):
            raw = None
            if arg in ("--data", "-d") and i + 1 < len(args):
                raw = args[i + 1]
            elif arg.startswith("--data="):
                raw = arg.removeprefix("--data=")
            if raw is not None:
                try:
                    parsed = json.loads(raw)
                except json.JSONDecodeError:
                    return {}
                return parsed if isinstance(parsed, dict) else {}
        return {}

    def is_verb(self, *verb: str) -> bool:
        return self.command[: len(verb)] == list(verb)

    @property
    def goto_step(self) -> str | None:
        if len(self.command) >= 2 and self.command[1] == "goto":
            return self.data().get("step")
        return None

    @property
    def step_after(self) -> str | None:
        """The step the workflow stood at once this command returned."""
        if self.state_after:
            return self.state_after.get("current_step")
        return self.goto_step


def read_events(path: Path) -> list[Event]:
    events = []
    for line in path.read_text().splitlines():
        if not line.strip():
            continue
        try:
            raw = json.loads(line)
        except json.JSONDecodeError:
            continue
        events.append(
            Event(
                at=parse_time(raw["timestamp"]),
                command=raw.get("command") or [],
                duration_ms=raw.get("duration_ms") or 0,
                exit_code=raw.get("exit_code", 0),
                state_before=raw.get("state_before"),
                state_after=raw.get("state_after"),
                response=raw.get("response") or "",
            )
        )
    return events


RESUME_REFUSAL = "workflow_in_progress"


@dataclass
class StepWindow:
    """One stretch of a lane spent at one workflow step."""

    step: str
    start: datetime
    end: datetime

    @property
    def duration_s(self) -> float:
        return (self.end - self.start).total_seconds()


@dataclass
class Lane:
    """Every command filed under one orchestrated workflow, across its files.

    A lane gets a new file each time `<kind> new` succeeds. A child that
    resumes a lane already in progress runs the same `<kind> new`, which the
    CLI refuses with `workflow_in_progress` and a resume report; that refusal
    is logged in the lane and marks where one child stopped and the next
    took over.
    """

    kind: str
    name: str
    files: int = 0
    events: list[Event] = field(default_factory=list)
    segments: list[list[Event]] = field(default_factory=list)

    @property
    def resumes(self) -> list[Event]:
        """Each `<kind> new` that resumed the lane instead of starting it."""
        return [
            e
            for e in self.events
            if e.is_verb(self.kind, "new") and not e.ok and RESUME_REFUSAL in e.response
        ]

    @property
    def children(self) -> int:
        """How many children worked the lane: each start and each resume."""
        return len(self.starts) + len(self.resumes)

    @property
    def stops(self) -> list[str]:
        """The step the lane stood at each time a child stopped and the next
        one took over.

        A child that was cut off, rather than finishing, leaves its lane at
        the step it was working on, and the next child resumes from there.
        """
        out = []
        for e in self.resumes:
            step = (e.state_before or {}).get("current_step")
            out.append(step or "?")
        for seg in self.segments[1:]:
            first = seg[0]
            out.append((first.state_before or {}).get("current_step") or "restart")
        return out

    @property
    def starts(self) -> list[Event]:
        return [e for e in self.events if e.is_verb(self.kind, "new") and e.ok]

    @property
    def start(self) -> Event | None:
        return self.starts[0] if self.starts else None

    @property
    def finish(self) -> Event | None:
        for e in self.events:
            if e.ok and e.goto_step == "finished":
                return e
        return None

    def step_windows(self) -> list[StepWindow]:
        """The steps the lane entered, in order, each with its start and end.

        A step starts at the command that moved the workflow onto it and ends
        at the command that moved it on; a failed command moves nothing. The
        last step of each child's run, `finished` included, ends with its last
        command. A new file or a resumed child always opens a fresh window,
        so the hand-off between children is never counted as time in a step.
        """
        resumes = {id(e) for e in self.resumes}
        out: list[StepWindow] = []
        for segment in self.segments:
            events = sorted(segment, key=lambda e: e.at)
            step: str | None = None
            start: datetime | None = None
            last: Event | None = None
            for e in events:
                if id(e) in resumes:
                    if step is not None and last is not None:
                        out.append(StepWindow(step, start, last.end))
                    step = (e.state_after or e.state_before or {}).get("current_step") or step
                    start = e.at
                elif e.ok and e.step_after:
                    if step is None:
                        step, start = e.step_after, e.at
                    elif e.step_after != step:
                        out.append(StepWindow(step, start, e.at))
                        step, start = e.step_after, e.at
                last = e
            if step is not None and events:
                out.append(StepWindow(step, start, max(e.end for e in events)))
        return out


class SessionLog:
    def __init__(self, debug_dir: Path, kinds: tuple[str, ...] = LANE_KINDS):
        """Reads every log file under debug_dir. Files of the given workflow
        kinds become lanes; the epic readers use plan and implement only, and
        the metrics recorder adds spec, whose single-agent runs also log to a
        file of their own."""
        pattern = LANE_FILE if kinds == LANE_KINDS else lane_file(kinds)
        self.debug_dir = Path(debug_dir)
        self.lanes: dict[tuple[str, str], Lane] = {}
        self.shared: list[Event] = []
        if not self.debug_dir.is_dir():
            return
        for path in sorted(self.debug_dir.glob("*.jsonl")):
            m = pattern.match(path.name)
            if m:
                key = (m["kind"], m["name"])
                lane = self.lanes.setdefault(key, Lane(kind=m["kind"], name=m["name"]))
                lane.files += 1
                segment = read_events(path)
                lane.segments.append(segment)
                lane.events.extend(segment)
            else:
                self.shared.extend(read_events(path))
        for lane in self.lanes.values():
            lane.events.sort(key=lambda e: e.at)
            lane.segments = sorted((seg for seg in lane.segments if seg), key=lambda seg: seg[0].at)
        self.shared.sort(key=lambda e: e.at)

    @property
    def empty(self) -> bool:
        return not self.lanes and not self.shared

    def lane(self, kind: str, name: str) -> Lane | None:
        return self.lanes.get((kind, name))

    def all_events(self) -> list[Event]:
        events = list(self.shared)
        for lane in self.lanes.values():
            events.extend(lane.events)
        return sorted(events, key=lambda e: e.at)

    def names(self, event: Event) -> set[str]:
        """The lanes' specs an event names, in its arguments or its --data."""
        lane_names = {name for _, name in self.lanes}
        named = {a for a in event.command if a in lane_names}
        data = event.data()
        named |= {data[k] for k in ("name", "spec") if isinstance(data.get(k), str) and data[k] in lane_names}
        return named

    def orchestrator_reaction(self, lane: Lane) -> Event | None:
        """The orchestrator's first command after a lane finished.

        That is an epic `status` poll, an `epic ...` command, or a shared
        command naming another lane's spec: a running child's commands naming
        its own spec are filed under its lane, so those left in the shared log
        are the orchestrator's. Commands naming the finished spec itself are
        skipped, since the child may still be reading its own plan to compose
        its hand-back.
        """
        if not lane.finish:
            return None
        polls = {id(e) for e in self.epic_polls()}
        for e in self.shared:
            if e.at < lane.finish.end:
                continue
            if id(e) in polls or e.is_verb("epic") or (self.names(e) - {lane.name}):
                return e
        return None

    def merge(self, spec: str) -> Event | None:
        """The successful `epic merge` of spec, or None."""
        for e in self.shared:
            if e.is_verb("epic", "merge") and e.ok and e.data().get("spec") == spec:
                return e
        return None

    def epic_polls(self) -> list[Event]:
        """The orchestrator's `status` calls for something other than a lane.

        A child may also run `status <its spec>`; those name a lane and are
        left out, which leaves the polls of the epic itself.
        """
        lane_names = {name for _, name in self.lanes}
        return [
            e
            for e in self.shared
            if e.is_verb("status") and len(e.command) > 1 and e.command[1] not in lane_names
        ]
