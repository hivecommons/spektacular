#!/usr/bin/env python3
"""Reports the metrics records harbor runs leave in their collected jobs.

Every suite's verifier writes verifier/metrics.json beside its reward (see
metrics.py). This reads those records, and each trial's result.json, and
nothing else: never the agent's logs, so it is fast over many jobs and
needs no elevated permissions for the root-owned files a run leaves under
agent/. Trials without a record, such as runs from before the recorder,
are skipped.

    python3 report.py [jobs or trial dirs or globs ...]   # one row per run, by suite
    python3 report.py --run <trial dir or trial name>     # one run's step split, helpers and totals
    python3 report.py --run <trial> --prompts             # ... and each helper's brief
    python3 report.py --compare <trial A> <trial B>       # two trials side by side, with B's difference
    python3 report.py --baseline <dirs or globs> --candidate <dirs or globs>
                                                          # per-suite medians, before and after

It reads schema-3 records, whose steps, helpers, waits and gaps come from
Spektacular's own logs, and still loads schema-1 and schema-2 records: a
figure an older record does not hold prints as `unknown`.

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
GAP_SHARE = 0.25  # a run whose gaps take more than this share of its wall time
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

TABLE_HEADERS = [
    "trial", "agent", "model", "wall m", "user wait m", "gap m", "turns", "out tok", "cache-read tok", "cost $", "checks", "result",
]
GAP_MARK = "  ◀ gaps"


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
            marks.append(GAP_MARK if is_gappy(run) else "")
            rows.append(
                [
                    run.trial,
                    meta.get("agent") or UNKNOWN,
                    meta.get("model") or UNKNOWN,
                    minutes(run.total("wall_time_s")),
                    minutes(run.total("user_wait_s")),
                    minutes(run.total("gap_s")),
                    number(run.total("turns")),
                    number(run.total("output_tokens")),
                    number(run.total("cache_read_tokens")),
                    number(run.total("cost_usd"), "{:.2f}"),
                    checks_text((run.record.get("result") or {}).get("checks")),
                    result_text(run.reward),
                ]
            )
        lines = render(TABLE_HEADERS, rows, right={3, 4, 5, 6, 7, 8, 9})
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


def gap_share(run: Run) -> float | None:
    """The run's gap time as a share of its wall time, when both are known."""
    gap, wall = run.total("gap_s"), run_wall_time(run)
    return gap / wall if gap is not None and wall else None


def is_gappy(run: Run) -> bool:
    share = gap_share(run)
    return share is not None and share > GAP_SHARE


def seconds(value) -> str:
    return number(value, "{:.1f}")


STEP_HEADERS = ["step", "entries", "seconds", "share", "working", "helper wait", "user wait", "helpers"]


def step_table(w: dict, wall: float | None, dominant: set) -> list[str]:
    rows, marks = [], []
    for s in w.get("steps") or []:
        duration = s.get("duration_s")
        share = UNKNOWN if duration is None or not wall else f"{duration / wall:.0%}"
        rows.append([
            s.get("step"), number(s.get("entries")), seconds(duration), share, seconds(s.get("working_s")),
            seconds(s.get("helper_wait_s")), seconds(s.get("user_wait_s")), number(s.get("helpers")),
        ])
        marks.append("  ◀ dominant" if (w.get("name"), s.get("step")) in dominant else "")
    lines = render(STEP_HEADERS, rows, right={1, 2, 3, 4, 5, 6, 7})
    return ["  " + line + mark for line, mark in zip(lines, ["", "", *marks])]


HELPER_HEADERS = ["helper", "spec", "step", "seconds", "end"]


def helpers_table(run: Run) -> list[str]:
    sub = run.record.get("subagents") or {}
    if "helpers" not in sub:
        return [f"Helper spans: not recorded (schema {run.record.get('schema_version') or 1})"]
    helpers = sub.get("helpers")
    if helpers is None:
        return ["Helper spans: unknown (see below)"]
    if not helpers:
        return ["Helper spans: none"]
    rows = [
        [
            h.get("name") or UNKNOWN,
            h.get("spec") or "-",
            h.get("step") or ("outside any run" if h.get("outside_run") else "-"),
            seconds(h.get("duration_s")),
            (h.get("end_reason") or UNKNOWN) + ("" if h.get("complete") else "*"),
        ]
        for h in helpers
    ]
    return ["Helper spans:", *("  " + line for line in render(HELPER_HEADERS, rows, right={3}))]


