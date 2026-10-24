"""Summarise harbor runs: score, pass rate, pass^k, cost, turns, time, tokens.

Harbor reports one reward per trial and leaves cost and tokens null for some
agents, though the agent's own log holds them. This reads every trial under
the job directories it is given and prints, for each suite, agent and model:

    runs        trials found
    pass        fraction of runs where every check passed
    pass^k      chance that k runs in a row all pass, estimated from the runs
                (unbiased: C(passes, k) / C(runs, k)); blank when runs < k
    score       mean fraction of checks passed (reward from reward.json)
    cost        mean USD per run, from the agent log
    turns       mean agent turns per run
    minutes     mean agent wall time per run
    in/out tok  mean input and output tokens per run (input includes cache)

then the mean score of each check group. A trial that crashed before its
verifier ran counts as a failed run with a score of 0.

Usage:

    python3 tests/harbor/scoring/report.py tests/harbor/jobs/<job> [...]
    python3 tests/harbor/scoring/report.py --k 3 --json tests/harbor/jobs

A directory may be one job or any directory above several; every trial
result.json beneath it is read.

Cost, turns and tokens come from:

    Claude  agent/claude-code.txt, the stream-json "result" event
            (total_cost_usd, num_turns, usage)
    Codex   agent/codex.txt, summing the usage of each "turn.completed"
            event; Codex reports no cost, and a Codex "turn" is one prompt
            round, not one model call, so turns are left blank

and fall back to the agent_result harbor recorded when the log has none. Wall
time is harbor's agent_execution timing, which every agent has.
"""

import argparse
import json
import math
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path


@dataclass
class Usage:
    cost_usd: float | None = None
    turns: int | None = None
    input_tokens: int | None = None
    output_tokens: int | None = None


@dataclass
class Trial:
    task: str
    agent: str
    model: str
    rewards: dict[str, float]
    passed: bool
    crashed: bool
    usage: Usage
    minutes: float | None
    path: Path


@dataclass
class Summary:
    task: str
    agent: str
    model: str
    trials: list[Trial] = field(default_factory=list)


def json_lines(path: Path):
    """Each JSON object in a log of one event per line, skipping the rest."""
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(event, dict):
            yield event


def claude_usage(log: Path) -> Usage:
    """Usage from Claude Code's stream-json result event; the last one wins."""
    usage = Usage()
    for event in json_lines(log):
        if event.get("type") != "result":
            continue
        tokens = event.get("usage") or {}
        usage = Usage(
            cost_usd=event.get("total_cost_usd"),
            turns=event.get("num_turns"),
            input_tokens=sum(
                tokens.get(key) or 0
                for key in (
                    "input_tokens",
                    "cache_creation_input_tokens",
                    "cache_read_input_tokens",
                )
            )
            if tokens
            else None,
            output_tokens=tokens.get("output_tokens") if tokens else None,
        )
    return usage


def codex_usage(log: Path) -> Usage:
    """Usage summed over `codex exec --json` turn.completed events.

    Codex's input_tokens already include its cached_input_tokens.
    """
    usage = Usage()
    for event in json_lines(log):
        if event.get("type") != "turn.completed":
            continue
        tokens = event.get("usage") or {}
        usage.input_tokens = (usage.input_tokens or 0) + (tokens.get("input_tokens") or 0)
        usage.output_tokens = (usage.output_tokens or 0) + (tokens.get("output_tokens") or 0)
    return usage


def agent_usage(trial_dir: Path, agent_result: dict | None) -> Usage:
    agent_dir = trial_dir / "agent"
    if (agent_dir / "claude-code.txt").exists():
        usage = claude_usage(agent_dir / "claude-code.txt")
    elif (agent_dir / "codex.txt").exists():
        usage = codex_usage(agent_dir / "codex.txt")
    else:
        usage = Usage()

    # Harbor's own figures, for whatever the log did not provide.
    recorded = agent_result or {}
    if usage.cost_usd is None:
        usage.cost_usd = recorded.get("cost_usd")
    if usage.input_tokens is None and recorded.get("n_input_tokens") is not None:
        usage.input_tokens = recorded["n_input_tokens"] + (recorded.get("n_cache_tokens") or 0)
    if usage.output_tokens is None:
        usage.output_tokens = recorded.get("n_output_tokens")
    return usage


def minutes_between(timing: dict | None) -> float | None:
    if not timing or not timing.get("started_at") or not timing.get("finished_at"):
        return None
    started = datetime.fromisoformat(timing["started_at"])
    finished = datetime.fromisoformat(timing["finished_at"])
    return (finished - started).total_seconds() / 60


