"""Verify that the spektacular guided repo add completed, and — more to the
point — that the *conversation* it held obeyed the promises the step
templates make about it.

Most of what a guided add promises cannot be seen on disk. "Only the
repository was asked for cold", "each later question states a proposal so
that agreeing alone records it", "the questions arrive one per exchange" and
"Spektacular's vocabulary never reaches the user" are all properties of what
the user was *shown*, not of what was written. The only artefact that records
that is the agent's own transcript under /logs/agent, so this verifier reads
it.

Two scenarios share this file. `SPEK_SCENARIO` is exported into the harbor
run by the Makefile and is either "one-at-a-time" (the user answers every
question) or "delegated" (the user hands the remaining details over after the
first proposal). Two tests are scenario-exclusive and say so in their
docstrings.

The rules themselves live in module-level pure functions that take a list of
already-parsed transcript objects. The pytest tests below are thin wrappers
that read the real transcript and call them, which lets
`test_verifier_selfcheck.py` run the same rules against small synthetic
transcripts locally, with no container, and prove each one can actually fail.

Nothing here imports outside the standard library plus pytest: the image
carries python 3.12 and pytest 8.4.1 and no PyYAML, so the small amount of
YAML this needs is read with string handling (see `config_repo_entries` and
`yaml_top_level`).
"""

import json
import os
import re
from pathlib import Path

import pytest

PROJECT_DIR = Path("/app")
SPEK_DIR = PROJECT_DIR / ".spektacular"
CONFIG_FILE = SPEK_DIR / "config.yaml"

# The guided add keeps its own workflow state in a *sibling* of the file the
# spec/plan/implement workflows share, so an add can run alongside a spec
# already in progress. It is repo-state.json, not state.json.
STATE_FILE = SPEK_DIR / "repo-state.json"
# The scenario the Makefile bakes into the build, beside this file.
SCENARIO_FILE = Path(__file__).resolve().parent / "scenario"

AGENT_LOG_DIR = Path("/logs/agent")

# The repo the container seeds for the agent to add.
TARGET_REPO_DIR = "/srv/harbour"

# Hand-written oracle: the step order the guided add is specified to walk.
# This is deliberately NOT derived from internal/steps/repo/steps.go — a
# verifier that reads the implementation for its expectation can only ever
# agree with it.
EXPECTED_STEP_ORDER = [
    "new",
    "locate",
    "name",
    "description",
    "role",
    "tags",
    "placement",
    "confirm",
    "register",
    "finished",
]

# In the delegated scenario a single `goto` jumps from name straight to
# placement, carrying the description, role and tags with it. Those three
# steps are legitimately never entered, so they are legitimately absent from
# completed_steps.
DELEGATION_SKIPPED_STEPS = ["description", "role", "tags"]
EXPECTED_STEP_ORDER_DELEGATED = [
    s for s in EXPECTED_STEP_ORDER if s not in DELEGATION_SKIPPED_STEPS
]

# The four values the flow proposes rather than asks for, in the order their
# questions must arrive.
PROPOSED_FIELDS = ["name", "description", "role", "tags"]

# Built-in agent tools that mutate files directly, bypassing the spektacular
# CLI.
BUILTIN_FILE_TOOLS = {"Write", "Edit", "MultiEdit", "NotebookEdit"}

# Spektacular's own vocabulary, which the step templates forbid showing to the
# user. Each entry is (label, compiled pattern).
#
# Two words from the templates' own list are deliberately NOT scanned for:
# "source" and "separate" are ordinary English, and a sentence like "where its
# code lives is separate from …" is not a vocabulary leak. Scanning for them
# would manufacture failures rather than catch them, so the templates keep
# forbidding them and this verifier stays quiet about them.
BANNED_VOCABULARY = [
    ("footprint", re.compile(r"\bfootprints?\b", re.IGNORECASE)),
    ("provider", re.compile(r"\bproviders?\b", re.IGNORECASE)),
    ("colocated", re.compile(r"\bco-?located\b", re.IGNORECASE)),
    ("repo.yaml", re.compile(r"repo\.yaml", re.IGNORECASE)),
    ("config.yaml", re.compile(r"config\.yaml", re.IGNORECASE)),
    ("repo-state.json", re.compile(r"repo-state\.json", re.IGNORECASE)),
    ("state.json", re.compile(r"\bstate\.json", re.IGNORECASE)),
    ("repo add", re.compile(r"\brepo\s+add\b", re.IGNORECASE)),
    ("repo goto", re.compile(r"\brepo\s+goto\b", re.IGNORECASE)),
    ("--data", re.compile(r"--data\b")),
    ("--force", re.compile(r"--force\b")),
]

# Command text that must not appear in the confirmation put to the user: the
# confirm template says "Do not put any command in front of them. Not its
# name, not its arguments, not its flags." The bare word "spektacular" is fine
# — the placement template explicitly tells the agent to ask whether
# "Spektacular may add a folder to their repo" — so only command *shapes* are
# matched here.
COMMAND_TEXT_PATTERNS = [
    re.compile(r"\bspektacular\s+repo\b", re.IGNORECASE),
    re.compile(r"\brepo\s+(add|goto|new|list)\b", re.IGNORECASE),
    re.compile(r"\bgo\s+run\s+\."),
    re.compile(r"(?:^|\s)--[a-z][a-z-]+\b"),
]

