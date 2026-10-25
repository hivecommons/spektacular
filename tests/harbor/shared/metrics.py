#!/usr/bin/env python3
"""Writes a harbor run's metrics record beside its reward.

Every suite's test.sh runs this once pytest has finished and the reward is
written, and ignores how it exits. It reads:

- the build info the Makefile render target left in /tests/build.json: the
  agent, suite, scenario, Spektacular version and commit;
- the agent's own log under /logs/agent: Claude's stream-json, the ATIF
  trajectory harbor writes for Bob and Codex, or Codex's rollout;
- Spektacular's session log under /app/.spektacular/logs, for the steps
  and for placing each child agent in the spec and step it was started in;
- pytest's JUnit XML and the reward, for the check score;

and writes /logs/verifier/metrics.json, in the same shape for every agent
and suite. A measure a source does not supply is null and is listed under
`unknown` with the reason, never written as zero.

It never fails the run: each section is gathered on its own, a failure costs
only that section's fields, and it always exits 0. It never writes the
reward. Run it inside the container, or locally against a collected trial:

    python3 metrics.py --agent-dir <trial>/agent \\
        --debug-dir <trial>/artifacts/spektacular/logs \\
        --junit <trial>/verifier/junit.xml --reward <trial>/verifier/reward.txt \\
        --out /tmp/metrics.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
import xml.etree.ElementTree as ET
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))

import spans  # noqa: E402
import transcripts as tx  # noqa: E402
from sessionlog import Lane, SessionLog, StepWindow  # noqa: E402
from transcripts import Child, Message, Subagents, SubagentLaunch, Totals, Unknown  # noqa: E402

SCHEMA_VERSION = 2
WORKFLOW_KINDS = ("spec", "plan", "implement")

BUILD_INFO = Path("/tests/build.json")
AGENT_DIR = Path("/logs/agent")
DEBUG_DIR = Path("/app/.spektacular/logs")
BOB_LOGS = Path("/root/.bob/logs")
JUNIT = Path("/logs/verifier/junit.xml")
REWARD = Path("/logs/verifier/reward.txt")
OUT = Path("/logs/verifier/metrics.json")

RUN_FIELDS = ("suite", "agent", "spektacular_version", "commit", "dirty")
SUBAGENT_FIELDS = ("started", "completed", "failed", "stopped", "durations_s")
CHILD_FIELDS = (
    "id", "parent", "kind", "description", "background", "spec", "step", "launched_at", "ended_at",
    "duration_s", "ending", "hand_back", "models", "output", "prompt", "prompt_chars",
)


def utc(at: datetime) -> datetime:
    return at.replace(tzinfo=timezone.utc) if at.tzinfo is None else at


def iso(at: datetime) -> str:
    return utc(at).astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


# Run identity.


def read_build_info(path: Path) -> tuple[dict, list[Unknown]]:
    """The run's identity from build.json, with an Unknown per missing field."""
    info: dict = {}
    reason = None
    try:
        loaded = json.loads(Path(path).read_text())
        if isinstance(loaded, dict):
            info = loaded
        else:
            reason = "build.json is not an object"
    except FileNotFoundError:
        reason = "no build.json (suite not run through its make target)"
    except (OSError, ValueError) as exc:
        reason = f"unreadable build.json: {exc}"
    run = {
        "suite": info.get("suite"),
        "scenario": info.get("scenario") or None,
        "agent": info.get("agent") or None,
        "spektacular_version": info.get("spektacular_version"),
        "commit": info.get("commit"),
        "dirty": info.get("dirty") if isinstance(info.get("dirty"), bool) else None,
    }
    unknowns = [Unknown(f"run.{name}", reason or "missing from build.json") for name in RUN_FIELDS if run[name] is None]
    return run, unknowns


# The agent's own log.


@dataclass
class AgentLog:
    """What one agent log says about the run, whichever format it was."""

    agent: str | None
    totals: Totals
    subagents: Subagents
    messages: list[Message] | None  # None when the format's messages are not read
    launch_root: dict[str, str] = field(default_factory=dict)
    launches: list[SubagentLaunch] = field(default_factory=list)
    # Reads the child agents when called, so a reader failure costs only them.
    read_children: Callable[[], tuple[list[Child] | None, list[Unknown]]] = lambda: (None, [])


