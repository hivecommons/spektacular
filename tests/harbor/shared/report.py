#!/usr/bin/env python3
"""Reports the metrics records harbor runs leave in their collected jobs.

Every suite's verifier writes verifier/metrics.json beside its reward (see
metrics.py). This reads those records, and each trial's result.json, and
nothing else: never the agent's logs, so it is fast over many jobs and
needs no elevated permissions for the root-owned files a run leaves under
agent/. Trials without a record, such as runs from before the recorder,
are skipped.

    python3 report.py [jobs or trial dirs or globs ...]   # one row per run, by suite
    python3 report.py --run <trial dir or trial name>     # one run's step breakdown and children
    python3 report.py --run <trial> --prompts             # ... and each child's full context
    python3 report.py --baseline <dirs or globs> --candidate <dirs or globs>
                                                          # per-suite medians, before and after

With no paths it reads tests/harbor/jobs. A value it cannot read prints
as `unknown`, never as zero.
"""

from __future__ import annotations

import argparse
import glob
import json
import statistics
import sys
from dataclasses import dataclass
from pathlib import Path

# The thresholds the report flags at. Tune them here: the records store
# measurements only, so a change needs no suite re-run.
DOMINANT_STEP_SHARE = 1 / 3  # a step taking more than this share of its run's wall time
IDLE_SHARE = 0.25  # a run idle for more than this share of its wall time
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

TABLE_HEADERS = ["trial", "agent", "model", "wall m", "idle m", "turns", "out tok", "cache-read tok", "cost $", "checks", "result"]
IDLE_MARK = "  ◀ idle"


def table(runs: list[Run]) -> str:
    groups: dict[tuple[str, str | None], list[Run]] = {}
    for run in runs:
        groups.setdefault(run.group, []).append(run)
    out = []
    for group in sorted(groups, key=lambda g: (g[0], g[1] or "")):
        out.append(group_title(group))
        rows, marks = [], []
        for run in groups[group]:
            meta = run.record.get("run") or {}
            marks.append(IDLE_MARK if is_idle(run) else "")
            rows.append(
                [
                    run.trial,
                    meta.get("agent") or UNKNOWN,
                    meta.get("model") or UNKNOWN,
                    minutes(run.total("wall_time_s")),
                    minutes(run.total("idle_s")),
                    number(run.total("turns")),
                    number(run.total("output_tokens")),
                    number(run.total("cache_read_tokens")),
                    number(run.total("cost_usd"), "{:.2f}"),
                    checks_text((run.record.get("result") or {}).get("checks")),
                    result_text(run.reward),
                ]
            )
        lines = render(TABLE_HEADERS, rows, right={3, 4, 5, 6, 7, 8})
        out.extend(line + mark for line, mark in zip(lines, ["", "", *marks]))
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


def idle_share(run: Run) -> float | None:
    """The run's idle time as a share of its wall time, when both are known."""
    idle, wall = run.total("idle_s"), run_wall_time(run)
    return idle / wall if idle is not None and wall else None


def is_idle(run: Run) -> bool:
    share = idle_share(run)
    return share is not None and share > IDLE_SHARE


# Child agents, from a version-2 record.

CHILD_HEADERS = ["child", "kind", "spec", "step", "bg", "seconds", "ending", "context"]


def children(run: Run) -> list[dict] | None:
    return (run.record.get("subagents") or {}).get("children")


def has_children_key(run: Run) -> bool:
    return "children" in (run.record.get("subagents") or {})


def unknown_child_fields(run: Run) -> set[str]:
    return {u.get("field") for u in run.record.get("unknown") or [] if str(u.get("field", "")).startswith("subagents.children")}


def child_value(child: dict, name: str, unknown: set[str]) -> object:
    """A child's field, or UNKNOWN where the record lists it unknown."""
    value = child.get(name)
    if value is None and {f"subagents.children.{name}", f"subagents.children[{child.get('id')}].{name}"} & unknown:
        return UNKNOWN
    return value


def child_depth(child: dict, by_id: dict[str, dict]) -> int:
    depth, seen = 0, set()
    while child.get("parent") in by_id and child["parent"] not in seen:
        seen.add(child["parent"])
        child = by_id[child["parent"]]
        depth += 1
    return depth


def short_id(child_id: str | None) -> str:
    return (child_id or UNKNOWN)[-8:]