# Markers strong enough to say "this turn is asking about role / tags".
# "name" and "description" are deliberately absent: every question in the flow
# mentions the repo's name, and the evidence the agent is handed literally
# reads "it describes itself as …", so both words appear in turns that are not
# asking about them. Only the two unambiguous markers are used.
TOPIC_MARKERS = {
    "role": re.compile(r"\brole\b", re.IGNORECASE),
    "tags": re.compile(r"\btags?\b", re.IGNORECASE),
}


# ---------------------------------------------------------------------------
# Transcript parsing
#
# These are lifted from tests/harbor/spec-workflow/tests/test_spec_workflow.py
# so the two verifiers read a transcript the same way.
# ---------------------------------------------------------------------------

def iter_transcript_objects():
    """Yield every JSON object across all agent transcripts, in order."""
    for transcript in sorted(AGENT_LOG_DIR.glob("*.txt")):
        for line in transcript.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def transcript_objects() -> list[dict]:
    """The whole transcript as a list, so the pure rules below can be handed
    the same shape a synthetic fixture builds."""
    return list(iter_transcript_objects())


def extract_tool_calls(objs=None) -> list[dict]:
    """Parse the agent transcript and return all Bash tool calls in order.

    Each entry is {"command": "…"}. Both transcript dialects are handled: the
    Claude-style `assistant` / `tool_use` block and the Codex-style
    `item.completed` / `command_execution` item.
    """
    if objs is None:
        objs = transcript_objects()

    calls = []
    for obj in objs:
        if obj.get("type") == "item.completed":
            item = obj.get("item", {}) or {}
            if item.get("type") == "command_execution":
                cmd = item.get("command", "")
                if cmd:
                    calls.append({"command": cmd})
            continue
        if obj.get("type") != "assistant":
            continue
        msg = obj.get("message", {}) or {}
        for block in msg.get("content", []) or []:
            if block.get("type") == "tool_use" and block.get("name") == "Bash":
                cmd = (block.get("input", {}) or {}).get("command", "")
                if cmd:
                    calls.append({"command": cmd})
    return calls


def extract_builtin_file_edits(objs=None) -> list[dict]:
    """Return every built-in file-mutation tool call (Write/Edit/...) in order.

    Each entry is {"name": <tool>, "file_path": <path>}. Only Claude-style
    `tool_use` blocks carry them.
    """
    if objs is None:
        objs = transcript_objects()

    edits = []
    for obj in objs:
        if obj.get("type") != "assistant":
            continue
        for block in (obj.get("message", {}) or {}).get("content", []) or []:
            if block.get("type") != "tool_use":
                continue
            if block.get("name") not in BUILTIN_FILE_TOOLS:
                continue
            inp = block.get("input", {}) or {}
            path = inp.get("file_path") or inp.get("notebook_path") or ""
            edits.append({"name": block["name"], "file_path": path})
    return edits


def agent_result_events(objs=None) -> list[dict]:
    """Return all `result`-type events from the transcript, in order."""
    if objs is None:
        objs = transcript_objects()
    return [o for o in objs if o.get("type") == "result"]


def agent_auth_failure(objs=None):
    """Return the first transcript object signalling an Anthropic API
    authentication failure (HTTP 401), or None if there is none."""
    if objs is None:
        objs = transcript_objects()
    for obj in objs:
        if obj.get("error") == "authentication_failed":
            return obj
        if obj.get("api_error_status") == 401:
            return obj
    return None


# ---------------------------------------------------------------------------
# The user-facing view of the transcript
#
# THE BOUNDARY THAT MATTERS: a guided add legitimately *runs* every command
# and reads every file whose name it must never say out loud. `spektacular
# repo goto --data '{"step":"role", …}'` appears in this transcript on every
# single run, by design. So the vocabulary and command-text rules below scan
# `assistant_texts()` and nothing else:
#
#   * `assistant_texts()`  — prose the agent emitted as a message. This is
#     what the user is shown, and the only thing the conversational rules may
#     look at.
#   * tool-call inputs (`tool_use` blocks, `command_execution` items) — the
#     agent's own working detail. NEVER scanned. Every command it runs lives
#     here, banned words and all.
#   * tool results (`user`-type objects carrying tool_result, and any stdout
#     echoed back) — Spektacular's output to the agent, not to the user.
#     NEVER scanned. The rendered step instructions themselves contain the
#     words "footprint" and "provider", so scanning results would fail every
#     run unconditionally.
#
# `transcript_events()` keeps text and Bash calls interleaved in transcript
# order, which is what lets a question be correlated with the `goto` that
# followed it.
# ---------------------------------------------------------------------------

