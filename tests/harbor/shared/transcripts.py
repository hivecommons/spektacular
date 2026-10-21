"""Read an agent's own record of a harbor run: tool calls and sub-agents.

Claude Code writes a stream-json transcript, one JSON object per line, in
which a sub-agent's messages carry the `parent_tool_use_id` of the Agent (or
Task) tool call that launched it, and every assistant message names its
model. Agents run through harbor's ACP agent, such as Bob, leave a trajectory
of tool calls instead, where `Running subagent: <prompt>` launches a
sub-agent. Codex leaves no sub-agent signal at all.

The functions here are pure over the parsed records, so a verifier's
self-check can exercise them on synthetic input. Copied into each suite's
tests/ by the Makefile, like sessionlog.py.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

AGENT_LOG_DIR = Path("/logs/agent")
CLAUDE_TRANSCRIPT = AGENT_LOG_DIR / "claude-code.txt"
ACP_TRAJECTORY = AGENT_LOG_DIR / "trajectory.json"

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