def children_table(run: Run) -> list[str]:
    if not has_children_key(run):
        return ["Children: not recorded (schema 1)"]
    kids = children(run)
    if kids is None:
        return ["Children: unknown (see below)"]
    if not kids:
        return ["Children: none"]
    unknown = unknown_child_fields(run)
    by_id = {c.get("id"): c for c in kids}
    rows = []
    for c in kids:
        depth = child_depth(c, by_id)
        step, spec, bg = (child_value(c, n, unknown) for n in ("step", "spec", "background"))
        rows.append(
            [
                ("  " * (depth - 1) + "└ " if depth else "") + short_id(c.get("id")),
                child_value(c, "kind", unknown) or UNKNOWN,
                spec if spec is not None else "-",
                step if step is not None else "-",
                UNKNOWN if bg in (None, UNKNOWN) else "bg" if bg else "fg",
                number(c.get("duration_s"), "{:.1f}"),
                c.get("ending") or UNKNOWN,
                number(c.get("prompt_chars")),
            ]
        )
    return ["Children:", *("  " + line for line in render(CHILD_HEADERS, rows, right={5, 7}))]


def prompts(run: Run) -> str:
    """Every child's full context, each under a header naming it."""
    kids = children(run) or []
    unknown = unknown_child_fields(run)
    out = []
    for c in kids:
        spec, step = (child_value(c, n, unknown) for n in ("spec", "step"))
        out.append(
            f"── {c.get('id')} {child_value(c, 'kind', unknown) or UNKNOWN} "
            f"{spec if spec is not None else '-'}/{step if step is not None else '-'} ({number(c.get('prompt_chars'))} chars)"
        )
        out.append(c.get("prompt") if c.get("prompt") is not None else UNKNOWN)
        out.append("")
    if not has_children_key(run):
        out.append("No child contexts recorded (schema 1).")
    elif children(run) is None:
        out.append("Child contexts: unknown (see the breakdown's Unknown list).")
    elif not kids:
        out.append("No child contexts recorded.")
    return "\n".join(out)


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
    share = idle_share(run)
    idle = run.total("idle_s")
    out.append(
        f"Idle: {number(idle, '{:.1f}')} s of {number(wall, '{:.1f}')} s"
        + ("" if share is None else f" ({share:.0%})")
        + (IDLE_MARK if is_idle(run) else "")
    )
    out.append("")
    out.extend(children_table(run))
    unknowns = run.record.get("unknown") or []
    if unknowns:
        out.append("")
        out.append("Unknown:")
        out.extend(f"  {u.get('field')}: {u.get('reason')}" for u in unknowns)
    out.append("")
    return "\n".join(out)


# Comparing a baseline set of runs with a candidate set.


@dataclass
class Measure:
    label: str
    value: object  # Run -> float | None
    fmt: object  # float -> str
    is_score: bool = False


def total_tokens(run: Run) -> float | None:
    parts = [run.total(n) for n in ("input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens")]
    return None if any(p is None for p in parts) else sum(parts)


def check_score(run: Run) -> float | None:
    return ((run.record.get("result") or {}).get("checks") or {}).get("score")


def count(value: float) -> str:
    return f"{value:,.0f}" if float(value).is_integer() else f"{value:,.1f}"


MEASURES = [
    Measure("wall m", lambda r: r.total("wall_time_s"), lambda v: f"{v / 60:.1f}"),
    Measure("turns", lambda r: r.total("turns"), count),
    Measure("out tok", lambda r: r.total("output_tokens"), count),
    Measure("total tok", total_tokens, count),
    Measure("cost $", lambda r: r.total("cost_usd"), lambda v: f"{v:.2f}"),
    Measure("check score", check_score, lambda v: f"{v:.2f}", is_score=True),
]


@dataclass
class Flag:
    group: tuple[str, str | None]
    measure: str
    baseline: float
    candidate: float


def median(runs: list[Run], measure: Measure) -> float | None:
    """The median over the runs whose value is known, or None if none is."""
    known = [v for v in (measure.value(r) for r in runs) if v is not None]
    return statistics.median(known) if known else None


def is_regression(measure: Measure, base: float | None, cand: float | None) -> bool:
    """A cost-like measure at least REGRESSION_RATIO times the baseline, or a
    check score down by SCORE_DROP or more. An unknown side never flags."""
    if base is None or cand is None:
        return False
    if measure.is_score:
        return round(base - cand, 9) >= SCORE_DROP
    return base > 0 and cand >= REGRESSION_RATIO * base