def transcript_events(objs) -> list[dict]:
    """Return assistant prose and Bash commands, interleaved, in order.

    Entries are {"kind": "text", "text": …} or {"kind": "bash", "command": …}.
    Nothing else is included: no tool results, no non-Bash tool inputs.
    """
    events: list[dict] = []
    for obj in objs:
        if obj.get("type") == "item.completed":
            item = obj.get("item", {}) or {}
            kind = item.get("type")
            if kind == "command_execution":
                cmd = item.get("command", "")
                if cmd:
                    events.append({"kind": "bash", "command": cmd})
            elif kind == "agent_message":
                text = item.get("text", "") or ""
                if text.strip():
                    events.append({"kind": "text", "text": text})
            continue
        if obj.get("type") != "assistant":
            continue
        for block in (obj.get("message", {}) or {}).get("content", []) or []:
            btype = block.get("type")
            if btype == "text":
                text = block.get("text", "") or ""
                if text.strip():
                    events.append({"kind": "text", "text": text})
            elif btype == "tool_use" and block.get("name") == "Bash":
                cmd = (block.get("input", {}) or {}).get("command", "")
                if cmd:
                    events.append({"kind": "bash", "command": cmd})
    return events


def assistant_texts(objs) -> list[str]:
    """Every piece of prose the agent addressed to the user, in order."""
    return [e["text"] for e in transcript_events(objs) if e["kind"] == "text"]


def conversation_texts(objs) -> list[str]:
    """The prose the agent addressed to the user, up to the instruction's
    "After completion" step.

    Once the agent copies `.spektacular` to /logs/artifacts the conversation
    with the user is over, and what follows is its report to this harness,
    which echoes the task's own success criteria (`repo add` among them).
    """
    texts = []
    for e in transcript_events(objs):
        if e["kind"] == "bash" and "/logs/artifacts" in e["command"]:
            break
        if e["kind"] == "text":
            texts.append(e["text"])
    return texts


def is_question(text: str) -> bool:
    """A turn counts as asking the user something if it contains a question
    mark. Crude, but it is what a reader of the transcript would use, and the
    guided add's questions are all literal questions."""
    return "?" in text


def normalize(text: str) -> str:
    """Collapse whitespace so a value can be found in prose that wrapped it
    across lines. Case is folded: agreeing to a proposal is not sensitive to
    how the agent capitalized it when echoing it back."""
    return re.sub(r"\s+", " ", text).strip().casefold()


# ---------------------------------------------------------------------------
# `repo goto` payloads
# ---------------------------------------------------------------------------

def _first_json_object(text: str):
    """Return the first balanced {...} run in text parsed as JSON, or None.

    Scanning for balanced braces rather than matching a quoting style keeps
    this working whether the agent wrote --data '{"…"}', --data "{\\"…\\"}"
    or a heredoc.
    """
    start = text.find("{")
    while start != -1:
        depth = 0
        in_string = False
        escaped = False
        for i in range(start, len(text)):
            ch = text[i]
            if in_string:
                if escaped:
                    escaped = False
                elif ch == "\\":
                    escaped = True
                elif ch == '"':
                    in_string = False
                continue
            if ch == '"':
                in_string = True
            elif ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    candidate = text[start:i + 1]
                    for attempt in (candidate, candidate.replace('\\"', '"')):
                        try:
                            parsed = json.loads(attempt)
                        except json.JSONDecodeError:
                            continue
                        if isinstance(parsed, dict):
                            return parsed
                    break
        start = text.find("{", start + 1)
    return None


def goto_payload(command: str):
    """Return the --data payload of a `repo goto` command, or None."""
    if not re.search(r"\brepo\s+goto\b", command):
        return None
    return _first_json_object(command)


def goto_calls(objs) -> list[dict]:
    """Every `repo goto` call, as {"index": <event index>, "payload": {...}}.

    A call whose payload could not be parsed is reported with an empty
    payload rather than dropped, so it still counts as a `goto` for the
    "exactly one question came first" rule.
    """
    calls = []
    for i, event in enumerate(transcript_events(objs)):
        if event["kind"] != "bash":
            continue
        if not re.search(r"\brepo\s+goto\b", event["command"]):
            continue
        payload = goto_payload(event["command"]) or {}
        calls.append({"index": i, "payload": payload, "command": event["command"]})
    return calls


def repo_add_commands(objs) -> list[str]:
    """Every Bash command that ran `repo add` — the direct registration the
    guided flow exists to replace."""
    return [
        e["command"]
        for e in transcript_events(objs)
        if e["kind"] == "bash" and re.search(r"\brepo\s+add\b", e["command"])
    ]


# ---------------------------------------------------------------------------
# The rules, as pure functions over parsed transcript objects
# ---------------------------------------------------------------------------

def questions_before_first_goto(objs) -> list[str]:
    """Return the question turns that precede the first `repo goto` call.

    The locate step is the only one asked empty-handed, so exactly one
    question belongs here. Anything more means the agent asked for something
    it was supposed to propose.
    """
    events = transcript_events(objs)
    calls = goto_calls(objs)
    limit = calls[0]["index"] if calls else len(events)
    return [
        e["text"]
        for e in events[:limit]
        if e["kind"] == "text" and is_question(e["text"])
    ]


def location_given_to_new(objs) -> bool:
    """Whether a `repo new` before the first `repo goto` carried the location.

    The skill passes a location the user already named straight to `repo
    new`, and the flow then has nothing left to ask empty-handed.
    """
    events = transcript_events(objs)
    calls = goto_calls(objs)
    limit = calls[0]["index"] if calls else len(events)
    for event in events[:limit]:
        if event["kind"] != "bash" or not re.search(r"\brepo\s+new\b", event["command"]):
            continue
        payload = _first_json_object(event["command"]) or {}
        if payload.get("location"):
            return True
    return False