def no_children(reason: str) -> Callable[[], tuple[None, list[Unknown]]]:
    return lambda: (None, [Unknown("subagents.children", reason)])


def missing_log(reason: str) -> AgentLog:
    totals = Totals()
    totals.unknowns = [Unknown(f"totals.{name}", reason) for name in tx.TOTAL_FIELDS]
    totals.unknowns.append(Unknown("run.model", reason))
    subagents = Subagents(unknowns=[Unknown(f"subagents.{name}", reason) for name in SUBAGENT_FIELDS])
    return AgentLog(agent=None, totals=totals, subagents=subagents, messages=None, read_children=no_children(reason))


def has_rollout(agent_dir: Path) -> bool:
    sessions = agent_dir / "sessions"
    return sessions.is_dir() and any(sessions.rglob("rollout-*.jsonl"))


def pick_reader(agent_dir: Path) -> str | None:
    """The log format the agent left: Claude's stream-json, an ATIF
    trajectory, or a Codex rollout, in that order of preference."""
    agent_dir = Path(agent_dir)
    if (agent_dir / "claude-code.txt").is_file():
        return "claude"
    if (agent_dir / "trajectory.json").is_file():
        return "atif"
    if has_rollout(agent_dir):
        return "codex-rollout"
    return None


def read_agent_log(agent_dir: Path, build_agent: str | None) -> tuple[AgentLog, list[Unknown]]:
    agent_dir = Path(agent_dir)
    reader = pick_reader(agent_dir)
    log = None
    if reader == "claude":
        objs = tx.read_jsonl(agent_dir / "claude-code.txt")
        log = AgentLog(
            agent="claude",
            totals=tx.claude_summary(objs),
            subagents=tx.claude_subagents(objs),
            messages=tx.claude_messages(objs),
            launch_root=tx.claude_launch_root(objs),
            launches=tx.claude_subagent_launches(objs),
            read_children=lambda: tx.claude_children(objs),
        )
    elif reader == "atif":
        try:
            traj = json.loads((agent_dir / "trajectory.json").read_text())
        except (OSError, ValueError) as exc:
            traj = None
            if not has_rollout(agent_dir):
                log = missing_log(f"unreadable trajectory.json: {exc}")
        if isinstance(traj, dict):
            log = AgentLog(
                agent=tx.atif_agent_name(traj),
                totals=tx.atif_summary(traj),
                subagents=tx.atif_subagents(traj),
                messages=tx.atif_messages(traj),
                read_children=lambda: tx.atif_children(traj),
            )
        elif log is None:
            reader = "codex-rollout"
    if reader == "codex-rollout":
        log = AgentLog(
            agent="codex",
            totals=tx.codex_rollout_summary(agent_dir / "sessions"),
            subagents=Subagents(unknowns=[Unknown(f"subagents.{n}", "codex reports no sub-agents") for n in SUBAGENT_FIELDS]),
            messages=None,
            read_children=no_children("codex reports no sub-agents"),
        )
    if log is None:
        log = missing_log("no agent log (oracle or missing)")

    unknowns = []
    if build_agent and log.agent and log.agent != build_agent:
        unknowns.append(Unknown("run.agent", f"the agent log is {log.agent}'s but build.json names {build_agent}"))
    return log, unknowns


# Check score.


def score_checks(junit_text: str) -> tuple[dict, list[dict]]:
    """Passed and total checks overall and per group, from pytest's JUnit
    XML. A group is the test class, or the module for a suite written as
    plain functions. Skipped tests count in neither."""
    root = ET.fromstring(junit_text)
    groups: dict[str, list[int]] = {}
    for case in root.iter("testcase"):
        if case.find("skipped") is not None:
            continue
        classname = case.get("classname") or ""
        last = classname.rsplit(".", 1)[-1]
        name = last if last.startswith("Test") else classname
        passed = case.find("failure") is None and case.find("error") is None
        tally = groups.setdefault(name, [0, 0])
        tally[0] += int(passed)
        tally[1] += 1

    def score(passed: int, total: int) -> dict:
        return {"passed": passed, "total": total, "score": passed / total if total else None}

    checks = score(sum(t[0] for t in groups.values()), sum(t[1] for t in groups.values()))
    return checks, [{"name": name, **score(*tally)} for name, tally in groups.items()]