def by_group(runs: list[Run]) -> dict[tuple[str, str | None], list[Run]]:
    groups: dict[tuple[str, str | None], list[Run]] = {}
    for run in runs:
        groups.setdefault(run.group, []).append(run)
    return groups


def regressions(baseline: list[Run], candidate: list[Run]) -> list[Flag]:
    base, cand = by_group(baseline), by_group(candidate)
    flags = []
    for group in sorted(base.keys() & cand.keys(), key=lambda g: (g[0], g[1] or "")):
        for m in MEASURES:
            b, c = median(base[group], m), median(cand[group], m)
            if is_regression(m, b, c):
                flags.append(Flag(group, m.label, b, c))
    return flags


def change(measure: Measure, base: float | None, cand: float | None) -> str:
    if base is None or cand is None:
        return UNKNOWN
    if measure.is_score:
        return f"{cand - base:+.2f}"
    delta = measure.fmt(abs(cand - base))
    sign = "-" if cand < base else "+"
    return f"{sign}{delta}" + (f" (x{cand / base:.2f})" if base > 0 else "")


def compare(baseline: list[Run], candidate: list[Run]) -> str:
    """Per suite present in both sets, the median of each measure before and
    after, the change, and a mark on each regression."""
    base, cand = by_group(baseline), by_group(candidate)
    flagged = {(f.group, f.measure) for f in regressions(baseline, candidate)}
    out = []
    for group in sorted(base.keys() & cand.keys(), key=lambda g: (g[0], g[1] or "")):
        out.append(f"{group_title(group)}  baseline {len(base[group])} run(s), candidate {len(cand[group])} run(s)")
        rows, marks = [], []
        for m in MEASURES:
            b, c = median(base[group], m), median(cand[group], m)
            rows.append([m.label, UNKNOWN if b is None else m.fmt(b), UNKNOWN if c is None else m.fmt(c), change(m, b, c)])
            marks.append("  ◀ regression" if (group, m.label) in flagged else "")
        lines = render(["median", "baseline", "candidate", "change"], rows, right={1, 2, 3})
        out.extend(line + mark for line, mark in zip(lines, ["", "", *marks]))
        out.append("")
    only_base = sorted(base.keys() - cand.keys(), key=lambda g: (g[0], g[1] or ""))
    only_cand = sorted(cand.keys() - base.keys(), key=lambda g: (g[0], g[1] or ""))
    if only_base or only_cand:
        out.append("Unmatched, in one set only:")
        out.extend(f"  {group_title(g).strip('= ')}: baseline only" for g in only_base)
        out.extend(f"  {group_title(g).strip('= ')}: candidate only" for g in only_cand)
        out.append("")
    if not base.keys() & cand.keys():
        out.insert(0, "No suite has runs in both sets.\n")
    return "\n".join(out)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Report the metrics records of collected harbor runs.")
    parser.add_argument("paths", nargs="*", help=f"job or trial directories, or globs (default: {DEFAULT_JOBS})")
    parser.add_argument("--run", metavar="TRIAL", help="show one run's step breakdown: a trial directory or trial name")
    parser.add_argument("--prompts", action="store_true", help="with --run, also print each child agent's full context")
    parser.add_argument("--baseline", nargs="+", metavar="DIR", help="job directories or globs: the runs before a change")
    parser.add_argument("--candidate", nargs="+", metavar="DIR", help="job directories or globs: the runs after it")
    args = parser.parse_args(argv)

    if args.prompts and not args.run:
        print("report: --prompts prints one run's child contexts, so it needs --run <trial>", file=sys.stderr)
        return 2
    if args.baseline or args.candidate:
        if not (args.baseline and args.candidate) or args.run or args.paths:
            print(
                "report: a comparison takes --baseline and --candidate together, each with job directories "
                "or globs, and no other paths or --run",
                file=sys.stderr,
            )
            return 2
        baseline, candidate = load_runs(args.baseline), load_runs(args.candidate)
        if not baseline or not candidate:
            empty = "baseline" if not baseline else "candidate"
            print(f"report: the {empty} set has no metrics records; pass job directories whose runs have one", file=sys.stderr)
            return 2
        print(compare(baseline, candidate))
        return 0

    paths = args.paths or [str(DEFAULT_JOBS)]
    if args.run and Path(args.run).is_dir():
        # A trial dir is found wherever it lives, without the default jobs.
        paths = [*args.paths, args.run]
    runs = load_runs(paths)
    if args.run:
        try:
            run = find_run(runs, args.run)
            print(breakdown(run))
            if args.prompts:
                print(prompts(run))
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