def _last_question_before(events, index):
    """The most recent question turn strictly before `index`, with its own
    event index, or (None, None)."""
    for i in range(index - 1, -1, -1):
        if events[i]["kind"] == "text" and is_question(events[i]["text"]):
            return i, events[i]["text"]
    return None, None


def field_proposals(objs) -> list[dict]:
    """Correlate each proposed value with the question that preceded it.

    For every `repo goto` payload, each of name/description/role/tags it
    carries is paired with the last question turn before that call. Entries
    are {"field", "value", "goto_index", "question_index", "question",
    "sole"}, where `sole` says whether that payload carried exactly one of
    the four fields — i.e. whether it is a step-by-step answer rather than
    the delegation hand-over that carries several at once.
    """
    events = transcript_events(objs)
    proposals = []
    for call in goto_calls(objs):
        present = [f for f in PROPOSED_FIELDS if f in call["payload"]]
        q_index, question = _last_question_before(events, call["index"])
        for field in present:
            proposals.append({
                "field": field,
                "value": call["payload"][field],
                "goto_index": call["index"],
                "question_index": q_index,
                "question": question,
                "sole": len(present) == 1,
            })
    return proposals


def proposal_precedes_payload(objs) -> list[str]:
    """Return a violation string for every value recorded without having been
    stated in the question that preceded it.

    This is the "agreeing alone records it" property: the user is only ever
    asked to agree, so whatever a `goto` records must already have been in
    front of them.

    Which values are checked:

      * `name` always — it is proposed in a question on every path, including
        the delegated one where the user hands the rest over in reply to it.
      * `description`, `role` and `tags` only when their `goto` carried
        exactly one of the four fields. A payload carrying several at once IS
        the delegation shortcut, where by design only the name was put to the
        user; `questions_after_delegation` is the rule that covers that path.
    """
    violations = []
    for p in field_proposals(objs):
        if p["field"] != "name" and not p["sole"]:
            continue
        if p["question"] is None:
            violations.append(
                f"{p['field']}={p['value']!r} was recorded with no question "
                f"before it — the user was never shown anything to agree to"
            )
            continue
        haystack = normalize(p["question"])
        values = p["value"] if isinstance(p["value"], list) else [p["value"]]
        for value in values:
            needle = normalize(str(value))
            if needle and needle not in haystack:
                violations.append(
                    f"{p['field']} recorded as {value!r} but that value does "
                    f"not appear in the question that preceded it: "
                    f"{p['question']!r}"
                )
    return violations


def question_order_violations(objs) -> list[str]:
    """Return violations of "one question per exchange, in order".

    Three things are checked, all of them read off the correlation between a
    question turn and the `goto` that recorded its answer:

      1. the four fields are recorded in the order name, description, role,
         tags;
      2. each arrives on its own `goto` — none share one, which is what
         delegation would look like;
      3. no two of them are preceded by the *same* assistant turn, which is
         what "no single message asks about more than one" reduces to once
         the questions are correlated with their answers.

    A fourth, narrower check uses the two unambiguous topic markers: the name
    and description questions must not mention role or tags. See
    TOPIC_MARKERS for why the other two words are not used as markers.
    """
    proposals = field_proposals(objs)
    violations = []

    seen_order = [p["field"] for p in proposals]
    if seen_order != PROPOSED_FIELDS:
        violations.append(
            f"values were recorded in the order {seen_order}, expected "
            f"{PROPOSED_FIELDS}"
        )

    by_goto: dict[int, list[str]] = {}
    for p in proposals:
        by_goto.setdefault(p["goto_index"], []).append(p["field"])
    for index, fields in sorted(by_goto.items()):
        if len(fields) > 1:
            violations.append(
                f"one call recorded {fields} together — these must arrive one "
                f"per exchange"
            )

    by_question: dict[int, list[str]] = {}
    for p in proposals:
        by_question.setdefault(p["question_index"], []).append(p["field"])
    for index, fields in sorted(by_question.items(), key=lambda kv: (kv[0] is None, kv[0])):
        if len(fields) > 1:
            violations.append(
                f"a single assistant turn asked about {fields} — each gets "
                f"its own turn"
            )

    for p in proposals:
        if p["field"] not in ("name", "description") or p["question"] is None:
            continue
        for topic, marker in TOPIC_MARKERS.items():
            if marker.search(p["question"]):
                violations.append(
                    f"the {p['field']} question also raised {topic}: "
                    f"{p['question']!r}"
                )

    return violations


def delegation_handover_index(objs):
    """The event index of the `goto` that carried more than one proposed
    field at once — the delegation shortcut — or None."""
    for call in goto_calls(objs):
        present = [f for f in PROPOSED_FIELDS if f in call["payload"]]
        if len(present) > 1:
            return call["index"]
    return None


