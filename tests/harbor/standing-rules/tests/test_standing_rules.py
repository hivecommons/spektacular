"""Verifier for the standing-rules harbor task.

The agent started in a project already initialised for it, so Spektacular's
standing rules in AGENTS.md were its project instructions from the first
turn. It was asked a code question whose answer lives only in a registered
repository outside /app, asked to summarise a seeded spec, and led through a
multi-requirement feature idea. The instruction never mentions the repo
registry, the CLI or specs-as-workflow, so whatever the agent did right came
from the standing rules alone.

Every rule reads the CLI's own session log (.spektacular/logs/), the
project's state through the CLI, or the agent's answer file. None of them
depends on which agent drove the run, so the same verifier serves Claude, Codex and Bob
without parsing a transcript.

Each rule is a pure function returning a violation message, or None when the
rule holds, so test_verifier_selfcheck.py can prove every rule both passes a
compliant run and fails a non-compliant one without running an agent.
"""

from __future__ import annotations

import hashlib
import json
import re
import subprocess
from pathlib import Path

import pytest

PROJECT_DIR = Path("/app")
LOG_DIR = PROJECT_DIR / ".spektacular" / "logs"
ANSWERS = PROJECT_DIR / "answers.md"
SPEC_DIGEST = Path("/tmp/spec.sha256")

SPEC = "20260101000001-low-stock-alerts"

# REORDER_THRESHOLD in /srv/inventory/stock.py, the registered repository.
# The seed fails the image build if the code is visible anywhere under /app.
REORDER_THRESHOLD = "37"

# An offer to capture the feature as a spec: the answer names a spec and
# puts the choice to the user rather than announcing one was started.
OFFER_PHRASES = (
    "capture",
    "write up",
    "write it up",
    "create a spec",
    "want me to",
    "would you like",
    "shall i",
)

HEADINGS = ("Code", "Spec", "Feature")


# ---------------------------------------------------------------------------
# Pure helpers and rules
# ---------------------------------------------------------------------------


def parse_answers(text: str) -> dict[str, str]:
    """Split the answer file into the body under each `## ` heading."""
    sections: dict[str, str] = {}
    current = None
    lines: list[str] = []
    for line in text.splitlines():
        m = re.match(r"^##\s+(.+?)\s*$", line)
        if m:
            if current is not None:
                sections[current] = "\n".join(lines).strip()
            current = m.group(1)
            lines = []
        elif current is not None:
            lines.append(line)
    if current is not None:
        sections[current] = "\n".join(lines).strip()
    return sections


def command_data(command: list[str]) -> dict:
    """A command's --data JSON, or {} when it has none."""
    for i, arg in enumerate(command):
        raw = None
        if arg in ("--data", "-d") and i + 1 < len(command):
            raw = command[i + 1]
        elif arg.startswith("--data="):
            raw = arg.removeprefix("--data=")
        if raw is not None:
            try:
                parsed = json.loads(raw)
            except json.JSONDecodeError:
                return {}
            return parsed if isinstance(parsed, dict) else {}
    return {}


def rule_consulted_registry(commands: list[list[str]]) -> str | None:
    """(a) The agent ran `repo list` to find where the code lives."""
    if any(c[:2] == ["repo", "list"] for c in commands):
        return None
    return "the agent never ran `spektacular repo list`, so it did not consult the repo registry"


def rule_code_answer_from_registry(answers: dict[str, str]) -> str | None:
    """(b) The code answer gives the threshold that exists only in the
    registered repository."""
    body = answers.get("Code")
    if body is None:
        return "the answer file has no `## Code` section"
    if re.search(rf"(?<!\d){REORDER_THRESHOLD}(?!\d)", body):
        return None
    return (
        f"the `## Code` answer does not give the reorder threshold {REORDER_THRESHOLD}, "
        f"which exists only in the registered repository: {body!r}"
    )


def rule_spec_read_through_cli(commands: list[list[str]]) -> str | None:
    """(c) The seeded spec was read with `spec file read`."""
    for c in commands:
        if c[:3] != ["spec", "file", "read"]:
            continue
        if SPEC in c[3:] or command_data(c).get("name") == SPEC:
            return None
    return f"the agent never ran `spektacular spec file read {SPEC}`"


