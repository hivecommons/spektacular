#!/usr/bin/env python3
"""Writes a harbor run's metrics record beside its reward.

Every suite's test.sh runs this once pytest has finished and the reward is
written, and ignores how it exits. It reads:

- the build info the Makefile render target left in /tests/build.json: the
  agent, suite, scenario, Spektacular version and commit;
- the agent's own log under /logs/agent: Claude's stream-json, the ATIF
  trajectory harbor writes for Bob and Codex, or Codex's rollout;
- Spektacular's own logs under /app/.spektacular/logs, read only through
  `spektacular stats` (runlog.py), for every step and its timing, each
  step's split into working, helper-wait and user-wait time, every helper
  agent with the spec and step it was started in, and the waits and gaps:
  the same way for every agent;
- pytest's JUnit XML and the reward, for the check score;

and writes /logs/verifier/metrics.json (schema 3), in the same shape for
every agent and suite. Tokens, models, cost and the helpers' briefs come
from the agent's log; everything else from Spektacular's. A measure a source
does not supply is null and is listed under `unknown` with the reason, never
written as zero, and a log-sourced measure is never filled in from a
transcript.

It never fails the run: each section is gathered on its own, a failure costs
only that section's fields, and it always exits 0. It never writes the
reward. Run it inside the container, or locally against a collected trial:

    python3 metrics.py --agent-dir <trial>/agent \\
        --log-dir <trial>/artifacts/spektacular/logs \\
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
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))

import runlog  # noqa: E402
import transcripts as tx  # noqa: E402
from transcripts import Child, Totals, Unknown  # noqa: E402

SCHEMA_VERSION = 3
WORKFLOW_KINDS = ("spec", "plan", "implement")

BUILD_INFO = Path("/tests/build.json")
AGENT_DIR = Path("/logs/agent")
LOG_DIR = Path("/app/.spektacular/logs")
BOB_LOGS = Path("/root/.bob/logs")
JUNIT = Path("/logs/verifier/junit.xml")
REWARD = Path("/logs/verifier/reward.txt")
OUT = Path("/logs/verifier/metrics.json")

RUN_FIELDS = ("suite", "agent", "spektacular_version", "commit", "dirty")
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
    # Reads the child agents when called, so a reader failure costs only them.
    read_children: Callable[[], tuple[list[Child] | None, list[Unknown]]] = lambda: (None, [])


def no_children(reason: str) -> Callable[[], tuple[None, list[Unknown]]]:
    return lambda: (None, [Unknown("subagents.children", reason)])


def missing_log(reason: str) -> AgentLog:
    totals = Totals()
    totals.unknowns = [Unknown(f"totals.{name}", reason) for name in tx.TOTAL_FIELDS]
    totals.unknowns.append(Unknown("run.model", reason))
    return AgentLog(agent=None, totals=totals, read_children=no_children(reason))


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
                read_children=lambda: tx.atif_children(traj),
            )
        elif log is None:
            reader = "codex-rollout"
    if reader == "codex-rollout":
        log = AgentLog(
            agent="codex",
            totals=tx.codex_rollout_summary(agent_dir / "sessions"),
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


# Steps, helpers, waits and gaps: from Spektacular's logs, through `stats`.


def seconds(ms) -> float | None:
    return round(ms / 1000, 3) if isinstance(ms, (int, float)) else None


def step_rows(report: dict) -> list[dict]:
    """A run report's per-step rows, in seconds, each split into working,
    helper-wait and user-wait time adding up to its duration."""
    first_entry: dict[str, str] = {}
    for e in report.get("step_entries") or []:
        first_entry.setdefault(e["step"], e["started_at"])
    helpers: dict[str, int] = {}
    for h in report.get("helpers") or []:
        helpers[h.get("step")] = helpers.get(h.get("step"), 0) + 1
    rows = []
    for r in report.get("steps") or []:
        duration, helper, user = r["duration_ms"], r["helper_wait_ms"], r["user_wait_ms"]
        rows.append({
            "step": r["step"],
            "started_at": first_entry.get(r["step"]),
            "entries": r["entries"],
            "duration_s": seconds(duration),
            "working_s": seconds(max(0, duration - helper - user)),
            "helper_wait_s": seconds(helper),
            "user_wait_s": seconds(user),
            "commands": r["commands"],
            "helpers": helpers.get(r["step"], 0),
            "incomplete": r["incomplete"],
        })
    return rows


def workflow_entry(summary: dict, report: dict) -> dict:
    t = report.get("totals") or {}
    return {
        "run_id": summary["run_id"],
        "kind": summary["kind"],
        "name": summary["name"],
        "status": summary.get("status"),
        "wall_s": seconds(t.get("wall_ms")),
        "user_wait_s": seconds(t.get("user_wait_ms")),
        "helper_wait_s": seconds(t.get("helper_wait_ms")),
        "gap_s": seconds(t.get("gap_ms")),
        "gaps": t.get("gaps_over_threshold"),
        "steps": step_rows(report),
    }


LOG_FIELDS = (
    "steps", "subagents.started", "subagents.helpers",
    "totals.user_wait_s", "totals.helper_wait_s", "totals.gap_s", "totals.trace_calls",
)


def no_logs(reason: str) -> tuple[dict, list[Unknown]]:
    """Every log-sourced section as null, each listed unknown with reason:
    nothing is filled in from a transcript."""
    return {"workflows": None, "totals": None, "subagents": None, "trace": None}, [Unknown(f, reason) for f in LOG_FIELDS]


def helper_record(s: runlog.Span, lanes: list[runlog.Lane], launched: dict[str, str]) -> dict:
    spec, step = s.spec, s.step
    if s.run_id is None:
        # An orchestrator's helper: the lane of the child launched under it,
        # else the lane whose first run started while it ran.
        spec = launched.get(s.span_id) or next(
            (l.name for l in lanes if l.runs and s.start <= runlog.parse_time(l.runs[0]["started_at"]) <= s.end), None
        )
        step = None
    return {
        "span_id": s.span_id, "parent": s.parent, "name": s.name, "spec": spec, "kind": s.kind,
        "step": step, "task_id": s.task_id, "outside_run": s.run_id is None,
        "started_at": iso(s.start), "ended_at": iso(s.end),
        "duration_s": round((s.end - s.start).total_seconds(), 3),
        "end_reason": s.end_reason, "status": s.status, "complete": s.complete,
    }


def read_logs(log_dir: Path) -> tuple[dict, list[Unknown]]:
    """The record's log-sourced sections, from `spektacular stats` over the
    logs directory, the same way for every agent."""
    log = runlog.RunLog(log_dir)
    try:
        runs = log.runs()
        if not runs:
            return no_logs("the Spektacular logs hold no workflow run")
        reports = [(r, log.report(r["run_id"])) for r in runs]
        spans = log.spans()
        commands = log.commands()
        lanes = list(log.lanes(WORKFLOW_KINDS).values())
    except runlog.Unavailable as u:
        return no_logs(u.reason)

    workflows = [workflow_entry(r, rep) for r, rep in reports]
    rep0 = reports[0][1]

    def total(key: str):
        return round(sum((w[key] for w in workflows if w[key] is not None), 0.0), 3)

    totals = {
        "user_wait_s": total("user_wait_s"),
        "helper_wait_s": total("helper_wait_s"),
        "gap_s": total("gap_s"),
        "gaps": sum(w["gaps"] or 0 for w in workflows),
        "gap_threshold_s": seconds(rep0.get("gap_threshold_ms")),
    }
    helpers = [s for s in spans if s.type == "sub-agent"]
    launched = {}
    for lane in lanes:
        for first in lane.child_starts:
            span = log.child_span(first)
            if span is not None:
                launched[span.span_id] = lane.name
    subagents = {
        "started": len(helpers),
        "completed": sum(1 for s in helpers if s.complete and s.status == "ok"),
        "failed": sum(1 for s in helpers if s.status == "error"),
        "unfinished": sum(1 for s in helpers if not s.complete),
        "durations_s": [round((s.end - s.start).total_seconds(), 3) for s in helpers if s.complete],
        "helpers": [helper_record(s, lanes, launched) for s in helpers],
    }
    calls = sum(1 for c in commands if c.is_verb("trace"))
    tasks = sum(len(rep.get("tasks") or []) for _, rep in reports)
    trace = {"trace_calls": calls, "trace_calls_per_task": round(calls / tasks, 2) if tasks else None}
    return {"workflows": workflows, "totals": totals, "subagents": subagents, "trace": trace}, []


def read_logs_safely(log_dir: Path) -> tuple[dict, list[Unknown]]:
    """read_logs, with a recorder bug costing every log-sourced field, each
    listed unknown, and nothing else."""
    try:
        return read_logs(log_dir)
    except Exception as exc:  # a recorder bug must never cost the record
        return no_logs(f"recorder error: {exc}")


# Child briefs: what each helper was told, from the transcript.


def read_briefs(log: AgentLog) -> tuple[list[dict] | None, list[Unknown]]:
    children, unknowns = log.read_children()
    if children is None:
        return None, [Unknown("subagents.briefs", u.reason) for u in unknowns[:1]] or [Unknown("subagents.briefs", "the agent log shows no helpers")]
    briefs = [
        {
            "id": c.id,
            "parent": c.parent,
            "kind": c.kind,
            "description": c.description,
            "models": sorted(c.models) if c.models is not None else None,
            "prompt": c.prompt,
            "prompt_chars": len(c.prompt) if c.prompt is not None else None,
        }
        for c in children
    ]
    kept = (".kind", ".description", ".models", ".prompt")
    found = [
        Unknown(u.field.replace("subagents.children", "subagents.briefs", 1), u.reason)
        for u in unknowns
        if u.field.endswith(kept)
    ]
    # A prompt the log could not show leaves its size unknown too.
    found += [Unknown(u.field.removesuffix(".prompt") + ".prompt_chars", u.reason) for u in found if u.field.endswith(".prompt")]
    return briefs, found


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


def log_span_s(log_dir: Path) -> float | None:
    """First start to last end over the logs' workflow runs, or None."""
    try:
        runs = runlog.RunLog(log_dir).runs()
    except runlog.Unavailable:
        return None
    if not runs:
        return None
    start = min(runlog.parse_time(r["started_at"]) for r in runs)
    end = max(runlog.parse_time(r["ended_at"]) for r in runs)
    return (end - start).total_seconds()


