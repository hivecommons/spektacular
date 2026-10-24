"""Judges a Claude run's child agents against what the workflows require.

The metrics recorder measures; this module judges. At the top is the oracle:
the hand-kept list of workflow steps whose instructions require a child
agent, those that only suggest one, and what a required child's context
must hold. Below it are small rules over the placed children, the session
log's lanes and the transcript's turns and wake-ups, each returning named
Failures, so a verifier asserts the list is empty and its failure message
names the step, spec or child at fault.

The oracle is maintained by hand, never derived from the templates at run
time: an oracle that reads the instructions it checks would agree with
them by construction. A local self-check ties the two instead, failing when
a step template's delegation wording and the oracle disagree, so the rules
follow the instructions when that set changes.

These rules are enforced for Claude runs only; other agents' logs cannot
show where a child was started. Copied into each suite's tests/ by the
Makefile with the other shared helpers.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import metrics
import transcripts as tx
from metrics import PlacedChild
from sessionlog import Lane, SessionLog, StepWindow
from transcripts import Wakeup

# The oracle. (workflow kind, step) pairs, by hand from the step templates.

# Steps whose instructions require a child: doing the step in the agent's
# own context does not complete it.
REQUIRED = {("spec", "verification"), ("implement", "test"), ("implement", "verify"), ("implement", "check_goal")}
# Steps whose instructions only suggest one. Recorded, never enforced.
SUGGESTED = {("plan", "discovery"), ("implement", "analyze"), ("implement", "update_changelog")}
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
    """What the rules read from one Claude run."""

    placed: list[PlacedChild]
    session: SessionLog
    windows: dict[tuple[str, str], list[StepWindow]]
    turns: list[tx.Turn]
    wakeups: list[Wakeup]
    end: datetime | None  # the run's last timestamp


def read_run(transcript: Path, debug_dir: Path) -> Run:
    """Reads the transcript and the session log, and places the children
    exactly as the metrics record does."""
    objs = tx.read_jsonl(Path(transcript))
    children, _ = tx.claude_children(objs)
    session = SessionLog(debug_dir, kinds=metrics.WORKFLOW_KINDS)
    windows = metrics.lane_windows(session) or {}
    placed, _ = metrics.place_children(children, windows, tx.claude_messages(objs))
    times = [t for t in (tx.parse_timestamp(o.get("timestamp")) for o in objs) if t]
    return Run(placed, session, windows, tx.claude_turns(objs), tx.claude_wakeups(objs), max(times) if times else None)


def worktree_paths(session: SessionLog) -> dict[str, list[str]]:
    """Each spec's worktree paths, from its successful `epic worktree` responses."""
    out: dict[str, list[str]] = {}
    for e in session.all_events():
        if not (e.ok and e.is_verb("epic", "worktree")):
            continue
        try:
            response = json.loads(e.response)
        except (json.JSONDecodeError, TypeError):
            continue
        spec = response.get("spec") or e.data().get("spec")
        paths = [r.get("path") for r in response.get("repos") or [] if isinstance(r, dict) and r.get("path")]
        if spec and paths:
            out[spec] = paths
    return out


# The rules.


def step_runs(windows: list[StepWindow]) -> list[tuple[str, list[int]]]:
    """The lane's steps as runs of consecutive windows at one step: a resumed
    child opens a fresh window on the same step, which is still one step."""
    runs: list[tuple[str, list[int]]] = []
    for i, w in enumerate(windows):
        if runs and runs[-1][0] == w.step:
            runs[-1][1].append(i)
        else:
            runs.append((w.step, [i]))
    return runs


def missing_required(placed: list[PlacedChild], windows_by_lane: dict[tuple[str, str], list[StepWindow]]) -> list[Failure]:
    """Each required step a lane completed with no child, at any depth, started in it.

    A step is completed when the lane moved on to another step afterwards;
    the step a lane ended on may have been cut off, and is not judged.
    """
    started = {(p.lane, p.window) for p in placed if p.lane is not None and p.window is not None}
    out = []
    for (kind, name), windows in windows_by_lane.items():
        runs = step_runs(windows)
        for n, (step, indexes) in enumerate(runs[:-1]):
            if (kind, step) not in REQUIRED:
                continue
            if not any(((kind, name), i) in started for i in indexes):
                attempt = sum(1 for s, _ in runs[:n] if s == step) + 1
                out.append(
                    Failure(
                        "missing_child",
                        f"{kind}:{name}:{step}" + (f" (attempt {attempt})" if attempt > 1 else ""),
                        f"the {step} step completed without a child agent started in it",
                    )
                )
    return out


