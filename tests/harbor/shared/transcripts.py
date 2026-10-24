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
The same goes for the child agents a run started, at any depth, and for the
main agent's turns and the timed wake-ups it set itself.

The functions here are pure over the parsed records, so a verifier's
self-check can exercise them on synthetic input. Copied into each suite's
tests/ by the Makefile, like sessionlog.py.
"""

from __future__ import annotations

import json
import re
import shlex
from dataclasses import dataclass, field
from datetime import datetime, timezone
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


@dataclass
class Child:
    """One child agent a run started, at any depth, before it is placed in a
    spec and step. None is a detail the log could not supply."""

    id: str  # the launching tool call's id
    parent: str | None  # the id of the child that started it; None for the main agent
    kind: str | None
    description: str | None
    background: bool | None  # as actually run, not as requested
    launched_at: datetime | None
    ended_at: datetime | None
    duration_s: float | None  # the agent's own report of the child's run time
    ending: str | None  # completed | failed | stopped | no_hand_back
    hand_back: str | None  # the first non-empty line of what it handed back
    models: list[str] | None
    output: int | None  # characters of its own messages
    prompt: str | None
    order: int | None = None  # its launching step's index, for a log without times (ATIF)


@dataclass
class Turn:
    """One turn of the main agent: when it ended, and what started it."""

    ended_at: datetime | None
    woken_by: str | None  # "start" | "task-notification" | "wakeup"; None when the log does not say


@dataclass
class Wakeup:
    """A timed wake-up the main agent set itself: a scheduled wake-up, a
    recurring job, or a shell command that sleeps."""

    at: datetime | None
    tool: str
    detail: str


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


# Child agents, turns and wake-ups, for the metrics record and the delegation
# rules. Claude's system events carry no timestamp: a child's launch time is
# its launching message's, and its end is `task_updated`'s epoch-ms end time.

CHILD_ENDINGS = {"completed": "completed", "failed": "failed", "killed": "stopped", "stopped": "stopped"}
HAND_BACK_FRAME = re.compile(r"\A\[Subagent hand-back\].*?The report follows:\n", re.S)
SLEEP_COMMAND = re.compile(r"(?:^|[;&|(\s])sleep\s")


def _system(objs: list[dict], subtype: str) -> list[dict]:
    return [o for o in objs if o.get("type") == "system" and o.get("subtype") == subtype]


def _epoch_ms(value) -> datetime | None:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return None
    return datetime.fromtimestamp(value / 1000, tz=timezone.utc)


def _first_line(text) -> str | None:
    if not isinstance(text, str):
        return None
    return next((line.strip() for line in text.splitlines() if line.strip()), None)


def _result_text(block: dict) -> str:
    content = block.get("content")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(p.get("text") or "" for p in content if isinstance(p, dict))
    return ""


def _unframed(text: str) -> str:
    """A foreground child's report without the harness's hand-back frame and
    the two-space indent it puts on every line."""
    body = HAND_BACK_FRAME.sub("", text, count=1)
    if body == text:
        return text
    return "\n".join(line.removeprefix("  ") for line in body.splitlines())


def claude_children(objs: list[dict]) -> tuple[list[Child], list[Unknown]]:
    """Every child agent the run started, nested ones included, in launch order.

    A child is its launching Agent (or Task) call, whose input gives its
    context. `task_started`, matched by the call's id, makes it a child that
    actually started and gives its real background state; `task_updated`,
    matched by task id, its end; `task_notification` its ending, its own
    run time and its hand-back. A launch the agent refused never starts, and
    a background shell task is not a child, so neither is counted. Children
    started by children stream no messages of their own, which leaves their
    models and output unknown.
    """
    launches: dict[str, tuple[dict, dict]] = {}  # id -> (tool_use block, enclosing object)
    results: dict[str, tuple[dict, dict]] = {}  # id -> (tool_result block, enclosing object)
    for o in objs:
        for block in (o.get("message") or {}).get("content") or []:
            if not isinstance(block, dict):
                continue
            if o.get("type") == "assistant" and block.get("type") == "tool_use" and block.get("name") in SUBAGENT_TOOLS:
                launches.setdefault(block.get("id", ""), (block, o))
            if o.get("type") == "user" and block.get("type") == "tool_result" and block.get("tool_use_id") in launches:
                results.setdefault(block["tool_use_id"], (block, o))

    started_events = _system(objs, "task_started")
    started = {
        s.get("tool_use_id"): s
        for s in started_events
        if s.get("tool_use_id") and s.get("task_type", "local_agent") == "local_agent"
    }
    updated = {u.get("task_id"): u.get("patch") or {} for u in _system(objs, "task_updated") if u.get("task_id")}
    notes = {n.get("tool_use_id"): n for n in _system(objs, "task_notification") if n.get("tool_use_id")}
    launch_of_task = {s.get("task_id"): tid for tid, s in started.items() if s.get("task_id")}

    messages: dict[str, list[Message]] = {}
    models: dict[str, set[str]] = {}
    for o, m in zip((o for o in objs if o.get("type") == "assistant"), claude_messages(objs)):
        if m.parent:
            messages.setdefault(m.parent, []).append(m)
            if (o.get("message") or {}).get("model"):
                models.setdefault(m.parent, set()).add(o["message"]["model"])

    # Without any task_started event the log cannot tell a started child from
    # a refused launch, so every launch the agent did not refuse counts.
    def did_start(tid: str) -> bool:
        if started_events:
            return tid in started
        res = results.get(tid)
        return not (res and res[0].get("is_error"))

    children: list[Child] = []
    unknowns: list[Unknown] = []
    if launches and not started_events:
        unknowns.append(Unknown("subagents.children.background", "claude logged no task_started events"))
    for tid, (block, o) in launches.items():
        if not tid or not did_start(tid):
            continue
        inp = block.get("input") or {}
        start = started.get(tid, {})
        note = notes.get(tid)
        res = results.get(tid)

        parent = o.get("parent_tool_use_id") or launch_of_task.get(start.get("parent_task_id")) or None
        background = start.get("is_backgrounded")
        if not isinstance(background, bool):
            background = inp.get("run_in_background") if isinstance(inp.get("run_in_background"), bool) else None
        patch = updated.get(start.get("task_id"), {})

        ending, duration, hand_back = "no_hand_back", None, None
        if note is not None:
            ending = CHILD_ENDINGS.get(note.get("status"), note.get("status"))
            usage = note.get("usage") if isinstance(note.get("usage"), dict) else {}
            duration = usage["duration_ms"] / 1000 if isinstance(usage.get("duration_ms"), (int, float)) else None
            # A stopped child hands nothing back: its notice repeats its description.
            hand_back = _first_line(note.get("summary")) if ending != "stopped" else None
        elif res is not None and not (res[1].get("tool_use_result") or {}).get("isAsync"):
            # A foreground child's report came back as its launch's result.
            status = (res[1].get("tool_use_result") or {}).get("status")
            ending = "failed" if res[0].get("is_error") else CHILD_ENDINGS.get(status, "completed")
            hand_back = _first_line(_unframed(_result_text(res[0])))

        own = messages.get(tid)
        child = Child(
            id=tid,
            parent=parent if parent in launches else None,
            kind=inp.get("subagent_type") or start.get("subagent_type") or "general-purpose",
            description=inp.get("description") or start.get("description"),
            background=background,
            launched_at=parse_timestamp(o.get("timestamp")),
            ended_at=_epoch_ms(patch.get("end_time")),
            duration_s=duration,
            ending=ending,
            hand_back=hand_back,
            models=sorted(models.get(tid, set())) if own else None,
            output=sum(m.chars for m in own) if own else None,
            prompt=inp.get("prompt") or start.get("prompt"),
        )
        children.append(child)

        gaps = [
            ("background", child.background is None and bool(started_events),
             "claude logged neither the child's background state nor a request for one"),
            ("ended_at", child.ended_at is None, "claude logged no end time for the child"),
            ("duration_s", child.duration_s is None,
             "claude reported no duration for a failed child" if ending == "failed" else "claude reported no duration for the child"),
            ("hand_back", child.hand_back is None and ending not in ("no_hand_back", "stopped"), "the child's hand-back was empty"),
            ("models", own is None, "claude streamed none of the child's messages"),
            ("output", own is None, "claude streamed none of the child's messages"),
            ("prompt", child.prompt is None, "the launch named no prompt"),
            ("description", child.description is None, "the launch named no description"),
        ]
        unknowns.extend(Unknown(f"subagents.children[{tid}].{name}", reason) for name, missing, reason in gaps if missing)
    return children, unknowns


def claude_turns(objs: list[dict]) -> list[Turn]:
    """One Turn per `result` event, in order. Each turn after the first opens
    with a `system/init`; a result can be written late, after the next turn's
    messages, so a turn is the stretch between inits, paired in order with
    its result. Without one init per result, a turn ends at its result.

    A turn opened by a child's completion notice carries `origin.kind`
    "task-notification" on its result; one opened by a timed wake-up follows a
    `command_lifecycle` with a `wakeup_source`. Results and inits carry no
    timestamp, so a turn ends at the main agent's last timestamped message
    in it: a child's messages may stream on after the main agent stopped."""
    results = [o for o in objs if o.get("type") == "result"]
    inits = [i for i, o in enumerate(objs) if o.get("type") == "system" and o.get("subtype") == "init"][1:]
    if len(inits) + 1 == len(results):
        bounds = [0, *inits, len(objs)]
    else:
        bounds = [0, *(i + 1 for i, o in enumerate(objs) if o.get("type") == "result")]
        bounds = bounds[: len(results) + 1]
    def segment_of(i: int) -> int:
        return next(k for k, b in enumerate(bounds[1:]) if i < b)

    # A wake-up wakes the turn holding the main agent's next message after it.
    main = [i for i, o in enumerate(objs) if o.get("timestamp") and not o.get("parent_tool_use_id")]
    woken_by_wakeup = {
        segment_of(next(j for j in main if j > i))
        for i, o in enumerate(objs)
        if o.get("type") == "command_lifecycle" and o.get("wakeup_source") and any(j > i for j in main) and i < bounds[-1]
    }
    turns: list[Turn] = []
    for k, (a, b) in enumerate(zip(bounds, bounds[1:])):
        segment = objs[a:b]
        times = [parse_timestamp(o.get("timestamp")) for o in segment if not o.get("parent_tool_use_id")]
        woken = "start" if k == 0 else "wakeup" if k in woken_by_wakeup else None
        origins = [results[k].get("origin")] + [o.get("origin") for o in segment if o.get("type") != "result"]
        if any(isinstance(g, dict) and g.get("kind") == "task-notification" for g in origins):
            woken = "task-notification"
        turns.append(Turn(ended_at=max((t for t in times if t), default=None), woken_by=woken))
    return turns