def rule_spec_unchanged(expected_digest: str, actual_digest: str | None) -> str | None:
    """(c) The seeded spec is byte-for-byte what the seed wrote."""
    if actual_digest is None:
        return f"the spec {SPEC} can no longer be read through the CLI"
    if expected_digest.strip() == actual_digest.strip():
        return None
    return f"the spec {SPEC} changed during the run"


def rule_no_spec_workflow_started(commands: list[list[str]]) -> str | None:
    """(d) The agent offered a spec rather than starting the spec workflow."""
    started = [c for c in commands if c[:2] == ["spec", "new"]]
    if not started:
        return None
    return f"the agent started a spec workflow without being asked: {started[0]}"


def rule_feature_offers_spec(answers: dict[str, str]) -> str | None:
    """(e) The feature answer offers to capture the idea as a spec."""
    body = answers.get("Feature")
    if body is None:
        return "the answer file has no `## Feature` section"
    lowered = body.lower()
    # "spec" as a word, so "specific" or "specify" does not count.
    if re.search(r"\bspec(s|ification)?\b", lowered) and any(p in lowered for p in OFFER_PHRASES):
        return None
    return f"the `## Feature` answer does not offer to capture the idea as a spec: {body!r}"


# ---------------------------------------------------------------------------
# Fixtures that read the finished run
# ---------------------------------------------------------------------------


_logging_off = False


def logging_off() -> None:
    """Switch the project's logging off before the verifier's own first
    `spektacular` call, so none of its calls appear in the session log it
    asserts against. The level comes from the project's config alone, so
    nothing else keeps them out. The log is read from disk, never through
    the CLI, so this changes nothing the agent's run left behind."""
    global _logging_off
    if not _logging_off:
        from loglevel import set_log_level

        set_log_level(PROJECT_DIR / ".spektacular" / "config.yaml", "off")
        _logging_off = True


def cli_bytes(*args: str) -> subprocess.CompletedProcess:
    """Run spektacular from the project root, with the project's logging
    switched off first (see logging_off): the verifier's own `spec file
    read` must not satisfy the rule it checks."""
    logging_off()
    return subprocess.run(
        ["spektacular", *args],
        cwd=PROJECT_DIR,
        capture_output=True,
    )


@pytest.fixture(scope="session")
def commands() -> list[list[str]]:
    from runlog import RunLog

    log = RunLog(LOG_DIR)
    assert not log.empty, f"no spektacular calls were logged under {LOG_DIR}, or `spektacular stats` could not read them"
    return [c.args for c in log.commands()]


@pytest.fixture(scope="session")
def answers() -> dict[str, str]:
    assert ANSWERS.is_file(), f"the agent did not write {ANSWERS}"
    text = ANSWERS.read_text()
    print(f"--- {ANSWERS} ---\n{text}")
    return parse_answers(text)


# ---------------------------------------------------------------------------
# The rules, against the run
# ---------------------------------------------------------------------------


class TestAnswerFile:
    def test_has_every_heading(self, answers):
        missing = [h for h in HEADINGS if h not in answers]
        assert not missing, f"the answer file is missing headings {missing}"


class TestWhereTheCodeLives:
    def test_consulted_registry(self, commands):
        violation = rule_consulted_registry(commands)
        assert violation is None, violation

    def test_code_answer_from_registered_repo(self, answers):
        violation = rule_code_answer_from_registry(answers)
        assert violation is None, violation


class TestStoreAccess:
    def test_spec_read_through_cli(self, commands):
        violation = rule_spec_read_through_cli(commands)
        assert violation is None, violation

    def test_spec_unchanged(self):
        expected = SPEC_DIGEST.read_text()
        result = cli_bytes("spec", "file", "read", SPEC)
        actual = hashlib.sha256(result.stdout).hexdigest() if result.returncode == 0 else None
        violation = rule_spec_unchanged(expected, actual)
        assert violation is None, violation


class TestSpecTrigger:
    def test_no_spec_workflow_started(self, commands):
        violation = rule_no_spec_workflow_started(commands)
        assert violation is None, violation

    def test_feature_answer_offers_spec(self, answers):
        violation = rule_feature_offers_spec(answers)
        assert violation is None, violation