def spec_children(placed: list[PlacedChild], lane: tuple[str, str]) -> list[PlacedChild]:
    """The top-level children that worked one epic lane's spec, in launch order."""
    return [p for p in placed if p.child.parent is None and p.spec == lane[1] and p.lane == lane]


def missing_spec_children(placed: list[PlacedChild], lanes: list[tuple[str, str]]) -> list[Failure]:
    """Each epic lane whose spec no top-level child of its own worked."""
    return [
        Failure("missing_spec_child", f"{kind}:{name}", f"no child of the orchestrator was started for spec {name}")
        for kind, name in lanes
        if kind in PER_SPEC_CHILD and not spec_children(placed, (kind, name))
    ]


def _squash(text: str) -> str:
    return re.sub(r"[\s\\]", "", text)


def epic_context_gaps(
    placed: list[PlacedChild],
    lanes: dict[tuple[str, str], Lane],
    root: str = PROJECT_ROOT,
    worktrees: dict[str, list[str]] | None = None,
) -> list[Failure]:
    """Each epic child whose prompt lacks what the epic skill says to pass:
    its spec, the project root and that its run is orchestrated; for an
    implementing child, its spec's worktree paths; and for a child continuing
    a spec an earlier child worked, the instruction to resume it. Only for an
    epic run, whose orchestrator starts one child per spec."""
    out = []
    for key in lanes:
        kind, name = key
        if kind not in PER_SPEC_CHILD:
            continue
        for n, p in enumerate(spec_children(placed, key)):
            prompt = p.child.prompt or ""
            missing = []
            if name not in prompt:
                missing.append(f"the spec it works on ({name})")
            if root not in prompt:
                missing.append(f"the project root ({root})")
            if ORCHESTRATED not in _squash(prompt) and ORCHESTRATED_WORDS not in prompt.lower():
                missing.append(f"that its run is orchestrated ({ORCHESTRATED})")
            if kind == "implement":
                missing += [f"its code location {path}" for path in (worktrees or {}).get(name, []) if path not in prompt]
            if n > 0 and RESUME not in prompt.lower():
                missing.append("the instruction to resume the spec's earlier run")
            out += [_gap(p, what) for what in missing]
    return out


def review_context_gaps(placed: list[PlacedChild]) -> list[Failure]:
    """Each spec review child not given the staged spec to read, or not told
    to review only what is written there."""
    out = []
    for p in placed:
        if p.lane is None or p.lane[0] != "spec" or p.step != "verification":
            continue
        prompt = p.child.prompt or ""
        if STAGED_SPEC not in prompt:
            out.append(_gap(p, f"the staged spec to read ({STAGED_SPEC})"))
        if not any(phrase in prompt.lower() for phrase in REVIEW_ONLY_WRITTEN):
            out.append(_gap(p, "the instruction to review only what is written"))
    return out


def _gap(p: PlacedChild, what: str) -> Failure:
    return Failure("context_gap", f"child {p.child.id} ({p.spec or '-'})", f"its prompt does not give {what}")


def running(p: PlacedChild, at: datetime, run_end: datetime | None) -> bool:
    start, end = p.child.launched_at, p.child.ended_at or run_end
    return start is not None and start <= at and (end is None or at < end)


def wakeups_while_children_run(wakeups: list[Wakeup], placed: list[PlacedChild], run_end: datetime | None = None) -> list[Failure]:
    """Each timed wake-up, timer or sleep the main agent set while a child of
    its own was running. A child counts as running until its end time, or the
    run's end when that is unknown."""
    top = [p for p in placed if p.child.parent is None]
    return [
        Failure("wakeup_while_children_run", f"{w.tool} {w.detail}", f"set at {w.at.isoformat()} while a child was running")
        for w in wakeups
        if w.at is not None and any(running(p, w.at, run_end) for p in top)
    ]
