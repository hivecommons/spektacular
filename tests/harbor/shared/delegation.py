"""Judges a Claude run's child agents against what the workflows require.

The metrics recorder measures; this module judges. At the top is the oracle:
the hand-kept list of workflow steps whose instructions require a child
agent, those that only suggest one, and what a required child's context
must hold. Below it are small rules, each returning named Failures, so a
verifier asserts the list is empty and its failure message names the step,
spec or child at fault. Where a helper was started, and in which spec and
step, is judged from the `sub-agent` spans in Spektacular's own logs, read
through `spektacular stats` (runlog.py). What a helper was told, and how the
main agent's turns and wake-ups went, is read from the transcript; a
transcript launch is joined to its span by launch time.

The oracle is maintained by hand, never derived from the templates at run
time: an oracle that reads the instructions it checks would agree with
them by construction. A local self-check ties the two instead, failing when
a step template's delegation wording and the oracle disagree, so the rules
follow the instructions when that set changes.

These rules are enforced for Claude runs only, as before: the brief, turn
and wake-up rules need its transcript. Copied into each suite's tests/ by the
Makefile with the other shared helpers.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import runlog
import transcripts as tx
from transcripts import Child, Wakeup

# The oracle. (workflow kind, step) pairs, by hand from the step templates.

# Steps whose instructions require a child: doing the step in the agent's
# own context does not complete it.
REQUIRED = {("spec", "verification"), ("implement", "check_goal")}
# Steps whose instructions only suggest one, or require one only for a
# High-complexity task (test). Recorded, never enforced.
SUGGESTED = {("plan", "discovery"), ("implement", "analyze"), ("implement", "test"), ("implement", "update_changelog")}
# In an epic run, each spec lane of these kinds is worked by its own child.
PER_SPEC_CHILD = {"plan", "implement"}

# An orchestrated run is named by the start command's data, or in the words
# the epic skill gives the child ("This run is orchestrated: ...").
ORCHESTRATED = '"orchestrated":true'
ORCHESTRATED_WORDS = "run is orchestrated"
RESUME = "resume"
STAGED_SPEC = ".spektacular/tmp/spec_template.md"
# The template tells the reviewer to "review only what is written in that
# file"; an agent that puts it in its own words, such as "judge it exactly
# as written", gives the same instruction.
REVIEW_ONLY_WRITTEN = ("only what is written", "exactly as written")
PROJECT_ROOT = "/app"
# Every worktree of a spec lives under <root>/WORKTREE_DIR/<spec>, by hand
# from internal/worktree's Dir; a local self-check ties the two.
WORKTREE_DIR = ".spektacular/worktrees"
WORKFLOW_KINDS = ("spec", "plan", "implement")

# The trace oracle, by hand from templates/partials/trace-helpers.md and
# trace-marking.md (spec 000074): an agent marks each helper launch with a
# `trace start ... --type sub-agent` span and each question to the user with
# a `--type user-wait` one. A local self-check ties these to the partials.
TRACE_START = "trace start"
SUB_AGENT_TYPE = "--type sub-agent"
USER_WAIT_TYPE = "--type user-wait"
# A launch is marked when a sub-agent span started at most this long before
# the launching tool call (the agent starts the span, then writes the brief),
# or within a small clock slack after it.
LAUNCH_MARK_BEFORE_S = 120
LAUNCH_MARK_AFTER_S = 5
# The furthest a transcript launch may follow its sub-agent span's start.
LAUNCH_SPAN_WINDOW_S = LAUNCH_MARK_BEFORE_S


@dataclass
class Failure:
    rule: str  # which rule failed
    subject: str  # what it failed on: a step, a spec or a child
    detail: str

    def __str__(self) -> str:
        return f"{self.rule} {self.subject}: {self.detail}"


# Reading a run, for the verifiers.


def is_claude(build_info: Path = Path("/tests/build.json"), transcript: Path = tx.CLAUDE_TRANSCRIPT) -> bool:
    """A run these rules apply to: built for Claude, and a Claude transcript was left."""
    try:
        agent = json.loads(Path(build_info).read_text()).get("agent")
    except (OSError, ValueError, AttributeError):
        return False
    return agent == "claude" and Path(transcript).is_file() and Path(transcript).stat().st_size > 0


@dataclass
class Run:
    """What the rules read from one Claude run: the transcript's children,
    turns and wake-ups, and Spektacular's logs."""

    children: list[Child]
    log: runlog.RunLog
    spans: list[runlog.Span]  # every agent-marked span, in runs and outside them
    lanes: dict[tuple[str, str], runlog.Lane]
    turns: list[tx.Turn]
    wakeups: list[Wakeup]
    end: datetime | None  # the run's last timestamp
    unavailable: str | None = None  # why the logs could not be read


