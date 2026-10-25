"""Reads Spektacular's logs, only ever through `spektacular stats` (spec 000076).

Spektacular writes one JSON Lines file per workflow run under
`.spektacular/logs/`, and one day file of records written outside any run,
such as an epic orchestrator's. Harbor never parses those files itself: it
runs `spektacular stats list|run|commands|spans --dir <logs> --format json`
and reads the documented JSON, so every step, helper, wait and gap figure has
one definition, in Go, and a maintainer running `stats` on a collected
trial's logs sees the same numbers as the metrics record. The JSON field
names are pinned by the Go golden files under internal/runstats/testdata,
which this module's self-tests read as their canned answers.

Everything here reads records written at Spektacular's default log level:
argv, times, exit status and message, the run, the step a command ran in and
the step it moved to. Responses and state snapshots are never needed.

Any failure (no binary, no logs directory, a `stats` refusal, a timeout,
unreadable output) raises Unavailable with a reason naming the fix, so a
caller reports the measure as unknown, never as zero. Copied into each
suite's tests/ by the Makefile with the other shared helpers.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

TIMEOUT_S = 60
# Go writes nanoseconds; datetime holds microseconds.
_FRACTION = re.compile(r"^(?P<head>.*\.\d{6})\d*(?P<tail>.*)$")
_VALUE_FLAGS = ("--repo", "--from", "--data", "-d")


def parse_time(value: str) -> datetime:
    value = value.replace("Z", "+00:00")
    m = _FRACTION.match(value)
    return datetime.fromisoformat(m["head"] + m["tail"] if m else value)


class Unavailable(Exception):
    """The logs cannot answer; reason says why and what to do."""

    def __init__(self, reason: str):
        super().__init__(reason)
        self.reason = reason


@dataclass
class Command:
    """One `stats commands` row: a command Spektacular recorded."""

    run_id: str | None
    kind: str | None
    name: str | None
    command: str
    args: list[str]
    step: str | None
    step_to: str | None
    at: datetime
    end: datetime
    exit_code: int
    message: str | None = None
    span_id: str = ""

    @classmethod
    def of(cls, row: dict) -> Command:
        return cls(
            run_id=row.get("run_id"), kind=row.get("kind"), name=row.get("name"),
            command=row.get("command") or "", args=list(row.get("args") or []),
            step=row.get("step"), step_to=row.get("step_to"),
            at=parse_time(row["started_at"]), end=parse_time(row["ended_at"]),
            exit_code=int(row.get("exit_code") or 0), message=row.get("message"), span_id=row.get("span_id") or "",
        )

    @property
    def ok(self) -> bool:
        return self.exit_code == 0

    def data(self) -> dict:
        """The command's --data JSON, or {} when it has none."""
        for i, arg in enumerate(self.args):
            raw = None
            if arg in ("--data", "-d") and i + 1 < len(self.args):
                raw = self.args[i + 1]
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
        return self.args[: len(verb)] == list(verb)

    @property
    def goto_step(self) -> str | None:
        """The step a `<kind> goto` asked for."""
        if len(self.args) >= 2 and self.args[1] == "goto":
            return self.data().get("step")
        return None

    @property
    def entered(self) -> str | None:
        """The step this command moved its run to, or the one it asked for."""
        return self.step_to or self.goto_step


@dataclass
class Span:
    """One `stats spans` row: a helper agent, a wait on the user or other work."""

    span_id: str
    parent: str | None
    type: str
    name: str
    run_id: str | None
    kind: str | None
    spec: str | None  # the workflow's name
    step: str | None
    task_id: str | None
    start: datetime
    end: datetime
    end_reason: str
    complete: bool
    status: str | None = None
    trace_id: str = ""

    @classmethod
    def of(cls, row: dict) -> Span:
        return cls(
            span_id=row.get("span_id") or "", parent=row.get("parent_span_id"), type=row.get("type") or "",
            name=row.get("name") or "", run_id=row.get("run_id"), kind=row.get("kind"), spec=row.get("workflow"),
            step=row.get("step"), task_id=row.get("task_id"),
            start=parse_time(row["started_at"]), end=parse_time(row["ended_at"]),
            end_reason=row.get("end_reason") or "", complete=bool(row.get("complete")),
            status=row.get("status"), trace_id=row.get("trace_id") or "",
        )

    def open_at(self, at: datetime) -> bool:
        """Open at a moment: started by then, and not yet stopped. A span
        never stopped counts as open from its start on."""
        return self.start <= at and (not self.complete or at <= self.end)


