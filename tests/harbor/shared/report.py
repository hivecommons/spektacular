#!/usr/bin/env python3
"""Reports the metrics records harbor runs leave in their collected jobs.

Every suite's verifier writes verifier/metrics.json beside its reward (see
metrics.py). This reads those records, and each trial's result.json, and
nothing else: never the agent's logs, so it is fast over many jobs and
needs no elevated permissions for the root-owned files a run leaves under
agent/. Trials without a record, such as runs from before the recorder,
are skipped.

    python3 report.py [jobs or trial dirs or globs ...]   # one row per run, by suite
    python3 report.py --run <trial dir or trial name>     # one run's step breakdown

With no paths it reads tests/harbor/jobs. A value it cannot read prints
as `unknown`, never as zero.
"""

from __future__ import annotations

import argparse
import glob
import json
import sys
from dataclasses import dataclass
from pathlib import Path

# The thresholds the report flags at. Tune them here: the records store
# measurements only, so a change needs no suite re-run.
DOMINANT_STEP_SHARE = 1 / 3  # a step taking more than this share of its run's wall time
REGRESSION_RATIO = 2.0  # a candidate median at least this multiple of the baseline's
SCORE_DROP = 0.1  # a check-score median falling by at least this much

DEFAULT_JOBS = Path(__file__).resolve().parent.parent / "jobs"
RECORD = Path("verifier") / "metrics.json"
UNKNOWN = "unknown"


@dataclass
class Run:
    trial_dir: Path
    trial: str
    job: str
    started_at: str | None
    reward: float | None
    record: dict

    @property
    def suite(self) -> str:
        return (self.record.get("run") or {}).get("suite") or "unknown suite"

    @property
    def scenario(self) -> str | None:
        return (self.record.get("run") or {}).get("scenario")

    @property
    def group(self) -> tuple[str, str | None]:
        return (self.suite, self.scenario)

    def total(self, name: str):
        return (self.record.get("totals") or {}).get(name)


def warn(message: str) -> None:
    print(f"report: {message}", file=sys.stderr)


def expand(paths: list[str]) -> list[Path]:
    """Each argument as given, or every directory a glob in it matches."""
    out = []
    for arg in paths:
        if any(c in arg for c in "*?["):
            matches = sorted(glob.glob(arg))
            if not matches:
                warn(f"{arg} matches nothing")
            out.extend(Path(m) for m in matches)
        else:
            out.append(Path(arg))
    return out


def record_paths(base: Path) -> list[Path]:
    """The records under a trial dir, a job dir, or a directory of jobs."""
    found = []
    for pattern in (RECORD, Path("*") / RECORD, Path("*") / "*" / RECORD):
        found.extend(sorted(base.glob(str(pattern))))
    return found


def read_json(path: Path, action: str = "skipping") -> dict | None:
    try:
        loaded = json.loads(path.read_text())
    except (OSError, ValueError) as exc:
        warn(f"{action} unreadable {path}: {exc}")
        return None
    if not isinstance(loaded, dict):
        warn(f"{action} {path}: not a JSON object")
        return None
    return loaded


def load_run(record_path: Path) -> Run | None:
    record = read_json(record_path)
    if record is None:
        return None
    trial_dir = record_path.parent.parent
    result = {}
    if (trial_dir / "result.json").exists():
        result = read_json(trial_dir / "result.json", action="ignoring") or {}
    rewards = (result.get("verifier_result") or {}).get("rewards") or {}
    reward = rewards.get("reward")
    if reward is None:
        reward = (record.get("result") or {}).get("reward")
    return Run(
        trial_dir=trial_dir,
        trial=result.get("trial_name") or trial_dir.name,
        job=trial_dir.parent.name,
        started_at=result.get("started_at"),
        reward=reward,
        record=record,
    )