def read_checks(path: Path) -> tuple[tuple[dict, list[dict] | None], list[Unknown]]:
    empty = {"passed": None, "total": None, "score": None}
    try:
        text = Path(path).read_text()
    except FileNotFoundError:
        reason = "no junit.xml from pytest"
    except OSError as exc:
        reason = f"unreadable junit.xml: {exc}"
    else:
        try:
            checks, groups = score_checks(text)
        except ET.ParseError as exc:
            reason = f"malformed junit.xml: {exc}"
        else:
            unknowns = [] if checks["total"] else [Unknown("result.checks.score", "pytest ran no checks")]
            return (checks, groups), unknowns
    return (empty, None), [Unknown("result.checks", reason), Unknown("result.groups", reason)]


def read_reward(path: Path) -> tuple[int | None, list[Unknown]]:
    """The reward the verifier already wrote. Read only, never written."""
    try:
        text = Path(path).read_text().strip()
    except OSError as exc:
        return None, [Unknown("result.reward", f"no reward.txt: {exc.strerror or exc}")]
    try:
        return int(float(text)), []
    except ValueError:
        return None, [Unknown("result.reward", f"reward.txt holds {text!r}")]


# Step breakdown.


def window_of(windows: list[StepWindow], at: datetime) -> int | None:
    at = utc(at)
    for i, w in enumerate(windows):
        if utc(w.start) <= at < utc(w.end):
            return i
    return None


def lane_for(prompt: str, names: list[str]) -> str | None:
    """The lane whose spec name a sub-agent's launch prompt names first, the
    longest name winning when one contains another. A prompt names its own
    spec before any it depends on."""
    named = [n for n in names if n in prompt]
    return min(named, key=lambda n: (prompt.index(n), -len(n))) if named else None


def step_breakdown(
    lanes: list[Lane],
    messages: list[Message] | None,
    root_of: dict[str, str],
    launches: list[SubagentLaunch],
    agent: str | None,
) -> tuple[dict, list[Unknown]]:
    """Each lane's steps with their durations and the characters written in
    them. Durations come from the session log alone; characters from the
    agent log, joined by time, or by the agent's own gotos when its messages
    carry no time."""
    windows = {(lane.kind, lane.name): lane.step_windows() for lane in lanes}
    outputs: dict[tuple[str, str], list[int | None]] = {
        key: [0] * len(ws) for key, ws in windows.items()
    }
    unknowns: list[Unknown] = []
    extra: dict = {}
    who = agent or "the agent"

    if messages is None:
        outputs = {key: [None] * len(ws) for key, ws in windows.items()}
        if windows:
            unknowns.append(Unknown("steps.output", f"no messages read from {who}'s log"))
    elif len(windows) == 1:
        (key, ws), = windows.items()
        if messages and all(m.at is not None for m in messages):
            for m in messages:
                i = window_of(ws, m.at)
                if i is not None:
                    outputs[key][i] += m.chars
        else:
            i = 0
            for m in sorted(messages, key=lambda m: m.order):
                if ws:
                    outputs[key][i] += m.chars
                if m.goto_step:
                    ahead = [j for j in range(i + 1, len(ws)) if ws[j].step == m.goto_step]
                    if ahead:
                        i = ahead[0]
    elif len(windows) > 1 and agent == "claude":
        prompts = {l.tool_use_id: l.prompt for l in launches}
        names = [name for _, name in windows]
        by_name = {key[1]: key for key in windows}
        orchestrator = 0
        for m in messages:
            lane = None
            if m.parent:
                lane = lane_for(prompts.get(root_of.get(m.parent, m.parent), ""), names)
            i = window_of(windows[by_name[lane]], m.at) if lane and m.at else None
            if i is None:
                orchestrator += m.chars
            else:
                outputs[by_name[lane]][i] += m.chars
        extra["orchestrator_output"] = orchestrator
    elif len(windows) > 1:
        outputs = {key: [None] * len(ws) for key, ws in windows.items()}
        unknowns.append(Unknown("steps.output", f"{who} output cannot be split between concurrent lanes"))

    workflows = [
        {
            "kind": kind,
            "name": name,
            "steps": [
                {"step": w.step, "started_at": iso(w.start), "duration_s": round(w.duration_s, 3), "output": out}
                for w, out in zip(ws, outputs[(kind, name)])
            ],
        }
        for (kind, name), ws in windows.items()
    ]
    return {"unit": "characters", "workflows": workflows, **extra}, unknowns