@dataclass
class Lane:
    """Every run of one workflow (kind, name), oldest first."""

    kind: str
    name: str
    runs: list[dict] = field(default_factory=list)  # `stats list` summaries
    commands: list[Command] = field(default_factory=list)
    start: Command | None = None  # the first successful `<kind> new`
    finish: Command | None = None  # the first successful move to `finished`
    last: Command | None = None  # the lane's last recorded command
    resumes: list[Command] = field(default_factory=list)  # each failed `<kind> new` after the start
    child_starts: list[Command] = field(default_factory=list)  # each child's first command
    stops: list[str] = field(default_factory=list)  # where each earlier child stopped

    @property
    def children(self) -> int:
        """How many children worked the lane: each start, restart and resume."""
        return len(self.child_starts)


def _positional(args: list[str]) -> str | None:
    """The first argument that is neither a flag nor a flag's value."""
    skip = False
    for a in args:
        if skip:
            skip = False
            continue
        if a in _VALUE_FLAGS:
            skip = True
            continue
        if a.startswith("-"):
            continue
        return a
    return None


def event_at(event: Command | Span, after: datetime) -> datetime:
    """When an orchestrator reaction happened: a command's start, or a span's
    start or end, whichever came after `after`."""
    if isinstance(event, Command):
        return event.at
    return event.start if event.start >= after else event.end