def prompts(run: Run) -> str:
    """Every helper's brief, each under a header naming it."""
    sub = run.record.get("subagents") or {}
    briefs = sub.get("briefs")
    out = []
    for b in briefs or []:
        out.append(f"── {b.get('id')} {b.get('kind') or UNKNOWN} {b.get('description') or '-'} ({number(b.get('prompt_chars'))} chars)")
        out.append(b.get("prompt") if b.get("prompt") is not None else UNKNOWN)
        out.append("")
    if "briefs" not in sub:
        out.append(f"No briefs recorded (schema {run.record.get('schema_version') or 1}).")
    elif briefs is None:
        out.append("Briefs: unknown (see the breakdown's Unknown list).")
    elif not briefs:
        out.append("No briefs recorded.")
    return "\n".join(out)


def breakdown(run: Run) -> str:
    meta = run.record.get("run") or {}
    steps = run.record.get("steps") or {}
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
        out.append(
            f"{w.get('kind')} {w.get('name')}: wall {seconds(w.get('wall_s'))} s, user wait {seconds(w.get('user_wait_s'))} s, "
            f"helper wait {seconds(w.get('helper_wait_s'))} s, gaps {seconds(w.get('gap_s'))} s"
        )
        out.extend(step_table(w, wall, dominant))
        out.append("")

    share = gap_share(run)
    threshold = run.total("gap_threshold_s")
    out.append(
        f"Totals: wall {seconds(wall)} s, user wait {seconds(run.total('user_wait_s'))} s, "
        f"helper wait {seconds(run.total('helper_wait_s'))} s, "
        f"gaps over {seconds(threshold)} s: {seconds(run.total('gap_s'))} s ({number(run.total('gaps'))})"
        + ("" if share is None else f", {share:.0%} of wall")
        + (GAP_MARK if is_gappy(run) else "")
    )
    sub = run.record.get("subagents") or {}
    durations = sub.get("durations_s")
    out.append(
        f"Helpers: started {number(sub.get('started'))}, completed {number(sub.get('completed'))}, "
        f"failed {number(sub.get('failed'))}, unfinished {number(sub.get('unfinished'))}; durations (s): "
        + (UNKNOWN if durations is None else (", ".join(f"{d:.1f}" for d in durations) or "none"))
    )
    out.append("")
    out.extend(helpers_table(run))
    unknowns = run.record.get("unknown") or []
    if unknowns:
        out.append("")
        out.append("Unknown:")
        out.extend(f"  {u.get('field')}: {u.get('reason')}" for u in unknowns)
    out.append("")
    return "\n".join(out)


# Two trials side by side.

STEP_FIGURES = [("secs", "duration_s"), ("work", "working_s"), ("helper", "helper_wait_s"), ("user", "user_wait_s"), ("helpers", "helpers")]


def difference(a, b, fmt) -> str:
    if a is None or b is None:
        return UNKNOWN
    return f"{'+' if b >= a else '-'}{fmt(abs(b - a))}"


def side(step: dict | None, field: str, fmt) -> str:
    """A step's figure: `-` when the trial never entered the step, `unknown`
    when its record does not hold the figure."""
    if step is None:
        return "-"
    value = step.get(field)
    return UNKNOWN if value is None else fmt(value)


def pair_workflows(a: list[dict], b: list[dict]) -> list[tuple[dict | None, dict | None]]:
    """The two trials' workflows side by side: the same kind and name first,
    then, in order, the rest of each kind (a suite's generated spec names
    differ from trial to trial), then whatever is left alone."""
    pairs, rest_b = [], list(b)
    unmatched_a = []
    for w in a:
        twin = next((x for x in rest_b if (x.get("kind"), x.get("name")) == (w.get("kind"), w.get("name"))), None)
        if twin is not None:
            rest_b.remove(twin)
            pairs.append((w, twin))
        else:
            unmatched_a.append(w)
    for w in unmatched_a:
        twin = next((x for x in rest_b if x.get("kind") == w.get("kind")), None)
        if twin is not None:
            rest_b.remove(twin)
        pairs.append((w, twin))
    pairs += [(None, x) for x in rest_b]
    return pairs