def read_session(debug_dir: Path) -> tuple[SessionLog | None, list[Unknown]]:
    session = SessionLog(debug_dir, kinds=WORKFLOW_KINDS)
    if session.empty:
        reason = "no session log" if not Path(debug_dir).is_dir() else "the session log is empty"
        return None, [Unknown("steps", reason)]
    return session, []


def session_lanes(session: SessionLog) -> list[Lane]:
    return sorted((l for l in session.lanes.values() if l.events), key=lambda l: l.events[0].at)


def lane_windows(session: SessionLog | None) -> dict[tuple[str, str], list[StepWindow]] | None:
    """Each lane's step windows, keyed by (kind, name); None without a session log."""
    if session is None:
        return None
    return {(lane.kind, lane.name): lane.step_windows() for lane in session_lanes(session)}


def read_trace_calls(session: SessionLog | None, debug_dir: Path) -> tuple[dict, list[Unknown]]:
    """The `spektacular trace` calls the run made (spec 000074), from the
    session log, and their average per plan task: the run's task spans. The
    session log records every call, chained or not, whichever agent made it.
    With no session log both are None, as read_session already said why."""
    if session is None:
        return {"trace_calls": None, "trace_calls_per_task": None}, []
    calls = sum(1 for e in session.all_events() if e.command[:1] == ["trace"])
    tasks = len(spans.of_type(spans.read_spans(debug_dir), spans.TASK))
    return {"trace_calls": calls, "trace_calls_per_task": round(calls / tasks, 2) if tasks else None}, []


def read_steps(session: SessionLog | None, log: AgentLog) -> tuple[dict, list[Unknown]]:
    if session is None:  # read_session already said why
        return {"unit": "characters", "workflows": None}, []
    return step_breakdown(session_lanes(session), log.messages, log.launch_root, log.launches, log.agent)


# Child agents: placed in a spec and a step, and counted per step.


@dataclass
class PlacedChild:
    """A child agent with the lane, step window, spec and step it was started in."""

    child: Child
    lane: tuple[str, str] | None
    window: int | None  # index into the lane's step windows
    spec: str | None
    step: str | None  # None: started outside any workflow step, unless listed unknown


def _gap(field_name: str, ids: list[str], every: int, reason: str) -> list[Unknown]:
    """One agent-wide Unknown when a gap holds for every child, else one per child."""
    if not ids:
        return []
    if len(ids) == every:
        return [Unknown(f"subagents.children.{field_name}", reason)]
    return [Unknown(f"subagents.children[{i}].{field_name}", reason) for i in ids]


