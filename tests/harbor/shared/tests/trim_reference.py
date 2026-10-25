#!/usr/bin/env python3
"""Trims a collected harbor trial into a compact reference run for the self-checks.

    python3 tests/harbor/shared/tests/trim_reference.py <trial-dir> <name> [--out <dir>]

writes tests/harbor/shared/tests/data/<name>/ (or <dir>), holding:

- claude-code.txt: every line of the agent's transcript, kept to its
  structure, timestamps, ids, models, turn and lifecycle events, and the full
  input of every child launch, wake-up and shell command. Every other piece of
  text is replaced by the same number of `x`, so the characters the recorder
  counts are unchanged and no conversation is checked in. A child's
  hand-back keeps only its first line, which the record reads;
- logs/: Spektacular's own logs, copied unchanged: recorded at the default
  level, they hold no response or conversation, only each command's argv,
  times, exit status and step, and the step, task and agent spans;
- build.json: the agent and suite, from the job's config.json;
- README: where the data came from.

It recomputes the metrics record from the trimmed copy and from the original,
and writes nothing if any value differs, so a trim can never change what the
self-checks prove. The trial can also be a reference run trimmed before,
which trims to the same bytes.

It lives under shared/tests/ so the Makefile, which copies shared/*.py into
each image, never ships it.
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))

import metrics  # noqa: E402
import transcripts as tx  # noqa: E402

DATA = HERE / "data"
KEEP_INPUT = {"Agent", "Task", "ScheduleWakeup", "CronCreate"}
KEEP_SHELL = ("command", "run_in_background")
KEEP_SYSTEM = {"task_started", "task_updated", "task_notification", "task_progress", "background_tasks_changed"}
BLANK_SYSTEM = {"task_notification": "summary", "task_progress": "description"}  # the child's own words
KEEP_INIT = ("type", "subtype", "model", "session_id", "uuid", "cwd", "claude_code_version")
KEEP_MESSAGE_OBJECT = ("type", "timestamp", "parent_tool_use_id", "uuid", "session_id", "origin", "agent_id")
KEEP_TOOL_USE_RESULT = ("isAsync", "status", "agentId", "totalDurationMs")


def blank(text: str) -> str:
    return "x" * len(text)


def keep_first_line(text: str) -> str:
    """Keeps a hand-back's harness frame and its first report line, the only
    part the record reads, and blanks the rest of the child's report."""
    frame = tx.HAND_BACK_FRAME.match(text)
    start = frame.end() if frame else 0
    lines = text[start:].split("\n")
    for i, line in enumerate(lines):
        if line.strip():
            head = text[:start] + "\n".join(lines[: i + 1])
            return head + re.sub(r"[^\n]", "x", text[len(head):])  # lines stay lines
    return text


def blank_json(value):
    """Blanks every string so json.dumps(value) keeps its length: an escaped
    character becomes as many `x` as its escape takes."""
    if isinstance(value, str):
        return "x" * (len(json.dumps(value)) - 2)
    if isinstance(value, list):
        return [blank_json(v) for v in value]
    if isinstance(value, dict):
        return {k: blank_json(v) for k, v in value.items()}
    return value


def trim_block(block: dict, hand_backs: set[str]) -> dict:
    kind = block.get("type")
    if kind == "text":
        return {"type": "text", "text": blank(block.get("text") or "")}
    if kind == "thinking":
        return {"type": "thinking", "thinking": blank(block.get("thinking") or "")}
    if kind == "tool_use":
        inp = block.get("input") or {}
        name = block.get("name")
        if name in KEEP_INPUT:
            kept = inp
        elif name == "Bash":
            kept = {k: (v if k in KEEP_SHELL else blank_json(v)) for k, v in inp.items()}
        else:
            kept = blank_json(inp)
        return {"type": "tool_use", "id": block.get("id"), "name": name, "input": kept}
    if kind == "tool_result":
        out = {"type": "tool_result", "tool_use_id": block.get("tool_use_id")}
        if block.get("is_error"):
            out["is_error"] = True
        # A child's report is kept only where it is the hand-back the record reads.
        text = tx._result_text(block)
        out["content"] = keep_first_line(text) if block.get("tool_use_id") in hand_backs else blank(text)
        return out
    return {"type": kind}