def confirmation_turn(objs):
    """The question turn that put the registration to the user, as
    (event index, text), or (None, None).

    It is the last question before the `goto` that advances to the register
    step — the one call in the flow whose payload names no value, because
    nothing is written until the user has agreed.
    """
    events = transcript_events(objs)
    for call in goto_calls(objs):
        if call["payload"].get("step") == "register":
            return _last_question_before(events, call["index"])
    return None, None


def questions_after_delegation(objs) -> list[str]:
    """Return question turns that still ask about description, role or tags
    after the user handed those over.

    The window runs from the hand-over call to the confirmation turn,
    exclusive. The confirmation is left out on purpose: it is allowed to
    summarize what will be registered, and criterion 7 governs it instead.
    """
    handover = delegation_handover_index(objs)
    if handover is None:
        return []

    events = transcript_events(objs)
    confirm_index, _ = confirmation_turn(objs)
    end = confirm_index if confirm_index is not None else len(events)

    offenders = []
    for event in events[handover:end]:
        if event["kind"] != "text" or not is_question(event["text"]):
            continue
        for marker in TOPIC_MARKERS.values():
            if marker.search(event["text"]):
                offenders.append(event["text"])
                break
    return offenders


def banned_terms_in_assistant_text(objs) -> list[tuple[str, str]]:
    """Return (term, turn) for every piece of Spektacular's internal
    vocabulary that reached the user.

    Only assistant prose is scanned — see the boundary note above. The agent
    runs `repo goto --data …` on every successful run, and the step
    instructions it reads back are full of "footprint" and "provider"; both
    are working detail, and neither is a leak. The agent's closing report to
    this harness is not scanned either: see conversation_texts.
    """
    found = []
    for text in conversation_texts(objs):
        for label, pattern in BANNED_VOCABULARY:
            if pattern.search(text):
                found.append((label, text))
    return found


def command_text_in(text: str) -> list[str]:
    """Return the command-shaped fragments quoted in a piece of prose."""
    hits = []
    for pattern in COMMAND_TEXT_PATTERNS:
        match = pattern.search(text)
        if match:
            hits.append(match.group(0).strip())
    return hits


# ---------------------------------------------------------------------------
# On-disk state
#
# config.yaml and repo.yaml are read with string handling rather than a YAML
# parser: the image has python 3.12, pytest and the standard library, and no
# PyYAML. Both files are written by the same marshaller and the handful of
# keys needed here are plain scalars and a flat list, so a small
# indentation-aware reader is enough. Neither reader is a general YAML
# implementation and neither should grow into one.
# ---------------------------------------------------------------------------