class RunLog:
    """Harbor's only route to Spektacular's logs. Each `stats` question is
    asked once per reader, so a verifier module builds one and reuses it."""

    def __init__(self, log_dir: Path, binary: str | None = None, runner=subprocess.run):
        self.log_dir = Path(log_dir)
        self.binary = binary or os.environ.get("SPEKTACULAR_BIN") or shutil.which("spektacular")
        self.runner = runner
        self._cache: dict[tuple, object] = {}

    def _stats(self, sub: str, *flags: str) -> dict:
        key = (sub, *flags)
        if key in self._cache:
            return self._cache[key]
        if not self.binary:
            raise Unavailable("no spektacular binary on PATH; set SPEKTACULAR_BIN to one that has `stats`")
        if not self.log_dir.is_dir():
            raise Unavailable(f"no Spektacular logs at {self.log_dir}")
        argv = [self.binary, "stats", sub, *flags, "--dir", str(self.log_dir), "--format", "json"]
        try:
            # From a folder outside any project, so no project's gate applies.
            done = self.runner(argv, capture_output=True, text=True, timeout=TIMEOUT_S, cwd=tempfile.gettempdir())
            out = json.loads(done.stdout)
        except subprocess.TimeoutExpired:
            raise Unavailable(f"`spektacular stats {sub}` took longer than {TIMEOUT_S}s") from None
        except OSError as exc:
            raise Unavailable(f"cannot run {self.binary}: {exc}; set SPEKTACULAR_BIN to a binary that has `stats`") from None
        except (ValueError, TypeError):
            raise Unavailable(
                f"`spektacular stats {sub}` gave no JSON; use a spektacular build that has `stats {sub}`"
            ) from None
        if not isinstance(out, dict):
            raise Unavailable(f"`spektacular stats {sub}` gave unexpected JSON")
        if out.get("error"):
            raise Unavailable(f"stats {sub}: {out.get('code')}: {out.get('message')}")
        self._cache[key] = out
        return out

    def runs(self) -> list[dict]:
        """Every workflow run's summary, oldest first."""
        return sorted(self._stats("list").get("runs") or [], key=lambda r: (r["started_at"], r["run_id"]))

    def report(self, run_id: str) -> dict:
        return self._stats("run", "--run", run_id)

    def commands(self) -> list[Command]:
        if "commands" not in self._cache:
            rows = [Command.of(r) for r in self._stats("commands").get("commands") or []]
            self._cache["commands"] = sorted(rows, key=lambda c: c.at)
        return self._cache["commands"]

    def spans(self) -> list[Span]:
        if "spans" not in self._cache:
            rows = [Span.of(r) for r in self._stats("spans").get("spans") or []]
            self._cache["spans"] = sorted(rows, key=lambda s: s.start)
        return self._cache["spans"]

    def lanes(self, kinds: tuple[str, ...] = ("plan", "implement")) -> dict[tuple[str, str], Lane]:
        lanes: dict[tuple[str, str], Lane] = {}
        for r in self.runs():
            if r["kind"] in kinds:
                lanes.setdefault((r["kind"], r["name"]), Lane(r["kind"], r["name"])).runs.append(r)
        for c in self.commands():
            if c.kind in kinds and c.name:
                lanes.setdefault((c.kind, c.name), Lane(c.kind, c.name)).commands.append(c)
        for lane in lanes.values():
            _settle(lane)
        return lanes

    def lane(self, kind: str, name: str) -> Lane | None:
        """One workflow's lane, or None when the logs hold none."""
        return self.lanes((kind,)).get((kind, name))

    @property
    def empty(self) -> bool:
        """True when the logs hold no run and no command, or cannot be read."""
        try:
            return not self.runs() and not self.commands()
        except Unavailable:
            return True

    def outside(self) -> list[Command]:
        """The commands recorded outside any workflow run, oldest first."""
        return [c for c in self.commands() if c.run_id is None]

    def orchestrator_spans(self) -> list[Span]:
        """The `sub-agent` spans started outside any run, oldest first."""
        return [s for s in self.spans() if s.type == "sub-agent" and s.run_id is None]

    def _child_spans(self) -> dict[int, Span]:
        if "child_spans" not in self._cache:
            starts = sorted(
                (c for lane in self.lanes(("spec", "plan", "implement")).values() for c in lane.child_starts),
                key=lambda c: c.at,
            )
            free = list(self.orchestrator_spans())
            matched: dict[tuple, Span] = {}
            for c in starts:
                s = next((s for s in free if s.open_at(c.at)), None)
                if s is not None:
                    free.remove(s)
                    matched[(c.run_id, c.span_id, c.at)] = s
            self._cache["child_spans"] = matched
        return self._cache["child_spans"]

    def child_span(self, first: Command) -> Span | None:
        """The orchestrator span a child was launched under: the earliest one,
        not matched to an earlier child, open when the child's first command
        ran. Children are matched in the order of their first commands."""
        return self._child_spans().get((first.run_id, first.span_id, first.at))

    def names(self, cmd: Command) -> set[str]:
        """The lanes' specs a command names, in its arguments or its --data."""
        lane_names = {name for _, name in self.lanes()}
        named = {a for a in cmd.args if a in lane_names}
        data = cmd.data()
        named |= {data[k] for k in ("name", "spec") if isinstance(data.get(k), str) and data[k] in lane_names}
        return named

    def orchestrator_reaction(self, lane: Lane) -> Command | Span | None:
        """The orchestrator's first recorded act after a lane's last command:
        an outside-run `epic` or `trace` command, or one naming another spec,
        or an orchestrator span starting or stopping. Commands naming only
        the lane's own spec are skipped: the child may still be reading its
        own plan to compose its hand-back."""
        if lane.last is None:
            return None
        after = lane.last.end
        best: tuple[datetime, Command | Span] | None = None
        for c in self.outside():
            if c.at >= after and (c.is_verb("epic") or c.is_verb("trace") or (self.names(c) - {lane.name})):
                best = (c.at, c)
                break
        for s in self.orchestrator_spans():
            edges = [s.start] + ([s.end] if s.complete else [])
            at = next((t for t in edges if t > after), None)
            if at is not None and (best is None or at < best[0]):
                best = (at, s)
        return best[1] if best else None

    def merge(self, spec: str) -> Command | None:
        """The successful `epic merge` of spec, or None."""
        return next((c for c in self.outside() if c.is_verb("epic", "merge") and c.ok and c.data().get("spec") == spec), None)

    def steps_entered(self, kind: str) -> list[str]:
        """The step each successful `<kind> new` or `<kind> goto` left its
        workflow at, in the order they ran."""
        return [
            c.entered
            for c in self.commands()
            if c.ok and (c.is_verb(kind, "new") or c.is_verb(kind, "goto")) and c.entered
        ]

    def changelog_writes_by_repo(self, name: str | None = None) -> dict[str | None, int]:
        """How many successful `changelog file write <name>` calls ran for
        each repo: the value of `--repo` (or `--repo=`), or None for the
        project's own record. A write of another name, or one that only asked
        for help, is not counted."""
        counts: dict[str | None, int] = {}
        for c in self.commands():
            if not (c.ok and c.is_verb("changelog", "file", "write")):
                continue
            args = c.args[3:]
            if "--help" in args or "-h" in args:
                continue
            target = _positional(args)
            if target is None or (name is not None and target != name):
                continue
            repo = None
            for i, a in enumerate(args):
                if a == "--repo" and i + 1 < len(args):
                    repo = args[i + 1]
                elif a.startswith("--repo="):
                    repo = a.removeprefix("--repo=")
            counts[repo] = counts.get(repo, 0) + 1
        return counts


def _settle(lane: Lane) -> None:
    """Works out a lane's start, finish, last command, resumes and children.

    A child that resumes a lane already in progress runs `<kind> new`, which
    the CLI refuses; the refusal is filed in the lane's own run, carrying the
    step the run stood at. A restart (`new --force`) starts a new run of the
    lane with a successful `new`."""
    lane.commands.sort(key=lambda c: c.at)
    lane.runs.sort(key=lambda r: r["started_at"])
    news = [c for c in lane.commands if c.is_verb(lane.kind, "new")]
    starts = [c for c in news if c.ok]
    lane.start = starts[0] if starts else None
    lane.finish = next((c for c in lane.commands if c.ok and c.entered == "finished"), None)
    lane.last = lane.commands[-1] if lane.commands else None
    if lane.start is None:
        return
    later = [c for c in news if c.at > lane.start.at]
    lane.resumes = [c for c in later if not c.ok]
    lane.child_starts = [lane.start] + later
    lane.stops = [(c.step or "?") if not c.ok else "restart" for c in later]