def read_run(transcript: Path, log_dir: Path) -> Run:
    """Reads the transcript, and Spektacular's logs through the run log
    reader. When the logs cannot be read, the rules that need them fail
    naming why, rather than passing on nothing."""
    objs = tx.read_jsonl(Path(transcript))
    children, _ = tx.claude_children(objs)
    times = [t for t in (tx.parse_timestamp(o.get("timestamp")) for o in objs) if t]
    log = runlog.RunLog(log_dir)
    spans, lanes, unavailable = [], {}, None
    try:
        spans, lanes = log.spans(), log.lanes(WORKFLOW_KINDS)
    except runlog.Unavailable as u:
        unavailable = u.reason
    return Run(
        children, log, spans, lanes, tx.claude_turns(objs), tx.claude_wakeups(objs), max(times) if times else None, unavailable
    )


def worktree_folder(spec: str, root: str = PROJECT_ROOT) -> str:
    """The folder every worktree of a spec lives under."""
    return f"{root}/{WORKTREE_DIR}/{spec}"


def lane_for(prompt: str, names: list[str]) -> str | None:
    """The lane whose spec name a launch prompt names first, the longest
    name winning when one contains another. A prompt names its own spec
    before any it depends on."""
    named = [n for n in names if n in prompt]
    return min(named, key=lambda n: (prompt.index(n), -len(n))) if named else None


def sub_agents(spans: list[runlog.Span]) -> list[runlog.Span]:
    return [s for s in spans if s.type == "sub-agent"]


def _unavailable(log: runlog.RunLog, lanes: dict) -> Failure | None:
    """A Failure naming why the logs cannot be read, or None when they can.
    With no lane at all, the reader is asked again, so an unreadable log is
    never mistaken for a run that started no workflow."""
    if lanes:
        return None
    try:
        log.runs()
    except runlog.Unavailable as u:
        return Failure("logs_unavailable", str(log.log_dir), u.reason)
    return None


# The rules.


def attempts(entries: list[tuple[str, dict]]) -> list[tuple[str, list[tuple[str, dict]]]]:
    """A lane's step entries, each with its run id, as attempts: runs of
    consecutive entries of one step. A resumed child re-enters the step it
    stopped at, which is still one attempt."""
    out: list[tuple[str, list[tuple[str, dict]]]] = []
    for run_id, e in entries:
        if out and out[-1][0] == e["step"]:
            out[-1][1].append((run_id, e))
        else:
            out.append((e["step"], [(run_id, e)]))
    return out


def _started_in(span: runlog.Span, run_id: str, entry: dict) -> bool:
    return (
        span.run_id == run_id
        and span.step == entry["step"]
        and runlog.parse_time(entry["started_at"]) <= span.start <= runlog.parse_time(entry["ended_at"])
    )


def missing_required(log: runlog.RunLog, lanes: dict[tuple[str, str], runlog.Lane], spans: list[runlog.Span]) -> list[Failure]:
    """Each required step a lane completed with no helper started in it, as
    the lane's `sub-agent` spans record, nested helpers included.

    A step is completed when the lane moved on to another step afterwards;
    the step a lane ended on may have been cut off, and is not judged.
    """
    gone = _unavailable(log, lanes)
    if gone:
        return [gone]
    helpers = sub_agents(spans)
    out = []
    for (kind, name), lane in lanes.items():
        try:
            entries = [(r["run_id"], e) for r in lane.runs for e in log.report(r["run_id"]).get("step_entries") or []]
        except runlog.Unavailable as u:
            out.append(Failure("logs_unavailable", f"{kind}:{name}", u.reason))
            continue
        tried = attempts(entries)
        for n, (step, attempt) in enumerate(tried[:-1]):
            if (kind, step) not in REQUIRED:
                continue
            if not any(_started_in(s, run_id, e) for s in helpers for run_id, e in attempt):
                count = sum(1 for st, _ in tried[:n] if st == step) + 1
                out.append(
                    Failure(
                        "missing_child",
                        f"{kind}:{name}:{step}" + (f" (attempt {count})" if count > 1 else ""),
                        f"the {step} step completed without a child agent started in it",
                    )
                )
    return out


def spec_children(children: list[Child], lane: tuple[str, str], names: list[str]) -> list[Child]:
    """The top-level children that worked one epic lane's spec, in launch
    order: those whose prompt names it first, or every one when the run
    worked that one spec alone."""
    top = [c for c in children if c.parent is None]
    if set(names) == {lane[1]}:
        return top
    return [c for c in top if lane_for(c.prompt or "", names) == lane[1]]


