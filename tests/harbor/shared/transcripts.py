"""Read an agent's own record of a harbor run: tool calls and sub-agents.

Claude Code writes a stream-json transcript, one JSON object per line, in
which a sub-agent's messages carry the `parent_tool_use_id` of the Agent (or
Task) tool call that launched it, and every assistant message names its
model. Agents run through harbor's ACP agent, such as Bob, leave a trajectory
of tool calls instead, where `Running subagent: <prompt>` launches a
sub-agent. Codex leaves no sub-agent signal at all.

They also read each agent's own summary of a run, for the metrics record:
Claude's `result` events, the ATIF trajectory harbor writes for ACP agents
and Codex, and Codex's native rollout as a fallback. Whatever a log does not
supply comes back as None with an Unknown naming the reason, never as zero.

The functions here are pure over the parsed records, so a verifier's
self-check can exercise them on synthetic input. Copied into each suite's
tests/ by the Makefile, like sessionlog.py.
"""

from __future__ import annotations

import json
import re
import shlex
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

AGENT_LOG_DIR = Path("/logs/agent")
CLAUDE_TRANSCRIPT = AGENT_LOG_DIR / "claude-code.txt"
ACP_TRAJECTORY = AGENT_LOG_DIR / "trajectory.json"
CODEX_OUTPUT = AGENT_LOG_DIR / "codex.txt"
CODEX_SESSIONS = AGENT_LOG_DIR / "sessions"

SUBAGENT_TOOLS = ("Agent", "Task")


@dataclass
class SubagentLaunch:
    """One sub-agent the main agent launched, and what it ran on."""

    tool_use_id: str
    subagent_type: str
    prompt: str
    models: set[str] = field(default_factory=set)  # models of its own messages
    returned: bool = False  # a tool_result for the launch came back


def read_jsonl(path: Path) -> list[dict]:
    if not path.exists():
        return []
    out = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(obj, dict):
            out.append(obj)
    return out


def claude_main_models(objs: list[dict]) -> set[str]:
    """The models of the main agent's own assistant messages."""
    return {
        (o.get("message") or {}).get("model", "")
        for o in objs
        if o.get("type") == "assistant" and not o.get("parent_tool_use_id")
    } - {""}


def claude_subagent_launches(objs: list[dict]) -> list[SubagentLaunch]:
    """Every sub-agent the main agent launched, in transcript order, with the
    models its messages ran on and whether its result came back."""
    launches: dict[str, SubagentLaunch] = {}
    order: list[str] = []
    for o in objs:
        msg = o.get("message") or {}
        parent = o.get("parent_tool_use_id")
        if o.get("type") == "assistant" and not parent:
            for block in msg.get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_use" and block.get("name") in SUBAGENT_TOOLS:
                    inp = block.get("input") or {}
                    tid = block.get("id", "")
                    launches[tid] = SubagentLaunch(
                        tool_use_id=tid,
                        subagent_type=inp.get("subagent_type", "") or "",
                        prompt=inp.get("prompt", "") or "",
                    )
                    order.append(tid)
        if o.get("type") == "assistant" and parent in launches and msg.get("model"):
            launches[parent].models.add(msg["model"])
        if o.get("type") == "user" and not parent:
            for block in msg.get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_result" and block.get("tool_use_id") in launches:
                    launches[block["tool_use_id"]].returned = True
    return [launches[t] for t in order]


def acp_steps(path: Path = ACP_TRAJECTORY) -> list[dict]:
    try:
        return json.loads(path.read_text()).get("steps") or []
    except (OSError, json.JSONDecodeError, AttributeError):
        return []


def acp_subagent_launches(steps: list[dict]) -> list[SubagentLaunch]:
    """Every `Running subagent` call in an ACP trajectory. The prompt is the
    tool call's arguments as text, since ACP agents name it differently."""
    out = []
    for step in steps:
        for tc in step.get("tool_calls") or []:
            name = tc.get("function_name", "") or ""
            if not name.startswith("Running subagent"):
                continue
            args = tc.get("arguments") or {}
            prompt = name.removeprefix("Running subagent").lstrip(": ") + "\n" + json.dumps(args)
            out.append(SubagentLaunch(tool_use_id=tc.get("tool_call_id", "") or "", subagent_type="", prompt=prompt, returned=True))
    return out


# Run summaries, for the metrics record.


@dataclass
class Unknown:
    """A measure a log could not supply, by its dotted path in the record."""

    field: str
    reason: str


@dataclass
class Totals:
    wall_time_s: float | None = None
    model_time_s: float | None = None
    turns: int | None = None
    cost_usd: float | None = None
    input_tokens: int | None = None
    output_tokens: int | None = None
    cache_read_tokens: int | None = None
    cache_write_tokens: int | None = None
    model: str | None = None
    unknowns: list[Unknown] = field(default_factory=list)