def compare_trials(a: Run, b: Run) -> str:
    """Each workflow's per-step split for both trials, with B's difference
    from A, then the totals the same way. A step only one trial entered
    shows `-` on the other side, never 0."""
    def fmt_of(key):
        return (lambda v: f"{v:,.0f}") if key == "helpers" else (lambda v: f"{v:.1f}")

    out = [f"== {a.trial} (A) against {b.trial} (B) =="]
    unknown_steps = [r.trial for r in (a, b) if (r.record.get("steps") or {}).get("workflows") is None]
    if unknown_steps:
        out.append("Steps: unknown in " + " and ".join(unknown_steps))
    headers = ["step"] + [f"{label} {x}" for label, _ in STEP_FIGURES for x in ("A", "B", "B-A")]
    for wa, wb in pair_workflows(workflows(a), workflows(b)):
        out.append("")
        name_a = f"{wa.get('kind')} {wa.get('name')}" if wa else None
        name_b = f"{wb.get('kind')} {wb.get('name')}" if wb else None
        if name_a and name_b:
            out.append(name_a if name_a == name_b else f"{name_a} (A) / {name_b} (B)")
        else:
            out.append(f"{name_a or name_b} (in {'A' if wa else 'B'} only)")
        sa = {s.get("step"): s for s in (wa or {}).get("steps") or []}
        sb = {s.get("step"): s for s in (wb or {}).get("steps") or []}
        rows = []
        for step in list(sa) + [s for s in sb if s not in sa]:
            row = [step]
            for _, field in STEP_FIGURES:
                fmt = fmt_of(field)
                va = sa[step].get(field) if step in sa else None
                vb = sb[step].get(field) if step in sb else None
                both = step in sa and step in sb
                row += [side(sa.get(step), field, fmt), side(sb.get(step), field, fmt), difference(va, vb, fmt) if both else "-"]
            rows.append(row)
        out.extend("  " + line for line in render(headers, rows, right=set(range(1, len(headers)))))
    out.append("")
    out.append("Totals")
    rows = []
    for label, value, fmt in [
        ("wall s", lambda r: r.total("wall_time_s"), lambda v: f"{v:.1f}"),
        ("user wait s", lambda r: r.total("user_wait_s"), lambda v: f"{v:.1f}"),
        ("helper wait s", lambda r: r.total("helper_wait_s"), lambda v: f"{v:.1f}"),
        ("gap s", lambda r: r.total("gap_s"), lambda v: f"{v:.1f}"),
        ("cost $", lambda r: r.total("cost_usd"), lambda v: f"{v:.2f}"),
        ("total tok", total_tokens, lambda v: f"{v:,.0f}"),
    ]:
        va, vb = value(a), value(b)
        rows.append([label, UNKNOWN if va is None else fmt(va), UNKNOWN if vb is None else fmt(vb), UNKNOWN if va is None or vb is None else difference(va, vb, fmt)])
    out.extend("  " + line for line in render(["total", "A", "B", "B-A"], rows, right={1, 2, 3}))
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
    Measure("user wait m", lambda r: r.total("user_wait_s"), lambda v: f"{v / 60:.1f}"),
    Measure("helper wait m", lambda r: r.total("helper_wait_s"), lambda v: f"{v / 60:.1f}"),
    Measure("gap m", lambda r: r.total("gap_s"), lambda v: f"{v / 60:.1f}"),
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
    parser.add_argument("--prompts", action="store_true", help="with --run, also print each helper's brief")
    parser.add_argument("--compare", nargs=2, metavar=("A", "B"), help="two trial directories or names, side by side")
    parser.add_argument("--baseline", nargs="+", metavar="DIR", help="job directories or globs: the runs before a change")
    parser.add_argument("--candidate", nargs="+", metavar="DIR", help="job directories or globs: the runs after it")
    args = parser.parse_args(argv)

    if args.prompts and not args.run:
        print("report: --prompts prints one run's helper briefs, so it needs --run <trial>", file=sys.stderr)
        return 2
    if args.compare:
        if args.run or args.baseline or args.candidate:
            print("report: --compare takes two trials and no --run, --baseline or --candidate", file=sys.stderr)
            return 2
        paths = [*args.paths] + [t for t in args.compare if Path(t).is_dir()]
        runs = load_runs(paths or [str(DEFAULT_JOBS)])
        try:
            a, b = (find_run(runs, t) for t in args.compare)
        except LookupError as exc:
            print(f"report: {exc}", file=sys.stderr)
            return 2
        print(compare_trials(a, b))
        return 0
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