def load_runs(paths: list[str]) -> list[Run]:
    """Every recorded run under the given paths, oldest first."""
    seen: set[Path] = set()
    runs = []
    for base in expand(paths):
        if not base.is_dir():
            warn(f"{base} is not a directory")
            continue
        for path in record_paths(base):
            key = path.resolve()
            if key in seen:
                continue
            seen.add(key)
            run = load_run(path)
            if run:
                runs.append(run)
    return sorted(runs, key=lambda r: (r.started_at is None, r.started_at or "", r.trial))


# Formatting.


def number(value, fmt: str = "{:,}") -> str:
    return UNKNOWN if value is None else fmt.format(value)


def minutes(seconds) -> str:
    return UNKNOWN if seconds is None else f"{seconds / 60:.1f}"


def checks_text(checks: dict | None) -> str:
    checks = checks or {}
    if checks.get("total") is None:
        return UNKNOWN
    score = checks.get("score")
    return f"{checks.get('passed')}/{checks['total']}" + ("" if score is None else f" ({score:.2f})")


def result_text(reward) -> str:
    if reward is None:
        return UNKNOWN
    return "pass" if reward >= 1 else "fail"


def render(headers: list[str], rows: list[list[str]], right: set[int]) -> list[str]:
    widths = [max(len(str(c)) for c in col) for col in zip(headers, *rows)]

    def line(cells):
        return "  ".join(
            str(c).rjust(w) if i in right else str(c).ljust(w) for i, (c, w) in enumerate(zip(cells, widths))
        ).rstrip()

    return [line(headers), line(["-" * w for w in widths]), *(line(r) for r in rows)]


def group_title(group: tuple[str, str | None]) -> str:
    suite, scenario = group
    return f"== {suite}" + (f" ({scenario})" if scenario else "") + " =="


# The run table.

TABLE_HEADERS = ["trial", "agent", "model", "wall m", "turns", "out tok", "cache-read tok", "cost $", "checks", "result"]


def table(runs: list[Run]) -> str:
    groups: dict[tuple[str, str | None], list[Run]] = {}
    for run in runs:
        groups.setdefault(run.group, []).append(run)
    out = []
    for group in sorted(groups, key=lambda g: (g[0], g[1] or "")):
        out.append(group_title(group))
        rows = []
        for run in groups[group]:
            meta = run.record.get("run") or {}
            rows.append(
                [
                    run.trial,
                    meta.get("agent") or UNKNOWN,
                    meta.get("model") or UNKNOWN,
                    minutes(run.total("wall_time_s")),
                    number(run.total("turns")),
                    number(run.total("output_tokens")),
                    number(run.total("cache_read_tokens")),
                    number(run.total("cost_usd"), "{:.2f}"),
                    checks_text((run.record.get("result") or {}).get("checks")),
                    result_text(run.reward),
                ]
            )
        out.extend(render(TABLE_HEADERS, rows, right={3, 4, 5, 6, 7}))
        out.append("")
    return "\n".join(out)


# One run's step breakdown.


def find_run(runs: list[Run], key: str) -> Run:
    """The run a trial dir or trial name (or a unique prefix of one) names."""
    path = Path(key)
    if path.is_dir():
        for run in runs:
            if run.trial_dir.resolve() == path.resolve():
                return run
    exact = [r for r in runs if r.trial == key]
    if len(exact) == 1:
        return exact[0]
    matches = exact or [r for r in runs if r.trial.startswith(key)]
    if len(matches) == 1:
        return matches[0]
    if not matches:
        raise LookupError(f"no recorded run matches {key!r}; pass a trial directory or a trial name from the table")
    names = "\n  ".join(str(r.trial_dir) for r in matches)
    raise LookupError(f"{key!r} matches {len(matches)} runs; pass one trial directory instead:\n  {names}")


def workflows(run: Run) -> list[dict]:
    return (run.record.get("steps") or {}).get("workflows") or []


def run_wall_time(run: Run) -> float | None:
    """The run's wall time, else the sum of its step durations."""
    wall = run.total("wall_time_s")
    if wall:
        return wall
    durations = [s.get("duration_s") or 0 for w in workflows(run) for s in w.get("steps") or []]
    return sum(durations) or None