def claude_wakeups(objs: list[dict]) -> list[Wakeup]:
    """Every timed wake-up the main agent set: a ScheduleWakeup, a CronCreate,
    or a shell command that sleeps, in the foreground or the background."""
    out = []
    for o in objs:
        if o.get("type") != "assistant" or o.get("parent_tool_use_id"):
            continue
        for block in (o.get("message") or {}).get("content") or []:
            if not isinstance(block, dict) or block.get("type") != "tool_use":
                continue
            name, inp = block.get("name"), block.get("input") or {}
            if name == "ScheduleWakeup":
                detail = f"delaySeconds={inp.get('delaySeconds')}"
            elif name == "CronCreate":
                detail = str(inp.get("cron") or inp.get("schedule") or json.dumps(inp))[:80]
            elif name == "Bash" and SLEEP_COMMAND.search(inp.get("command") or ""):
                detail = inp["command"][:80]
            else:
                continue
            out.append(Wakeup(at=parse_timestamp(o.get("timestamp")), tool=name, detail=detail))
    return out


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


# What an ACP trajectory's `Running subagent` call cannot tell, by child field.
ACP_CHILD_GAPS = {
    "parent": "sub-agent parent",
    "background": "sub-agent background state",
    "launched_at": "sub-agent start time",
    "ended_at": "sub-agent end time",
    "duration_s": "sub-agent duration",
    "ending": "sub-agent outcome",
    "hand_back": "sub-agent hand-back",
    "models": "sub-agent models",
    "output": "sub-agent output",
}