TOTAL_FIELDS = (
    "wall_time_s",
    "model_time_s",
    "turns",
    "cost_usd",
    "input_tokens",
    "output_tokens",
    "cache_read_tokens",
    "cache_write_tokens",
)


@dataclass
class Message:
    """One assistant message: when it was written and how much it wrote."""

    at: datetime | None  # None when the log has no timestamps (Bob)
    chars: int
    parent: str | None  # the launching tool call, for a Claude sub-agent's message
    order: int
    goto_step: str | None = None  # the step a `spektacular ... goto` in it moves to


@dataclass
class Subagents:
    started: int | None = None
    completed: int | None = None
    failed: int | None = None
    stopped: int | None = None
    durations_s: list[float] | None = None
    unknowns: list[Unknown] = field(default_factory=list)


def parse_timestamp(value: str | None) -> datetime | None:
    if not isinstance(value, str) or not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _span_s(times: list[datetime]) -> float | None:
    if len(times) < 2:
        return None
    return (max(times) - min(times)).total_seconds()


def _mark_unknown(totals: Totals, reason: str, *names: str) -> None:
    for name in names:
        if getattr(totals, name) is None:
            totals.unknowns.append(Unknown(f"totals.{name}", reason))


_STEP_IN_DATA = re.compile(r'"step"\s*:\s*"(?P<step>[^"]+)"')


def goto_step_of(command: str) -> str | None:
    """The step a shell command's `spektacular <kind> goto --data` moves to."""
    if not isinstance(command, str) or "spektacular" not in command or "goto" not in command:
        return None
    try:
        args = shlex.split(command)
    except ValueError:
        args = []
    for i, arg in enumerate(args):
        if arg != "goto":
            continue
        rest = args[i + 1 :]
        for j, a in enumerate(rest):
            raw = None
            if a in ("--data", "-d") and j + 1 < len(rest):
                raw = rest[j + 1]
            elif a.startswith("--data="):
                raw = a.removeprefix("--data=")
            if raw is None:
                continue
            try:
                data = json.loads(raw)
            except json.JSONDecodeError:
                break
            step = data.get("step") if isinstance(data, dict) else None
            return step if isinstance(step, str) else None
    m = _STEP_IN_DATA.search(command) if re.search(r"spektacular\s+\w+\s+goto\b", command) else None
    return m["step"] if m else None


def claude_summary(objs: list[dict]) -> Totals:
    """The run's totals from Claude's `result` events.

    With one result event, its `duration_ms` and `num_turns` are the run's.
    Background sub-agents split a run into several, each with a segment's
    `duration_ms` and `num_turns`, while cost, tokens, model time and
    sub-agent stats are cumulative. Then turns are summed, wall time is the
    transcript's first-to-last timestamp, and the rest comes from the last.
    """
    results = [o for o in objs if o.get("type") == "result"]
    init_model = next(
        (o.get("model") for o in objs if o.get("type") == "system" and o.get("subtype") == "init" and o.get("model")),
        None,
    )
    totals = Totals(model=init_model)
    if not results:
        _mark_unknown(totals, "no result event in claude-code.txt", *TOTAL_FIELDS)
        if totals.model is None:
            totals.unknowns.append(Unknown("run.model", "no result or init event in claude-code.txt"))
        return totals

    last = results[-1]
    if len(results) == 1:
        if isinstance(last.get("duration_ms"), (int, float)):
            totals.wall_time_s = last["duration_ms"] / 1000
        totals.turns = last.get("num_turns")
    else:
        totals.wall_time_s = _span_s([t for t in (parse_timestamp(o.get("timestamp")) for o in objs) if t])
        turns = [r.get("num_turns") for r in results]
        totals.turns = sum(turns) if all(isinstance(t, int) for t in turns) else None
    if isinstance(last.get("duration_api_ms"), (int, float)):
        totals.model_time_s = last["duration_api_ms"] / 1000
    totals.cost_usd = last.get("total_cost_usd")

    usage = last.get("modelUsage")
    if isinstance(usage, dict) and usage:
        def total(key: str) -> int | None:
            values = [u.get(key) for u in usage.values() if isinstance(u, dict)]
            return sum(values) if values and all(isinstance(v, int) for v in values) else None

        totals.input_tokens = total("inputTokens")
        totals.output_tokens = total("outputTokens")
        totals.cache_read_tokens = total("cacheReadInputTokens")
        totals.cache_write_tokens = total("cacheCreationInputTokens")
        totals.model = "+".join(usage)
    _mark_unknown(totals, "missing from the last result event in claude-code.txt", *TOTAL_FIELDS)
    if totals.model is None:
        totals.unknowns.append(Unknown("run.model", "claude-code.txt names no model"))
    return totals