def _strip_scalar(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        value = value[1:-1]
    return value.strip()


def _indent(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


def yaml_top_level(text: str) -> dict:
    """Read top-level scalars and flat string lists out of a small YAML file.

    Values are returned as str (scalars, with wrapped continuation lines
    rejoined) or list[str] (block sequences). Nested mappings are skipped —
    nothing here needs them.
    """
    lines = [l.rstrip() for l in text.splitlines()]
    result: dict = {}
    i = 0
    while i < len(lines):
        line = lines[i]
        if not line.strip() or line.lstrip().startswith("#") or _indent(line) != 0:
            i += 1
            continue
        match = re.match(r"^([A-Za-z_][\w.-]*):\s*(.*)$", line)
        if not match:
            i += 1
            continue
        key, rest = match.group(1), match.group(2)
        i += 1

        if rest:
            parts = [_strip_scalar(rest)]
            # A long scalar may have been wrapped onto indented continuation
            # lines that are neither a new key nor a sequence item.
            while i < len(lines):
                nxt = lines[i]
                if not nxt.strip() or _indent(nxt) == 0:
                    break
                stripped = nxt.strip()
                if stripped.startswith("- ") or re.match(r"^[A-Za-z_][\w.-]*:", stripped):
                    break
                parts.append(stripped)
                i += 1
            result[key] = " ".join(parts).strip()
            continue

        items = []
        saw_nested = False
        while i < len(lines):
            nxt = lines[i]
            if not nxt.strip():
                i += 1
                continue
            if _indent(nxt) == 0:
                break
            stripped = nxt.strip()
            if stripped.startswith("- "):
                items.append(_strip_scalar(stripped[2:]))
            else:
                saw_nested = True
            i += 1
        if items:
            result[key] = items
        elif saw_nested:
            result[key] = {}
        else:
            result[key] = ""
    return result


def config_repo_entries(text: str) -> list[dict]:
    """Read the `repos:` registry out of config.yaml as [{name, location}].

    Only the two keys this verifier needs are picked up; anything else on an
    entry is ignored.
    """
    lines = [l.rstrip() for l in text.splitlines()]
    entries: list[dict] = []
    in_repos = False
    current: dict | None = None

    for line in lines:
        if not line.strip():
            continue
        if _indent(line) == 0:
            if current:
                entries.append(current)
                current = None
            in_repos = line.strip() == "repos:"
            continue
        if not in_repos:
            continue

        stripped = line.strip()
        if stripped.startswith("- "):
            if current:
                entries.append(current)
            current = {}
            stripped = stripped[2:].strip()
        if current is None:
            continue
        match = re.match(r"^([A-Za-z_][\w.-]*):\s*(.*)$", stripped)
        if match and match.group(1) in ("name", "location"):
            current[match.group(1)] = _strip_scalar(match.group(2))

    if current:
        entries.append(current)
    return entries


def load_state() -> dict:
    assert STATE_FILE.exists(), f"Guided-add state not found at {STATE_FILE}"
    return json.loads(STATE_FILE.read_text())


def scenario() -> str:
    """Which scenario this run is: "one-at-a-time" or "delegated".

    SPEK_SCENARIO, when it reaches the verifier, is the authority; otherwise
    the `scenario` file the Makefile writes beside this verifier is. Harbor
    does not pass the host's environment into the verifier container, so the
    file is what a real run reads. Only when neither exists is the scenario
    inferred from whether the delegated path's skipped steps were walked, and
    that inference would grade an agent that delegated when told not to as
    the delegated scenario, so a run never relies on it.
    """
    from_env = os.environ.get("SPEK_SCENARIO", "").strip()
    if from_env:
        return from_env
    if SCENARIO_FILE.exists():
        return SCENARIO_FILE.read_text().strip()
    try:
        completed = load_state().get("completed_steps", [])
    except (AssertionError, OSError, json.JSONDecodeError):
        return "one-at-a-time"
    return "one-at-a-time" if "description" in completed else "delegated"


def expected_step_order() -> list[str]:
    return (
        EXPECTED_STEP_ORDER_DELEGATED
        if scenario() == "delegated"
        else EXPECTED_STEP_ORDER
    )


def registered_repo() -> dict:
    """The registry entry for the repo the agent added, as {name, location}.

    The project registers itself under its own name at location "." during
    init; the added repo is the entry that is neither.
    """
    assert CONFIG_FILE.exists(), f"config.yaml missing at {CONFIG_FILE} — init not run"
    text = CONFIG_FILE.read_text()
    # The image pre-seeds a config holding only log.level; init records the
    # installed skills.
    assert "skills_version:" in text, f"config.yaml at {CONFIG_FILE} records no installed skills — init not run"
    project_name = yaml_top_level(text).get("name", "")
    entries = config_repo_entries(text)
    added = [
        e for e in entries
        if e.get("name") and e.get("name") != project_name and e.get("location") != "."
    ]
    assert len(added) == 1, (
        f"expected exactly one newly registered repo in {CONFIG_FILE}, found "
        f"{added} (all entries: {entries}, project name: {project_name!r})"
    )
    return added[0]


def registered_repo_config() -> dict:
    """The added repo's own repo.yaml, parsed.

    A registry location is relative to the folder holding config.yaml and
    names the folder holding that repo's repo.yaml, so nothing is appended
    beyond the filename.
    """
    entry = registered_repo()
    location = entry.get("location", "")
    base = Path(location) if os.path.isabs(location) else (SPEK_DIR / location)
    path = (base / "repo.yaml").resolve()
    assert path.exists(), (
        f"registered repo {entry.get('name')!r} has no repo.yaml at {path}"
    )
    return yaml_top_level(path.read_text())


# ---------------------------------------------------------------------------
# Agent-execution preflight — runs first
# ---------------------------------------------------------------------------

class TestAgentExecution:
    """Preflight: the coding agent authenticated and actually ran.

    Defined first so it runs before any workflow assertion. If the agent
    never started — e.g. an invalid ANTHROPIC_API_KEY — then config.yaml,
    repo-state.json and the transcript rules all fail for a reason that has
    nothing to do with the guided add. When this class fails, read it first:
    every other failure in the run is a consequence.
    """

    def test_transcript_exists(self):
        transcripts = sorted(AGENT_LOG_DIR.glob("*.txt"))
        assert transcripts, (
            f"No agent transcript under {AGENT_LOG_DIR} — the agent never ran."
        )

    def test_agent_authenticated(self):
        failure = agent_auth_failure()
        assert failure is None, (
            "Agent failed to authenticate with the Anthropic API "
            f"(api_error_status={failure.get('api_error_status')}): "
            f"{failure.get('result') or failure.get('error')!r}. "
            "Set a valid ANTHROPIC_API_KEY before running harbor — every "
            "other failure in this run is a consequence of this."
        )

    def test_agent_run_succeeded(self):
        results = agent_result_events()
        assert results, (
            "No `result` event in the transcript — the agent did not finish."
        )
        final = results[-1]
        assert not final.get("is_error"), (
            f"Agent run ended in error: {final.get('result')!r} "
            f"(api_error_status={final.get('api_error_status')}, "
            f"num_turns={final.get('num_turns')})."
        )

    def test_agent_did_work(self):
        calls = extract_tool_calls()
        assert calls, (
            "Agent produced no tool calls — it never ran 'spektacular init' "
            "or any workflow command. Check the transcript for an earlier "
            "failure (often an auth error)."
        )


# ---------------------------------------------------------------------------
# Criterion 1 — the workflow completed
# ---------------------------------------------------------------------------

class TestWorkflowCompleted:
    """Criterion: the guided add ran to completion.

    Its state lives in its own file, repo-state.json, carries kind "repo",
    ends on the finished step, and records exactly the steps the hand-written
    order says it should — allowing for the three the delegated scenario
    legitimately jumps over.
    """

    def test_state_file_exists(self):
        assert STATE_FILE.exists(), (
            f"No guided-add state at {STATE_FILE}. Note this workflow keeps "
            "its state in repo-state.json, a sibling of state.json, so that "
            "an add can run alongside a spec already in progress."
        )

    def test_state_kind_is_repo(self):
        state = load_state()
        assert state.get("kind") == "repo", (
            f"state kind is {state.get('kind')!r}, expected 'repo' — the file "
            "at repo-state.json belongs to a different workflow."
        )

    def test_workflow_reached_finished(self):
        state = load_state()
        assert state.get("current_step") == "finished", (
            f"Guided add did not finish, stuck at: {state.get('current_step')!r}"
        )

    def test_steps_completed_in_expected_order(self):
        state = load_state()
        completed = state.get("completed_steps", [])
        expected = expected_step_order()
        assert completed == expected, (
            f"completed_steps does not match the guided add's step order "
            f"(scenario: {scenario()}).\n"
            f"  Expected: {expected}\n"
            f"  Got:      {completed}\n"
            "In the delegated scenario description, role and tags are "
            "legitimately absent: one goto jumps from name to placement."
        )


# ---------------------------------------------------------------------------
# Criterion 2 — the registration carries complete metadata
# ---------------------------------------------------------------------------

class TestRegistrationMetadata:
    """Criterion: the repo is registered with complete metadata — a
    description, a role and tags — not merely registered.

    The registry entry in config.yaml says where the repo's files live; the
    descriptive metadata lives in that repo's own repo.yaml. Both are read,
    and all three fields must be non-empty on either scenario: delegation
    changes who supplied them, not whether they exist.
    """

    def test_repo_is_registered(self):
        entry = registered_repo()
        assert entry.get("name"), f"registry entry has no name: {entry}"
        assert entry.get("location"), f"registry entry has no location: {entry}"

    def test_description_is_present(self):
        cfg = registered_repo_config()
        assert cfg.get("description", "").strip(), (
            f"registered repo has no description in its repo.yaml: {cfg}"
        )

    def test_role_is_present(self):
        cfg = registered_repo_config()
        assert str(cfg.get("role", "")).strip(), (
            f"registered repo has no role in its repo.yaml: {cfg}"
        )

    def test_tags_are_present(self):
        cfg = registered_repo_config()
        tags = cfg.get("tags") or []
        assert isinstance(tags, list) and [t for t in tags if t.strip()], (
            f"registered repo has no tags in its repo.yaml: {cfg}"
        )


# ---------------------------------------------------------------------------
# Criterion 3 — only the repository was asked for cold
# ---------------------------------------------------------------------------

class TestOnlyRepositoryAskedCold:
    """Criterion: the repository itself is the only thing asked for
    empty-handed.

    The locate step is the one question the agent cannot answer for itself.
    Everything after it must arrive with a proposal, which means exactly one
    question may precede the first `repo goto` call, or none when the user
    already named the repo and `repo new` was given its location. The turns
    are correlated against the tool calls in transcript order, so "before the
    first goto" means what it says.
    """

    def test_exactly_one_question_before_first_goto(self):
        objs = transcript_objects()
        questions = questions_before_first_goto(objs)
        if location_given_to_new(objs):
            assert questions == [], (
                f"`repo new` was given the location, so nothing is left to ask "
                f"before the first `repo goto`, but found {len(questions)}: "
                f"{questions!r}"
            )
            return
        assert len(questions) == 1, (
            f"Expected exactly one question before the first `repo goto` — "
            f"the one asking which repo to add — but found {len(questions)}: "
            f"{questions!r}"
        )


# ---------------------------------------------------------------------------
# Criterion 4 — each later question states a proposed value
# ---------------------------------------------------------------------------

class TestQuestionsCarryProposals:
    """Criterion: every question after the first states its proposed value,
    so agreeing alone is enough to record it.

    Each value that lands in a `repo goto` payload is matched against the
    question that preceded that call: if the user was only ever asked to
    agree, the value must already have been in front of them.
    """

    def test_every_recorded_value_was_proposed_first(self):
        objs = transcript_objects()
        violations = proposal_precedes_payload(objs)
        assert not violations, (
            "Values were recorded that the user was never shown:\n  - "
            + "\n  - ".join(violations)
        )


# ---------------------------------------------------------------------------
# Criterion 5 — one question per exchange, in order
# ---------------------------------------------------------------------------

class TestOneQuestionPerExchange:
    """Criterion: the name, description, role and tags questions arrive one
    per exchange, in that order, each in its own assistant turn.

    Only meaningful when the user is answering them one at a time — the
    delegated scenario exists precisely to collapse them, so this class is
    skipped there and criterion 6 takes over.
    """

    @pytest.mark.skipif(
        scenario() == "delegated",
        reason="the delegated scenario collapses these four questions on purpose",
    )
    def test_questions_arrive_one_at_a_time_in_order(self):
        objs = transcript_objects()
        violations = question_order_violations(objs)
        assert not violations, (
            "The four proposals were not put to the user one per exchange in "
            "order:\n  - " + "\n  - ".join(violations)
        )


# ---------------------------------------------------------------------------
# Criterion 6 — delegation ends the questioning
# ---------------------------------------------------------------------------

class TestDelegationEndsQuestioning:
    """Criterion: once the user hands the remaining details over, they are
    not asked about them again — and the values still get recorded.

    Only meaningful in the delegated scenario, where the user answers the
    first proposal with "just use what you think". The window examined runs
    from the hand-over call to the confirmation turn; the confirmation itself
    is criterion 7's business.
    """

    @pytest.mark.skipif(
        scenario() != "delegated",
        reason="only the delegated scenario hands the remaining details over",
    )
    def test_handover_actually_happened(self):
        objs = transcript_objects()
        assert delegation_handover_index(objs) is not None, (
            "No `repo goto` carried more than one of name/description/role/"
            "tags, so the user's hand-over never took the shortcut it was "
            "supposed to."
        )

    @pytest.mark.skipif(
        scenario() != "delegated",
        reason="only the delegated scenario hands the remaining details over",
    )
    def test_no_further_questions_about_delegated_fields(self):
        objs = transcript_objects()
        offenders = questions_after_delegation(objs)
        assert not offenders, (
            "After the user handed the details over, the agent kept asking "
            "about them:\n  - " + "\n  - ".join(repr(o) for o in offenders)
        )

    @pytest.mark.skipif(
        scenario() != "delegated",
        reason="only the delegated scenario hands the remaining details over",
    )
    def test_delegated_values_were_still_recorded(self):
        cfg = registered_repo_config()
        tags = cfg.get("tags") or []
        assert cfg.get("description", "").strip(), (
            f"delegation left the description empty: {cfg}"
        )
        assert str(cfg.get("role", "")).strip(), (
            f"delegation left the role empty: {cfg}"
        )
        assert isinstance(tags, list) and [t for t in tags if t.strip()], (
            f"delegation left the tags empty: {cfg}"
        )


# ---------------------------------------------------------------------------
# Criterion 7 — the confirmation names repo, folder and code location
# ---------------------------------------------------------------------------

class TestConfirmation:
    """Criterion: before anything is written, the user is told which repo is
    being registered, which folder will be created and where its code lives —
    and is shown no command.

    The confirmation is the last question before the call that advances to
    the register step, which is the only point where the flow writes.
    """

    def test_confirmation_turn_exists(self):
        objs = transcript_objects()
        index, text = confirmation_turn(objs)
        assert text, (
            "No question was put to the user before advancing to the register "
            "step — nothing is supposed to be written until they agree."
        )

    def test_confirmation_names_repo_and_paths(self):
        objs = transcript_objects()
        _, text = confirmation_turn(objs)
        assert text, "no confirmation turn found"
        haystack = normalize(text)

        name = registered_repo().get("name", "")
        assert normalize(name) in haystack, (
            f"the confirmation never names the repo being registered "
            f"({name!r}): {text!r}"
        )

        location = (load_state().get("data") or {}).get("location") or TARGET_REPO_DIR
        assert normalize(str(location)) in haystack, (
            f"the confirmation never states where the repo's code lives / "
            f"which folder is created under it ({location!r}): {text!r}"
        )

    def test_confirmation_quotes_no_command(self):
        objs = transcript_objects()
        _, text = confirmation_turn(objs)
        assert text, "no confirmation turn found"
        hits = command_text_in(text)
        assert not hits, (
            f"the confirmation put command text in front of the user "
            f"({hits!r}): {text!r}"
        )


# ---------------------------------------------------------------------------
# Criterion 8 — no internal vocabulary reaches the user
# ---------------------------------------------------------------------------

class TestNoInternalVocabulary:
    """Criterion: Spektacular's own vocabulary stays out of what the user is
    shown.

    Scope, precisely: only assistant *prose* is scanned. The agent
    legitimately runs `spektacular repo goto --data '{…}'` on every
    successful run, and the step instructions it reads back name repo.yaml,
    footprints and providers throughout. Neither tool-call inputs nor tool
    results are user-facing, so neither is examined; see the boundary note
    beside `transcript_events`.
    """

    def test_no_banned_vocabulary_in_assistant_prose(self):
        objs = transcript_objects()
        found = banned_terms_in_assistant_text(objs)
        assert not found, (
            "Spektacular's internal vocabulary reached the user:\n  - "
            + "\n  - ".join(f"{term!r} in {text!r}" for term, text in found)
        )


# ---------------------------------------------------------------------------
# Criterion 9 — the guided flow was used, not the direct command
# ---------------------------------------------------------------------------

class TestGuidedFlowUsed:
    """Criterion: the repo was registered through the guided conversation,
    not by calling `repo add` directly.

    `repo add` is a real, supported command; the point of this exercise is
    that the skill holds the conversation instead. Unlike the vocabulary
    rule, this one scans the commands the agent *ran*.
    """

    def test_repo_add_never_called(self):
        objs = transcript_objects()
        offenders = repo_add_commands(objs)
        assert not offenders, (
            "The agent registered the repo with `repo add` instead of the "
            "guided flow:\n  - " + "\n  - ".join(offenders)
        )

    def test_guided_flow_was_driven(self):
        objs = transcript_objects()
        assert goto_calls(objs), (
            "No `repo goto` call in the transcript — the guided add was never "
            "driven."
        )