def place_children(
    children: list[Child],
    windows_by_lane: dict[tuple[str, str], list[StepWindow]] | None,
    messages: list[Message] | None = None,
) -> tuple[list[PlacedChild], list[Unknown]]:
    """Places each child in the spec and workflow step it was started in.

    Its spec is the single lane's in a one-lane run; otherwise the spec its own
    prompt names first, else its top-level ancestor's. Its step is the window
    of that lane holding its launch time. A child with no launch time (Bob) is
    placed by the order of the agent's messages and gotos, as the step
    breakdown places their output, which only a one-lane run allows.
    """
    if windows_by_lane is None:
        reason = "no session log to place children in"
        unknowns = [Unknown("subagents.children.spec", reason), Unknown("subagents.children.step", reason)] if children else []
        return [PlacedChild(c, None, None, None, None) for c in children], unknowns

    by_id = {c.id: c for c in children}
    names = list(dict.fromkeys(name for _, name in windows_by_lane))

    def ancestor(c: Child) -> Child:
        seen = {c.id}
        while c.parent in by_id and c.parent not in seen:
            c = by_id[c.parent]
            seen.add(c.id)
        return c

    def spec_of(c: Child) -> str | None:
        if len(names) == 1:
            return names[0]
        return lane_for(c.prompt or "", names) or (lane_for(ancestor(c).prompt or "", names) if c.parent else None)

    # For a log without times: the window the agent stood in at each message.
    window_at_order: dict[int, int] = {}
    if len(windows_by_lane) == 1 and messages:
        (ws,) = windows_by_lane.values()
        i = 0
        for m in sorted(messages, key=lambda m: m.order):
            window_at_order[m.order] = i
            if m.goto_step:
                ahead = [j for j in range(i + 1, len(ws)) if ws[j].step == m.goto_step]
                if ahead:
                    i = ahead[0]

    placed, unplaced = [], []
    for c in children:
        spec = spec_of(c)
        keys = [k for k in windows_by_lane if k[1] == spec]
        window = None
        if c.launched_at is not None:
            key = next((k for k in keys if window_of(windows_by_lane[k], c.launched_at) is not None), keys[0] if keys else None)
            window = window_of(windows_by_lane[key], c.launched_at) if key else None
        else:
            key = keys[0] if keys else None
            if key and window_at_order and c.order in window_at_order and windows_by_lane[key]:
                window = window_at_order[c.order]
            elif any(windows_by_lane.values()):
                # Without a time it may have been started in any lane's step.
                unplaced.append(c.id)
        step = windows_by_lane[key][window].step if key and window is not None else None
        placed.append(PlacedChild(c, key, window, spec, step))
    return placed, _gap("step", unplaced, len(children), "the child has no launch time to place it in a step")


def step_child_counts(
    placed: list[PlacedChild], windows_by_lane: dict[tuple[str, str], list[StepWindow]]
) -> dict[tuple[tuple[str, str], int], int]:
    """How many children, at any depth, were started in each lane's step window."""
    counts = {(key, i): 0 for key, ws in windows_by_lane.items() for i in range(len(ws))}
    for p in placed:
        if p.lane is not None and p.window is not None:
            counts[(p.lane, p.window)] += 1
    return counts


def child_record(p: PlacedChild) -> dict:
    c = p.child
    return {
        "id": c.id,
        "parent": c.parent,
        "kind": c.kind,
        "description": c.description,
        "background": c.background,
        "spec": p.spec,
        "step": p.step,
        "launched_at": iso(c.launched_at) if c.launched_at else None,
        "ended_at": iso(c.ended_at) if c.ended_at else None,
        "duration_s": round(c.duration_s, 3) if c.duration_s is not None else None,
        "ending": c.ending,
        "hand_back": c.hand_back,
        "models": sorted(c.models) if c.models is not None else None,
        "output": c.output,
        "prompt": c.prompt,
        "prompt_chars": len(c.prompt) if c.prompt is not None else None,
    }


def read_children(log: AgentLog, session: SessionLog | None) -> tuple[tuple[list[dict] | None, list[PlacedChild] | None], list[Unknown]]:
    """The record's child list, placed in specs and steps, and the placement itself."""
    children, unknowns = log.read_children()
    if children is None:
        return (None, None), unknowns
    placed, gaps = place_children(children, lane_windows(session), log.messages)
    unknowns = unknowns + gaps
    # A prompt the log could not show leaves its size unknown too.
    unknowns += [Unknown(u.field.removesuffix(".prompt") + ".prompt_chars", u.reason) for u in unknowns if u.field.endswith(".prompt")]
    return ([child_record(p) for p in placed], placed), unknowns


def count_step_children(
    steps: dict, session: SessionLog | None, placed: list[PlacedChild] | None, unknowns: list[Unknown]
) -> tuple[None, list[Unknown]]:
    """Writes each step entry's `children`: how many children it started, or
    None when the children, or where they were started, are unknown."""
    workflows = steps.get("workflows")
    if not workflows:
        return None, []
    windows = lane_windows(session) or {}
    unplaced = any(u.field in ("subagents.children", "subagents.children.step") or (u.field.startswith("subagents.children[") and u.field.endswith("].step")) for u in unknowns)
    counts = step_child_counts(placed, windows) if placed is not None and not unplaced else None
    for wf in workflows:
        for i, entry in enumerate(wf["steps"]):
            entry["children"] = counts.get(((wf["kind"], wf["name"]), i)) if counts is not None else None
    if counts is None:
        return None, [Unknown("steps.children", "the run's children, or the steps they were started in, are unknown")]
    return None, []