def _block_chars(block: dict) -> int:
    kind = block.get("type")
    if kind == "text":
        return len(block.get("text") or "")
    if kind == "thinking":
        return len(block.get("thinking") or "")
    if kind == "tool_use":
        return len(json.dumps(block.get("input") or {}))
    return 0


def claude_messages(objs: list[dict]) -> list[Message]:
    """Every assistant message, main agent and sub-agent alike, with the
    characters it wrote: text, thinking and tool-call input."""
    out = []
    for o in objs:
        if o.get("type") != "assistant":
            continue
        content = (o.get("message") or {}).get("content") or []
        blocks = [b for b in content if isinstance(b, dict)]
        goto = None
        for b in blocks:
            if b.get("type") == "tool_use":
                goto = goto_step_of((b.get("input") or {}).get("command", "")) or goto
        out.append(
            Message(
                at=parse_timestamp(o.get("timestamp")),
                chars=sum(_block_chars(b) for b in blocks),
                parent=o.get("parent_tool_use_id") or None,
                order=len(out),
                goto_step=goto,
            )
        )
    return out


def claude_subagents(objs: list[dict]) -> Subagents:
    """Sub-agent counts from the last result's `subagent_stats`, and each
    sub-agent's duration from its `task_notification`."""
    notes = [o for o in objs if o.get("type") == "system" and o.get("subtype") == "task_notification"]
    durations = [
        n["usage"]["duration_ms"] / 1000
        for n in notes
        if isinstance(n.get("usage"), dict) and isinstance(n["usage"].get("duration_ms"), (int, float))
    ]
    results = [o for o in objs if o.get("type") == "result"]
    stats = (results[-1].get("subagent_stats") if results else None) or None
    out = Subagents(durations_s=durations)
    if isinstance(stats, dict):
        killed = stats.get("killed") or {}
        out.started = stats.get("spawned")
        out.completed = stats.get("completed")
        out.failed = stats.get("failed")
        out.stopped = sum(v for v in killed.values() if isinstance(v, int)) if isinstance(killed, dict) else None
    else:
        statuses = [n.get("status") for n in notes]
        out.started = len(claude_subagent_launches(objs))
        out.completed = statuses.count("completed")
        out.failed = statuses.count("failed")
        out.stopped = len(statuses) - out.completed - out.failed
    for name in ("started", "completed", "failed", "stopped"):
        if getattr(out, name) is None:
            out.unknowns.append(Unknown(f"subagents.{name}", "missing from subagent_stats in claude-code.txt"))
    missing = (out.started or 0) - len(durations)
    if missing > 0:
        out.unknowns.append(Unknown("subagents.durations_s", f"{missing} sub-agents never reported a duration"))
    return out


def claude_launch_root(objs: list[dict]) -> dict[str, str]:
    """Maps every sub-agent message's `parent_tool_use_id` to the main
    agent's launch it descends from, walking up nested launches."""
    parent_of: dict[str, str] = {}
    for o in objs:
        parent = o.get("parent_tool_use_id")
        if o.get("type") != "assistant" or not parent:
            continue
        for b in (o.get("message") or {}).get("content") or []:
            if isinstance(b, dict) and b.get("type") == "tool_use" and b.get("name") in SUBAGENT_TOOLS and b.get("id"):
                parent_of[b["id"]] = parent

    def root(tid: str) -> str:
        seen = set()
        while tid in parent_of and tid not in seen:
            seen.add(tid)
            tid = parent_of[tid]
        return tid

    return {p: root(p) for p in {o.get("parent_tool_use_id") for o in objs} if p}


def atif_agent_name(traj: dict) -> str:
    return ((traj.get("agent") or {}).get("name") or "agent").lower()


def _atif_text(message) -> str:
    if isinstance(message, str):
        return message
    if isinstance(message, list):
        return "".join(p.get("text") or "" for p in message if isinstance(p, dict))
    return ""


def atif_summary(traj: dict) -> Totals:
    """Totals from an ATIF trajectory: Bob through harbor's ACP agent, or
    Codex through harbor's rollout conversion.

    Turns are the agent's steps. ATIF's prompt tokens include cached ones,
    so the uncached input is the difference, as Claude reports it. Bob's
    trajectory has no timestamps, tokens or cost; its wall time is left for
    the recorder to fill from another log.
    """
    name = atif_agent_name(traj)
    steps = [s for s in traj.get("steps") or [] if isinstance(s, dict)]
    final = traj.get("final_metrics") or {}
    totals = Totals(turns=sum(1 for s in steps if s.get("source") == "agent"))
    totals.wall_time_s = _span_s([t for t in (parse_timestamp(s.get("timestamp")) for s in steps) if t])
    prompt, cached = final.get("total_prompt_tokens"), final.get("total_cached_tokens")
    totals.output_tokens = final.get("total_completion_tokens")
    totals.cache_read_tokens = cached
    if isinstance(prompt, int):
        totals.input_tokens = prompt - (cached if isinstance(cached, int) else 0)
    totals.cost_usd = final.get("total_cost_usd")
    totals.model = (traj.get("agent") or {}).get("model_name") or next(
        (s.get("model_name") for s in steps if s.get("model_name")), None
    )
    if totals.wall_time_s is None:
        totals.unknowns.append(Unknown("totals.wall_time_s", f"{name} trajectory has no step timestamps"))
    _mark_unknown(totals, f"{name} reports no model time", "model_time_s")
    _mark_unknown(totals, f"{name} reports no cost", "cost_usd")
    _mark_unknown(totals, f"{name} reports no token counts", "input_tokens", "output_tokens", "cache_read_tokens")
    _mark_unknown(totals, f"{name} reports no cache writes", "cache_write_tokens")
    if totals.model is None:
        totals.unknowns.append(Unknown("run.model", f"{name} reports no model"))
    return totals