def dominant_steps(run: Run) -> list[tuple[str, str, float]]:
    """(lane, step, share) for each step over DOMINANT_STEP_SHARE of the run."""
    wall = run_wall_time(run)
    if not wall:
        return []
    out = []
    for w in workflows(run):
        for s in w.get("steps") or []:
            share = (s.get("duration_s") or 0) / wall
            if share > DOMINANT_STEP_SHARE:
                out.append((w.get("name"), s.get("step"), share))
    return out


def breakdown(run: Run) -> str:
    meta = run.record.get("run") or {}
    steps = run.record.get("steps") or {}
    unit = steps.get("unit") or "characters"
    wall = run_wall_time(run)
    dominant = {(lane, step) for lane, step, _ in dominant_steps(run)}
    out = [
        f"== {run.trial} ==",
        f"{run.suite}" + (f" ({run.scenario})" if run.scenario else "")
        + f", {meta.get('agent') or UNKNOWN} on {meta.get('model') or UNKNOWN}, {result_text(run.reward)}",
        f"wall {minutes(run.total('wall_time_s'))} m, turns {number(run.total('turns'))}, "
        f"cost ${number(run.total('cost_usd'), '{:.2f}')}, "
        f"checks {checks_text((run.record.get('result') or {}).get('checks'))}",
        "",
    ]
    if steps.get("workflows") is None:
        out.append("Steps: unknown (see below)")
    elif not workflows(run):
        out.append("Steps: the run entered no workflow")
    for w in workflows(run):
        out.append(f"{w.get('kind')} {w.get('name')}")
        rows, marks = [], []
        for s in w.get("steps") or []:
            duration = s.get("duration_s")
            share = UNKNOWN if duration is None or not wall else f"{duration / wall:.0%}"
            rows.append([s.get("step"), number(duration, "{:.1f}"), share, number(s.get("output"))])
            marks.append("  ◀ dominant" if (w.get("name"), s.get("step")) in dominant else "")
        lines = render(["step", "seconds", "share", unit], rows, right={1, 2, 3})
        out.extend("  " + line + mark for line, mark in zip(lines, ["", "", *marks]))
        out.append("")
    if "orchestrator_output" in steps:
        out.append(f"Orchestrator output, outside every lane: {number(steps['orchestrator_output'])} {unit}")
        out.append("")

    sub = run.record.get("subagents") or {}
    durations = sub.get("durations_s")
    out.append(
        f"Sub-agents: started {number(sub.get('started'))}, completed {number(sub.get('completed'))}, "
        f"failed {number(sub.get('failed'))}, stopped {number(sub.get('stopped'))}; durations (s): "
        + (UNKNOWN if durations is None else (", ".join(f"{d:.1f}" for d in durations) or "none"))
    )
    unknowns = run.record.get("unknown") or []
    if unknowns:
        out.append("")
        out.append("Unknown:")
        out.extend(f"  {u.get('field')}: {u.get('reason')}" for u in unknowns)
    out.append("")
    return "\n".join(out)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Report the metrics records of collected harbor runs.")
    parser.add_argument("paths", nargs="*", help=f"job or trial directories, or globs (default: {DEFAULT_JOBS})")
    parser.add_argument("--run", metavar="TRIAL", help="show one run's step breakdown: a trial directory or trial name")
    args = parser.parse_args(argv)

    paths = args.paths or [str(DEFAULT_JOBS)]
    if args.run and Path(args.run).is_dir():
        paths.append(args.run)  # a trial dir is found wherever it lives
    runs = load_runs(paths)
    if args.run:
        try:
            print(breakdown(find_run(runs, args.run)))
        except LookupError as exc:
            print(f"report: {exc}", file=sys.stderr)
            return 2
        return 0
    if not runs:
        where = ", ".join(args.paths) or str(DEFAULT_JOBS)
        print(f"No metrics records under {where}. Runs from before the recorder have none.")
        return 0
    print(table(runs))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