def wall_time_fallback(totals: Totals, bob_logs: Path, log_dir: Path) -> None:
    """Fills an unknown wall time from Bob's shell logs, else from the span
    of the workflow runs in Spektacular's logs, dropping its Unknown only
    when one succeeds."""
    if totals.wall_time_s is not None:
        return
    span = bob_log_span_s(bob_logs)
    if span is None:
        span = log_span_s(log_dir)
    if span is not None:
        totals.wall_time_s = round(span, 3)
        totals.unknowns = [u for u in totals.unknowns if u.field != "totals.wall_time_s"]


# The record.


@dataclass
class Paths:
    build_info: Path = BUILD_INFO
    agent_dir: Path = AGENT_DIR
    log_dir: Path = LOG_DIR
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
    gather("totals.wall_time_s", lambda: (wall_time_fallback(log.totals, paths.bob_logs, paths.log_dir), []), None)
    unknowns.extend(log.totals.unknowns)
    (checks, groups) = gather("result.checks", lambda: read_checks(paths.junit), ({"passed": None, "total": None, "score": None}, None))
    reward = gather("result.reward", lambda: read_reward(paths.reward), None)
    logs = gather("steps", lambda: read_logs_safely(paths.log_dir), no_logs("recorder error")[0])
    briefs = gather("subagents.briefs", lambda: read_briefs(log), None)

    totals = log.totals
    trace = logs["trace"] or {"trace_calls": None, "trace_calls_per_task": None}
    log_totals = logs["totals"] or dict.fromkeys(("user_wait_s", "helper_wait_s", "gap_s", "gaps", "gap_threshold_s"))
    helpers = logs["subagents"] or {
        **dict.fromkeys(("started", "completed", "failed", "unfinished", "durations_s")), "helpers": None,
    }
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
        "totals": {**{name: getattr(totals, name) for name in tx.TOTAL_FIELDS}, **trace, **log_totals},
        "steps": {"workflows": logs["workflows"]},
        "subagents": {**helpers, "briefs": briefs},
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
    workflows = record["steps"]["workflows"]
    steps = "unknown" if workflows is None else sum(len(w["steps"]) for w in workflows)
    started = record["subagents"]["started"]
    helpers = "unknown" if started is None else started
    return (
        f"metrics: {record['run']['agent'] or 'unknown agent'} wall {wall}, turns {turns}, "
        f"cost {cost}, checks {score}, {steps} steps, {helpers} helpers, {len(record['unknown'])} unknown"
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Write a harbor run's metrics record. Always exits 0.")
    parser.add_argument("--build-info", type=Path, default=BUILD_INFO)
    parser.add_argument("--agent-dir", type=Path, default=AGENT_DIR)
    parser.add_argument("--log-dir", type=Path, default=LOG_DIR)
    # The old name, kept so hand-run commands from before schema 3 still work.
    parser.add_argument("--debug-dir", dest="log_dir", type=Path, default=argparse.SUPPRESS, help=argparse.SUPPRESS)
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