def atif_messages(traj: dict) -> list[Message]:
    """One message per agent step: its text, reasoning and tool-call
    arguments in characters. `at` is None where the step has no timestamp."""
    out = []
    for order, step in enumerate(s for s in traj.get("steps") or [] if isinstance(s, dict)):
        if step.get("source") != "agent":
            continue
        chars = len(_atif_text(step.get("message"))) + len(step.get("reasoning_content") or "")
        goto = None
        for tc in step.get("tool_calls") or []:
            args = tc.get("arguments") or {}
            chars += len(json.dumps(args))
            if isinstance(args, dict):
                goto = goto_step_of(args.get("command", "")) or goto
        out.append(Message(at=parse_timestamp(step.get("timestamp")), chars=chars, parent=None, order=order, goto_step=goto))
    return out


def atif_subagents(traj: dict) -> Subagents:
    """What an ATIF trajectory says about sub-agents. Codex leaves no
    sub-agent signal; an ACP agent names launches but not their outcomes."""
    name = atif_agent_name(traj)
    out = Subagents()
    if name == "codex":
        for f in ("started", "completed", "failed", "stopped", "durations_s"):
            out.unknowns.append(Unknown(f"subagents.{f}", "codex reports no sub-agents"))
        return out
    out.started = len(acp_subagent_launches(traj.get("steps") or []))
    for f in ("completed", "failed", "stopped", "durations_s"):
        out.unknowns.append(Unknown(f"subagents.{f}", f"{name} reports no sub-agent outcomes"))
    return out


def codex_rollout_summary(sessions_dir: Path = CODEX_SESSIONS) -> Totals:
    """Totals from Codex's newest native rollout, for when harbor left no
    trajectory. Each `token_count` event carrying usage is one model call;
    Codex also sends them with no usage, for rate-limit updates."""
    rollouts = sorted(Path(sessions_dir).rglob("rollout-*.jsonl"), key=lambda p: p.name)
    totals = Totals()
    if not rollouts:
        _mark_unknown(totals, "no codex rollout under sessions/", *TOTAL_FIELDS)
        totals.unknowns.append(Unknown("run.model", "no codex rollout under sessions/"))
        return totals
    lines = read_jsonl(rollouts[-1])
    counts = [
        line["payload"]
        for line in lines
        if line.get("type") == "event_msg"
        and (line.get("payload") or {}).get("type") == "token_count"
        and isinstance(line["payload"].get("info"), dict)
    ]
    totals.turns = len(counts)
    totals.wall_time_s = _span_s([t for t in (parse_timestamp(line.get("timestamp")) for line in lines) if t])
    usage = next(
        (c["info"]["total_token_usage"] for c in reversed(counts) if isinstance((c.get("info") or {}).get("total_token_usage"), dict)),
        {},
    )
    cached = usage.get("cached_input_tokens")
    if isinstance(usage.get("input_tokens"), int):
        totals.input_tokens = usage["input_tokens"] - (cached if isinstance(cached, int) else 0)
    totals.output_tokens = usage.get("output_tokens")
    totals.cache_read_tokens = cached
    totals.model = next(
        (line["payload"]["model"] for line in reversed(lines) if line.get("type") == "turn_context" and (line.get("payload") or {}).get("model")),
        None,
    )
    _mark_unknown(totals, "codex reports no cost", "cost_usd")
    _mark_unknown(totals, "codex reports no model time", "model_time_s")
    _mark_unknown(totals, "codex reports no cache writes", "cache_write_tokens")
    _mark_unknown(totals, "no token_count event in the codex rollout", "input_tokens", "output_tokens", "cache_read_tokens")
    _mark_unknown(totals, "codex rollout has fewer than two timestamps", "wall_time_s")
    if totals.model is None:
        totals.unknowns.append(Unknown("run.model", "codex rollout names no model"))
    return totals