def missing_spec_children(log: runlog.RunLog, lanes: dict[tuple[str, str], runlog.Lane], spans: list[runlog.Span]) -> list[Failure]:
    """Each epic lane started under no helper span of the orchestrator's: no
    child of its own was launched for it. A lane's first command is matched
    to the orchestrator `sub-agent` span (outside any run) open when it ran,
    children taking spans in the order they started, so two children
    launched together each need a span of their own."""
    gone = _unavailable(log, lanes)
    if gone:
        return [gone]
    out = []
    for (kind, name), lane in lanes.items():
        if kind not in PER_SPEC_CHILD:
            continue
        try:
            span = log.child_span(lane.start) if lane.start else None
        except runlog.Unavailable as u:
            return [Failure("logs_unavailable", str(log.log_dir), u.reason)]
        if span is None:
            out.append(Failure("missing_spec_child", f"{kind}:{name}", f"no child of the orchestrator was started for spec {name}"))
    return out


def _squash(text: str) -> str:
    return re.sub(r"[\s\\]", "", text)


def epic_context_gaps(children: list[Child], lanes: dict[tuple[str, str], runlog.Lane], root: str = PROJECT_ROOT) -> list[Failure]:
    """Each epic child whose prompt lacks what the epic skill says to pass:
    its spec, the project root and that its run is orchestrated; for an
    implementing child, its spec's worktree folder; and for a child continuing
    a spec an earlier child worked, the instruction to resume it. Only for an
    epic run, whose orchestrator starts one child per spec."""
    names = [name for _, name in lanes]
    out = []
    for key in lanes:
        kind, name = key
        if kind not in PER_SPEC_CHILD:
            continue
        for n, c in enumerate(spec_children(children, key, names)):
            prompt = c.prompt or ""
            missing = []
            if name not in prompt:
                missing.append(f"the spec it works on ({name})")
            if root not in prompt:
                missing.append(f"the project root ({root})")
            if ORCHESTRATED not in _squash(prompt) and ORCHESTRATED_WORDS not in prompt.lower():
                missing.append(f"that its run is orchestrated ({ORCHESTRATED})")
            if kind == "implement" and worktree_folder(name, root) not in prompt:
                missing.append(f"its worktree folder {worktree_folder(name, root)}")
            if n > 0 and RESUME not in prompt.lower():
                missing.append("the instruction to resume the spec's earlier run")
            out += [_gap(c, name, what) for what in missing]
    return out


def review_context_gaps(children: list[Child], spans: list[runlog.Span]) -> list[Failure]:
    """Each spec review child (a launch joined by time to a helper span
    started in a spec run's verification step) not given the staged spec to
    read, or not told to review only what is written there."""
    marks = match_spans(children, spans)
    out = []
    for c in children:
        span = marks.get(c.id)
        if span is None or span.kind != "spec" or span.step != "verification":
            continue
        prompt = c.prompt or ""
        if STAGED_SPEC not in prompt:
            out.append(_gap(c, span.spec, f"the staged spec to read ({STAGED_SPEC})"))
        if not any(phrase in prompt.lower() for phrase in REVIEW_ONLY_WRITTEN):
            out.append(_gap(c, span.spec, "the instruction to review only what is written"))
    return out


def _gap(c: Child, spec: str | None, what: str) -> Failure:
    return Failure("context_gap", f"child {c.id} ({spec or '-'})", f"its prompt does not give {what}")


def running(c: Child, at: datetime, run_end: datetime | None) -> bool:
    start, end = c.launched_at, c.ended_at or run_end
    return start is not None and start <= at and (end is None or at < end)


def wakeups_while_children_run(wakeups: list[Wakeup], children: list[Child], run_end: datetime | None = None) -> list[Failure]:
    """Each timed wake-up, timer or sleep the main agent set while a child of
    its own was running. A child counts as running until its end time, or the
    run's end when that is unknown."""
    top = [c for c in children if c.parent is None]
    return [
        Failure("wakeup_while_children_run", f"{w.tool} {w.detail}", f"set at {w.at.isoformat()} while a child was running")
        for w in wakeups
        if w.at is not None and any(running(p, w.at, run_end) for p in top)
    ]


# How an epic orchestrator runs its per-spec children, settled by the
# background-child probe (tests/data/probe-background-child/OUTCOME): headless
# Claude Code stopped a background child about ten minutes after the main
# agent ended its turn, so the epic skills keep children in the foreground.
PROBE_OUTCOME = "stopped"