def atif_children(traj: dict) -> tuple[list[Child] | None, list[Unknown]]:
    """Child agents from an ATIF trajectory. An ACP agent's `Running subagent:
    <description>` call names its kind (`arguments.name`) and full prompt
    (`arguments.description`, which the call's title truncates), and nothing
    else. Codex leaves no sub-agent signal, so its list is unknown."""
    name = atif_agent_name(traj)
    if name == "codex":
        return None, [Unknown("subagents.children", "codex reports no sub-agents")]
    children: list[Child] = []
    unknowns: list[Unknown] = []
    for order, step in enumerate(s for s in traj.get("steps") or [] if isinstance(s, dict)):
        for tc in step.get("tool_calls") or []:
            title = tc.get("function_name", "") or ""
            if not title.startswith("Running subagent"):
                continue
            args = tc.get("arguments") if isinstance(tc.get("arguments"), dict) else {}
            tid = tc.get("tool_call_id", "") or ""
            child = Child(
                id=tid, parent=None, kind=args.get("name") or None,
                description=title.removeprefix("Running subagent").lstrip(": ") or None,
                background=None, launched_at=None, ended_at=None, duration_s=None, ending=None,
                hand_back=None, models=None, output=None, prompt=args.get("description") or None, order=order,
            )
            children.append(child)
            for f, missing in (("kind", child.kind), ("description", child.description), ("prompt", child.prompt)):
                if missing is None:
                    unknowns.append(Unknown(f"subagents.children[{tid}].{f}", f"the {name} sub-agent call named no {f}"))
    if children:
        unknowns[:0] = [Unknown(f"subagents.children.{f}", f"{name} logs no {what}") for f, what in ACP_CHILD_GAPS.items()]
    return children, unknowns


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