# Idle time: the longest stretch with no activity.


def idle_seconds(instants: list[datetime]) -> float | None:
    """The longest gap between consecutive instants; None with fewer than two."""
    times = sorted(utc(t) for t in instants)
    if len(times) < 2:
        return None
    return max((b - a).total_seconds() for a, b in zip(times, times[1:]))


def read_idle(
    log: AgentLog, placed: list[PlacedChild] | None, session: SessionLog | None, bob_logs: Path
) -> tuple[float | None, list[Unknown]]:
    """Idle time from every timestamped message, child start and end, and
    Spektacular command start and end. The session log alone would count the
    agent's time thinking between commands as idle, so a run whose agent log
    carries no times has its idle time unknown, unless Bob's own shell logs
    supply them."""
    instants = [m.at for m in log.messages or [] if m.at is not None]
    if not instants:
        instants = bob_log_times(bob_logs)
    if not instants:
        return None, [Unknown("totals.idle_s", f"{log.agent or 'the agent'}'s log carries no activity times")]
    for p in placed or []:
        instants += [t for t in (p.child.launched_at, p.child.ended_at) if t is not None]
    for e in session.all_events() if session is not None else []:
        instants += [e.at, e.end]
    idle = idle_seconds(instants)
    if idle is None:
        return None, [Unknown("totals.idle_s", "too little activity to measure")]
    return round(idle, 3), []


# Wall-time fallback for agents whose log carries no time.


def bob_log_span_s(log_dir: Path) -> float | None:
    """First to last `ts` across Bob's own shell logs."""
    times = bob_log_times(log_dir)
    return (max(times) - min(times)).total_seconds() if len(times) > 1 else None


def bob_log_times(log_dir: Path) -> list[datetime]:
    """Every `ts` in Bob's own shell logs."""
    times: list[datetime] = []
    if not Path(log_dir).is_dir():
        return times
    for path in Path(log_dir).rglob("*.log"):
        try:
            lines = path.read_text(errors="replace").splitlines()
        except OSError:
            continue
        for line in lines:
            try:
                ts = json.loads(line).get("ts")
            except (ValueError, AttributeError):
                continue
            at = tx.parse_timestamp(ts)
            if at:
                times.append(at)
    return times


def session_span_s(debug_dir: Path) -> float | None:
    events = SessionLog(debug_dir, kinds=WORKFLOW_KINDS).all_events()
    if not events:
        return None
    return (max(e.end for e in events) - events[0].at).total_seconds()


def wall_time_fallback(totals: Totals, bob_logs: Path, debug_dir: Path) -> None:
    """Fills an unknown wall time from Bob's shell logs, else from the
    session log's span, dropping its Unknown only when one succeeds."""
    if totals.wall_time_s is not None:
        return
    span = bob_log_span_s(bob_logs)
    if span is None:
        span = session_span_s(debug_dir)
    if span is not None:
        totals.wall_time_s = round(span, 3)
        totals.unknowns = [u for u in totals.unknowns if u.field != "totals.wall_time_s"]


# The record.


@dataclass
class Paths:
    build_info: Path = BUILD_INFO
    agent_dir: Path = AGENT_DIR
    debug_dir: Path = DEBUG_DIR
    bob_logs: Path = BOB_LOGS
    junit: Path = JUNIT
    reward: Path = REWARD
    out: Path = OUT