def turn_ended_while_running(c: Child, turns: list[tx.Turn]) -> tx.Turn | None:
    """The first turn that ended while the child was still running."""
    start, end = c.launched_at, c.ended_at
    return next((t for t in turns if t.ended_at and start and start <= t.ended_at and (end is None or t.ended_at < end)), None)


def epic_children_foreground_together(children: list[Child], turns: list[tx.Turn], lanes: list[tuple[str, str]]) -> list[Failure]:
    """The workaround for a session that stops background children: each
    spec's own child runs in the foreground, and the orchestrator handles it
    before its turn ends, so no turn ends while one is running."""
    names = [name for _, name in lanes]
    out = []
    for lane in lanes:
        if lane[0] not in PER_SPEC_CHILD:
            continue
        for c in spec_children(children, lane, names):
            if c.background:
                out.append(Failure("child_in_background", f"child {c.id} ({lane[1]})", "it was started in the background"))
            turn = turn_ended_while_running(c, turns)
            if turn is not None:
                out.append(
                    Failure("turn_ended_while_child_ran", f"child {c.id} ({lane[1]})", f"a turn ended at {turn.ended_at.isoformat()} while it was running")
                )
    return out


def epic_children_backgrounded(children: list[Child], turns: list[tx.Turn], lanes: list[tuple[str, str]]) -> list[Failure]:
    """A real session's way, for a session that keeps background children:
    each spec's own child runs in the background, a turn ends while one runs,
    and a child's completion notice starts a new turn."""
    names = [name for _, name in lanes]
    spec_of = {c.id: lane[1] for lane in lanes if lane[0] in PER_SPEC_CHILD for c in spec_children(children, lane, names)}
    children = [c for c in children if c.id in spec_of]
    out = [
        Failure("child_in_foreground", f"child {c.id} ({spec_of[c.id]})", "it was started in the foreground")
        for c in children
        if c.background is not True
    ]
    if children and not any(turn_ended_while_running(c, turns) for c in children):
        out.append(Failure("no_turn_ended_while_children_ran", "orchestrator", "no turn ended while a child was running"))
    if children and not any(t.woken_by == "task-notification" for t in turns):
        out.append(Failure("no_notice_woken_turn", "orchestrator", "no turn was started by a child's completion notice"))
    return out


def epic_children_run_as_settled(children: list[Child], turns: list[tx.Turn], lanes: list[tuple[str, str]]) -> list[Failure]:
    """The epic children rule the probe's recorded outcome calls for."""
    rule = epic_children_foreground_together if PROBE_OUTCOME == "stopped" else epic_children_backgrounded
    return rule(children, turns, lanes)


# Marked helper launches (spec 000074), and the join of a transcript launch to
# its span.


def _marking_span(c: Child, free: list[runlog.Span]) -> runlog.Span | None:
    at = c.launched_at
    if at is None:
        return None
    for s in free:
        lead = (at - s.start).total_seconds()
        if -LAUNCH_MARK_AFTER_S <= lead <= LAUNCH_SPAN_WINDOW_S:
            return s
    return None


def match_spans(children: list[Child], spans: list[runlog.Span]) -> dict[str, runlog.Span]:
    """Pairs each launch that has a time with one distinct sub-agent span
    started just before it, in launch order, by child id. A launch with no
    time is never paired."""
    free = sorted(sub_agents(spans), key=lambda s: s.start)
    out: dict[str, runlog.Span] = {}
    for c in sorted((c for c in children if c.launched_at), key=lambda c: c.launched_at):
        s = _marking_span(c, free)
        if s is not None:
            free.remove(s)
            out[c.id] = s
    return out


def unmarked(children: list[Child], spans: list[runlog.Span]) -> list[Child]:
    """The timed launches no sub-agent span marks, in launch order."""
    marks = match_spans(children, spans)
    return sorted((c for c in children if c.launched_at and c.id not in marks), key=lambda c: c.launched_at)


def unmarked_launches(children: list[Child], spans: list[runlog.Span]) -> list[Failure]:
    """Every helper launch with no sub-agent span marking it: the step's
    instruction tells the agent to run `trace start <helper> --type
    sub-agent` before each launch."""
    return [
        Failure(
            "unmarked_launch",
            f"child {c.id}",
            f"child {c.description or c.id} launched at {c.launched_at:%H:%M:%S} "
            f"has no sub-agent span (`{TRACE_START} <helper> {SUB_AGENT_TYPE}`)",
        )
        for c in unmarked(children, spans)
    ]


def marked_rate(children: list[Child], spans: list[runlog.Span]) -> float | None:
    """The share of timed helper launches marked by a sub-agent span, or
    None when there were none."""
    timed = [c for c in children if c.launched_at]
    return (len(timed) - len(unmarked(children, spans))) / len(timed) if timed else None