def trim_object(o: dict, hand_backs: set[str]) -> dict:
    kind = o.get("type")
    if kind == "system":
        if o.get("subtype") in KEEP_SYSTEM:
            key = BLANK_SYSTEM.get(o.get("subtype"))
            if key and isinstance(o.get(key), str):
                o = {**o, key: keep_first_line(o[key]) if key == "summary" else blank(o[key])}
            return o
        if o.get("subtype") == "init":
            return {k: o[k] for k in KEEP_INIT if k in o}
        return {k: o[k] for k in ("type", "subtype", "session_id", "uuid") if k in o}
    if kind == "result":
        return {**o, "result": ""}
    if kind == "command_lifecycle":
        return o
    if kind in ("assistant", "user"):
        out = {k: o[k] for k in KEEP_MESSAGE_OBJECT if k in o}
        msg = o.get("message") or {}
        content = msg.get("content")
        if isinstance(content, str):
            content = blank(content)
        else:
            content = [trim_block(b, hand_backs) for b in content or [] if isinstance(b, dict)]
        out["message"] = {k: msg[k] for k in ("model", "id", "role") if k in msg} | {"content": content}
        result = o.get("tool_use_result")
        if isinstance(result, dict):
            out["tool_use_result"] = {k: result[k] for k in KEEP_TOOL_USE_RESULT if k in result}
        return out
    return {k: o[k] for k in ("type", "subtype", "timestamp", "uuid", "session_id") if k in o}


def trim_transcript(objs: list[dict]) -> list[dict]:
    notified = {o.get("tool_use_id") for o in objs if o.get("type") == "system" and o.get("subtype") == "task_notification"}
    launches = {
        b.get("id")
        for o in objs
        if o.get("type") == "assistant"
        for b in (o.get("message") or {}).get("content") or []
        if isinstance(b, dict) and b.get("type") == "tool_use" and b.get("name") in tx.SUBAGENT_TOOLS
    }
    return [trim_object(o, launches - notified) for o in objs]


def layout(src: Path) -> tuple[Path, Path]:
    """The transcript's directory and Spektacular's logs of a collected
    trial, or of a reference run trimmed before."""
    if (src / "agent" / "claude-code.txt").is_file():
        return src / "agent", src / "artifacts" / "spektacular" / "logs"
    return src, src / "logs"


def build_info(src: Path) -> dict:
    if (src / "build.json").is_file():
        return json.loads((src / "build.json").read_text())
    config = json.loads((src / "config.json").read_text())
    suite = Path((config.get("task") or {}).get("path", "")).name.rsplit("-", 1)[0]
    agent = (config.get("agent") or {}).get("name", "").removesuffix("-code")
    return {"suite": suite, "agent": agent, "model": (config.get("agent") or {}).get("model_name")}


def write_jsonl(path: Path, rows: list[dict]) -> None:
    path.write_text("".join(json.dumps(r) + "\n" for r in rows))


def trim(src: Path, out: Path, source_note: str) -> None:
    agent_dir, log_dir = layout(src)
    out.mkdir(parents=True)
    write_jsonl(out / "claude-code.txt", trim_transcript(tx.read_jsonl(agent_dir / "claude-code.txt")))
    logs = sorted(log_dir.glob("*.jsonl")) if log_dir.is_dir() else []  # a run that never called spektacular has none
    if logs:
        (out / "logs").mkdir()
    for path in logs:
        shutil.copyfile(path, out / "logs" / path.name)
    (out / "build.json").write_text(json.dumps(build_info(src), indent=2) + "\n")
    readme = src / "README"
    (out / "README").write_text(readme.read_text() if readme.is_file() else source_note + "\n")


def measured(agent_dir: Path, log_dir: Path) -> dict:
    """The values a reference run proves: the metrics record (schema 3, its
    log sections read through `spektacular stats` over the copied logs) but
    the run's identity and its check score, which come from files the trim
    leaves out. A reason that names the logs directory names it as <logs>,
    so a trim into another folder compares equal."""
    none = Path("/nonexistent")
    record = metrics.build_record(
        metrics.Paths(build_info=none, agent_dir=agent_dir, log_dir=log_dir, bob_logs=none, junit=none, reward=none)
    )
    record["unknown"] = [{**u, "reason": u["reason"].replace(str(log_dir), "<logs>")} for u in record["unknown"]]
    return {k: v for k, v in record.items() if k not in ("run", "result")}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Trim a harbor trial into a checked-in reference run.")
    parser.add_argument("trial", type=Path)
    parser.add_argument("name")
    parser.add_argument("--out", type=Path, help="write here instead of tests/harbor/shared/tests/data/<name>")
    args = parser.parse_args(argv)
    src = args.trial.resolve()
    out = (args.out or DATA / args.name).resolve()
    if out.exists():
        print(f"trim: {out} already exists; remove it first", file=sys.stderr)
        return 1
    job = src.parent.name
    trial_dir = Path(tempfile.mkdtemp(prefix="trim-"))
    staged = trial_dir / args.name
    try:
        trim(src, staged, f"{args.name}: trimmed from harbor job {job}, trial {src.name}, by trim_reference.py")
        before = measured(*layout(src))
        after = measured(*layout(staged))
        if before != after:
            changed = sorted(k for k in before if before[k] != after.get(k))
            print(f"trim: the trimmed copy changes {', '.join(changed)}; nothing written", file=sys.stderr)
            return 1
        out.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(staged, out)
    finally:
        shutil.rmtree(trial_dir, ignore_errors=True)
    print(f"trim: wrote {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