def load_trial(result_path: Path) -> Trial | None:
    """A trial from its result.json, or None for a job-level result.json."""
    try:
        result = json.loads(result_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None
    if not isinstance(result, dict) or "trial_name" not in result:
        return None

    agent_info = result.get("agent_info") or {}
    model_info = agent_info.get("model_info") or {}
    rewards = (result.get("verifier_result") or {}).get("rewards") or {}
    rewards = {k: float(v) for k, v in rewards.items()}
    crashed = not rewards
    reward = rewards.get("reward", 0.0)
    # Runs from before reward.json carry only the pass/fail reward.
    passed = rewards.get("all_passed", 1.0 if reward == 1.0 else 0.0) == 1.0
    rewards.setdefault("reward", 0.0)

    return Trial(
        task=result.get("task_name", "?"),
        agent=agent_info.get("name", "?"),
        model=model_info.get("name", "?"),
        rewards=rewards,
        passed=passed,
        crashed=crashed,
        usage=agent_usage(result_path.parent, result.get("agent_result")),
        minutes=minutes_between(result.get("agent_execution")),
        path=result_path.parent,
    )


def load_trials(job_dirs: list[Path]) -> list[Trial]:
    trials = []
    seen: set[Path] = set()
    for job_dir in job_dirs:
        for result_path in sorted(job_dir.rglob("result.json")):
            if result_path.resolve() in seen:
                continue
            seen.add(result_path.resolve())
            trial = load_trial(result_path)
            if trial is not None:
                trials.append(trial)
    return trials


def pass_hat_k(runs: int, passes: int, k: int) -> float | None:
    """Unbiased estimate of the chance that k fresh runs all pass."""
    if runs < k:
        return None
    return math.comb(passes, k) / math.comb(runs, k)


def mean(values) -> float | None:
    values = [v for v in values if v is not None]
    return sum(values) / len(values) if values else None


def summarise(trials: list[Trial]) -> list[Summary]:
    summaries: dict[tuple[str, str, str], Summary] = {}
    for trial in trials:
        key = (trial.task, trial.agent, trial.model)
        summaries.setdefault(key, Summary(*key)).trials.append(trial)
    return [summaries[key] for key in sorted(summaries)]


def summary_row(summary: Summary, k: int) -> dict:
    trials = summary.trials
    passes = sum(t.passed for t in trials)
    groups = sorted({g for t in trials for g in t.rewards if g.startswith("group/")})
    return {
        "task": summary.task,
        "agent": summary.agent,
        "model": summary.model,
        "runs": len(trials),
        "pass_rate": passes / len(trials),
        f"pass^{k}": pass_hat_k(len(trials), passes, k),
        "score": mean(t.rewards["reward"] for t in trials),
        "cost_usd": mean(t.usage.cost_usd for t in trials),
        "turns": mean(t.usage.turns for t in trials),
        "minutes": mean(t.minutes for t in trials),
        "input_tokens": mean(t.usage.input_tokens for t in trials),
        "output_tokens": mean(t.usage.output_tokens for t in trials),
        # A trial that crashed before its verifier ran scored 0 on every group,
        # as it did overall. One without the group predates reward.json, or
        # skipped every check in it, and is left out of that group's mean.
        "groups": {
            g.removeprefix("group/"): mean(0.0 if t.crashed else t.rewards.get(g) for t in trials)
            for g in groups
        },
    }


def fmt(value, spec: str) -> str:
    return "-" if value is None else format(value, spec)


def print_table(rows: list[dict], k: int) -> None:
    header = (
        f"{'suite':<34} {'agent':<12} {'model':<22} {'runs':>4} {'pass':>5} "
        f"{f'pass^{k}':>6} {'score':>5} {'cost':>7} {'turns':>5} {'min':>5} "
        f"{'in tok':>9} {'out tok':>8}"
    )
    print(header)
    print("-" * len(header))
    for row in rows:
        print(
            f"{row['task'][:34]:<34} {row['agent'][:12]:<12} {row['model'][:22]:<22} "
            f"{row['runs']:>4} {fmt(row['pass_rate'], '.2f'):>5} "
            f"{fmt(row[f'pass^{k}'], '.2f'):>6} {fmt(row['score'], '.3f'):>5} "
            f"{'-' if row['cost_usd'] is None else '$' + format(row['cost_usd'], '.2f'):>7} "
            f"{fmt(row['turns'], '.1f'):>5} {fmt(row['minutes'], '.1f'):>5} "
            f"{fmt(row['input_tokens'], ',.0f'):>9} {fmt(row['output_tokens'], ',.0f'):>8}"
        )
    for row in rows:
        if not row["groups"]:
            continue
        print()
        print(f"{row['task']} / {row['agent']} / {row['model']}: score by check group")
        for group, score in row["groups"].items():
            print(f"  {fmt(score, '.3f'):>5}  {group}")


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("job_dirs", nargs="+", type=Path, help="harbor job directories")
    parser.add_argument("--k", type=int, default=3, help="k for pass^k (default 3)")
    parser.add_argument("--json", action="store_true", help="print JSON instead of a table")
    args = parser.parse_args(argv)
    if args.k < 1:
        parser.error("--k must be at least 1")

    trials = load_trials(args.job_dirs)
    if not trials:
        print("no harbor trials found under: " + ", ".join(map(str, args.job_dirs)), file=sys.stderr)
        return 1
    rows = [summary_row(s, args.k) for s in summarise(trials)]
    if args.json:
        json.dump(rows, sys.stdout, indent=2)
        print()
    else:
        print_table(rows, args.k)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