def build_record(paths: Paths) -> dict:
    """Gathers every section on its own, so one unreadable input costs only
    its own fields, and assembles the record."""
    unknowns: list[Unknown] = []

    def gather(section: str, fn, fallback):
        try:
            value, found = fn()
        except Exception as exc:  # a recorder bug must never cost the record
            unknowns.append(Unknown(section, f"recorder error: {exc}"))
            return fallback
        unknowns.extend(found)
        return value

    run = gather("run", lambda: read_build_info(paths.build_info), dict.fromkeys(("suite", "scenario", *RUN_FIELDS)))
    log = gather("totals", lambda: read_agent_log(paths.agent_dir, run.get("agent")), None)
    if log is None:
        log = missing_log("recorder error reading the agent log")
    if not run.get("agent") and log.agent:
        # The log names the agent when build.json could not.
        unknowns = [u for u in unknowns if u.field != "run.agent"]
    gather("totals.wall_time_s", lambda: (wall_time_fallback(log.totals, paths.bob_logs, paths.debug_dir), []), None)
    unknowns.extend(log.totals.unknowns)
    unknowns.extend(log.subagents.unknowns)
    (checks, groups) = gather("result.checks", lambda: read_checks(paths.junit), ({"passed": None, "total": None, "score": None}, None))
    reward = gather("result.reward", lambda: read_reward(paths.reward), None)
    session = gather("steps", lambda: read_session(paths.debug_dir), None)
    steps = gather("steps", lambda: read_steps(session, log), {"unit": "characters", "workflows": None})
    children, placed = gather("subagents.children", lambda: read_children(log, session), (None, None))
    child_unknowns = [u for u in unknowns if u.field.startswith("subagents.children")]
    gather("steps.children", lambda: count_step_children(steps, session, placed, child_unknowns), None)
    idle = gather("totals.idle_s", lambda: read_idle(log, placed, session, paths.bob_logs), None)
    trace = gather(
        "totals.trace_calls",
        lambda: read_trace_calls(session, paths.debug_dir),
        {"trace_calls": None, "trace_calls_per_task": None},
    )

    totals = log.totals
    return {
        "schema_version": SCHEMA_VERSION,
        "run": {
            "suite": run.get("suite"),
            "scenario": run.get("scenario"),
            "agent": run.get("agent") or log.agent,
            "model": totals.model,
            "spektacular_version": run.get("spektacular_version"),
            "commit": run.get("commit"),
            "dirty": run.get("dirty"),
        },
        "result": {"reward": reward, "checks": checks, "groups": groups},
        "totals": {**{name: getattr(totals, name) for name in tx.TOTAL_FIELDS}, "idle_s": idle, **trace},
        "steps": steps,
        "subagents": {**{name: getattr(log.subagents, name) for name in SUBAGENT_FIELDS}, "children": children},
        "unknown": [{"field": u.field, "reason": u.reason} for u in unknowns],
    }


def write_record(record: dict, out: Path) -> None:
    """Writes the record atomically, readable on the host."""
    out = Path(out)
    out.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=out.parent, prefix=".metrics-", suffix=".json")
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(record, f, indent=2)
            f.write("\n")
        os.chmod(tmp, 0o644)
        os.replace(tmp, out)
    except BaseException:
        Path(tmp).unlink(missing_ok=True)
        raise


def summary(record: dict) -> str:
    t, c = record["totals"], record["result"]["checks"]
    wall = "unknown" if t["wall_time_s"] is None else f"{t['wall_time_s'] / 60:.1f}m"
    cost = "unknown" if t["cost_usd"] is None else f"${t['cost_usd']:.2f}"
    score = "unknown" if c["total"] is None else f"{c['passed']}/{c['total']}"
    turns = "unknown" if t["turns"] is None else t["turns"]
    return (
        f"metrics: {record['run']['agent'] or 'unknown agent'} wall {wall}, turns {turns}, "
        f"cost {cost}, checks {score}, {len(record['unknown'])} unknown"
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Write a harbor run's metrics record. Always exits 0.")
    parser.add_argument("--build-info", type=Path, default=BUILD_INFO)
    parser.add_argument("--agent-dir", type=Path, default=AGENT_DIR)
    parser.add_argument("--debug-dir", type=Path, default=DEBUG_DIR)
    parser.add_argument("--bob-logs", type=Path, default=BOB_LOGS)
    parser.add_argument("--junit", type=Path, default=JUNIT)
    parser.add_argument("--reward", type=Path, default=REWARD)
    parser.add_argument("--out", type=Path, default=OUT)
    try:
        args = parser.parse_args(argv)
        paths = Paths(**{k: v for k, v in vars(args).items()})
        record = build_record(paths)
        write_record(record, paths.out)
        print(summary(record))
    except SystemExit:
        return 0
    except Exception as exc:  # never fail the verifier
        print(f"metrics: no record written: {exc}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
