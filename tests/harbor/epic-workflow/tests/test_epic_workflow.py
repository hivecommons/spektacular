"""Verifier for the epic-workflow harbor task.

The agent ran the spek-plan-epic skill over a seeded epic of three specs:
storage and help independent, commands depending on storage.

Every assertion here reads either the project's state through the
spektacular CLI or the CLI's own debug session log. Neither depends on which
agent drove the run, so the same verifier serves Claude, Codex and Bob
without parsing a transcript, except the Claude-only child checks
(TestChildren), which read the transcript and skip for other agents. The plan-workflow suite already proves a single
plan workflow's transcript-level rules (skill retrieval, CLI-only writes);
this suite proves the orchestration around it.
"""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

import pytest

from sessionlog import SessionLog

PROJECT_DIR = Path("/app")
DEBUG_DIR = PROJECT_DIR / ".spektacular" / "debug"

STORAGE = "20260101000001-storage"
COMMANDS = "20260101000002-commands"
HELP = "20260101000003-help"
SPECS = [STORAGE, COMMANDS, HELP]


def cli(*args: str, parse: bool = True):
    """Run spektacular from the project root.

    SPEKTACULAR_SESSION_LOG=off keeps the verifier's own calls out of the
    session log it is asserting against.
    """
    result = subprocess.run(
        ["spektacular", *args],
        cwd=PROJECT_DIR,
        capture_output=True,
        text=True,
        env={**os.environ, "SPEKTACULAR_SESSION_LOG": "off"},
    )
    if not parse:
        return result
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.fail(f"spektacular {' '.join(args)} returned non-JSON: {result.stdout!r} {result.stderr!r}")


@pytest.fixture(scope="session")
def epic() -> str:
    files = cli("epic", "list").get("files") or []
    names = [f["name"] for f in files]
    assert len(names) == 1, f"expected the project's single seeded epic, found {names}"
    return names[0]


@pytest.fixture(scope="session")
def status(epic) -> dict:
    out = cli("status", epic, "--format", "json")
    assert not out.get("error"), out
    return out


@pytest.fixture(scope="session")
def log() -> SessionLog:
    return SessionLog(DEBUG_DIR)


# ---------------------------------------------------------------------------
# The epic is fully planned.
# ---------------------------------------------------------------------------


class TestEverySpecPlanned:
    def test_every_spec_plan_done(self, status):
        states = {s["name"]: s["run"]["plan"]["state"] for s in status["specs"]}
        assert states == {name: "done" for name in SPECS}, (
            f"every spec's planning must be done; status reports {states}"
        )

    def test_epic_progress_counts_all_specs(self, status):
        progress = status["epic"]["progress"]
        assert progress["specs_planned"] == len(SPECS), progress

    @pytest.mark.parametrize("spec", SPECS)
    def test_spec_has_final_plan(self, spec):
        files = cli("plan", "file", "list", spec).get("files") or []
        plan = [f for f in files if Path(f.get("path", "")).name == "plan.md"]
        assert plan, f"{spec} has no plan.md in the plan store: {files}"
        assert plan[0].get("document_status") == "final", (
            f"{spec}'s plan.md is {plan[0].get('document_status')!r}, not final"
        )

    def test_epic_no_longer_reports_unplanned(self, status):
        codes = [p["code"] for p in status["epic"]["run"].get("problems") or []]
        assert "epic_unplanned" not in codes, status["epic"]["run"]["problems"]


# ---------------------------------------------------------------------------
# The orchestration, read from the session log.
# ---------------------------------------------------------------------------


class TestOrchestration:
    def test_session_log_recorded(self, log):
        assert not log.empty, (
            f"no session log under {DEBUG_DIR}: SPEKTACULAR_SESSION_LOG=on, set in "
            "the image, did not reach the agent's spektacular calls"
        )

    @pytest.mark.parametrize("spec", SPECS)
    def test_spec_planned_in_orchestrated_lane(self, log, spec):
        lane = log.lane("plan", spec)
        assert lane is not None and lane.start is not None, (
            f"{spec} was never started in an orchestrated plan lane; the skill "
            "starts each spec with `plan new` carrying \"orchestrated\": true"
        )
        assert lane.start.data().get("orchestrated") is True, lane.start.command

    @pytest.mark.parametrize("spec", SPECS)
    def test_lane_reached_finished(self, log, spec):
        lane = log.lane("plan", spec)
        assert lane is not None and lane.finish is not None, (
            f"{spec}'s plan lane never reached the finished step"
        )

    @pytest.mark.parametrize("spec", SPECS)
    def test_one_child_per_spec(self, log, spec):
        lane = log.lane("plan", spec)
        assert lane is not None
        assert lane.children == 1, (
            f"{spec}'s plan lane was worked by {lane.children} children, each earlier "
            f"one stopping at {', '.join(lane.stops)}. The skill starts one child per "
            "spec; a child cut off by its agent's limit on a sub-agent's run (for "
            "Bob, session.maxTurns) is resumed by a fresh one, which shows here."
        )

    def test_commands_planned_after_storage(self, log):
        storage = log.lane("plan", STORAGE)
        commands = log.lane("plan", COMMANDS)
        assert storage and storage.finish, "storage was never planned to finished"
        assert commands and commands.start, "commands was never started"
        assert commands.start.at >= storage.finish.at, (
            f"{COMMANDS} depends on {STORAGE} and must wait for its plan: it "
            f"started at {commands.start.at:%H:%M:%S}, before storage finished "
            f"at {storage.finish.at:%H:%M:%S}"
        )

    def test_orchestrator_polled_epic_status(self, log):
        assert log.epic_polls(), (
            "the orchestrator never ran `spektacular status <epic>`; the skill "
            "drives its loop from that report"
        )

    def test_epic_order_run(self, log, epic):
        runs = [e for e in log.shared if e.is_verb("epic", "order") and epic in e.command and e.ok]
        assert runs, "`spektacular epic order <epic>` was never run after planning"


# ---------------------------------------------------------------------------
# Child agents, Claude only: only its transcript shows where a child agent
# was started and what it was told, so these checks skip for other agents.
# Every other check reads only CLI state and the session log, and serves
# every agent.
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def children_run():
    import delegation as dl

    if not dl.is_claude():
        pytest.skip("only a Claude transcript shows where a child agent was started")
    return dl.read_run(dl.tx.CLAUDE_TRANSCRIPT, DEBUG_DIR)


def failed(failures: list) -> str:
    return "\n".join(str(f) for f in failures)


class TestChildren:
    def test_every_spec_has_its_own_child(self, children_run):
        import delegation as dl

        failures = dl.missing_spec_children(children_run.placed, list(children_run.windows))
        assert not failures, failed(failures)

    def test_children_are_given_their_context(self, children_run):
        import delegation as dl

        failures = dl.epic_context_gaps(children_run.placed, children_run.session.lanes)
        assert not failures, failed(failures)

    def test_no_wakeup_while_children_run(self, children_run):
        import delegation as dl

        failures = dl.wakeups_while_children_run(children_run.wakeups, children_run.placed, children_run.end)
        assert not failures, failed(failures)


# ---------------------------------------------------------------------------
# The planning summary.
# ---------------------------------------------------------------------------


class TestSummary:
    @pytest.fixture(scope="class")
    def summary(self, epic) -> str:
        result = cli("epic", "summary", "read", epic, parse=False)
        assert result.returncode == 0, f"epic has no planning summary: {result.stdout}"
        return result.stdout

    @staticmethod
    def sections(summary: str) -> dict[str, str]:
        found, current = {}, None
        for line in summary.splitlines():
            if line.startswith("## "):
                current = line[3:].strip()
                found[current] = ""
            elif current is not None:
                found[current] += line + "\n"
        return found

    def test_decisions_section(self, summary):
        assert "Decisions" in self.sections(summary), summary

    @pytest.mark.parametrize("spec", SPECS)
    def test_section_per_spec(self, summary, spec):
        body = self.sections(summary).get(spec)
        assert body is not None, f"the summary has no section for {spec}"
        assert len(body.strip()) > 200, f"{spec}'s summary section is too thin: {body!r}"

    @pytest.mark.parametrize("spec", SPECS)
    def test_section_lists_manual_checks(self, summary, spec):
        body = self.sections(summary).get(spec, "")
        # Case-insensitive: the heading's casing is the agent's prose, and
        # "### Manual Checks" carries the same list.
        assert "### manual checks" in body.lower(), (
            f"{spec}'s summary section has no `### Manual checks` sub-heading"
        )


# ---------------------------------------------------------------------------
# Planning only: the code is untouched.
# ---------------------------------------------------------------------------


def test_code_unchanged():
    # Only the seeded code is checked: init adds the agent's own files at the
    # project root, and those differ per agent.
    result = subprocess.run(
        ["git", "status", "--porcelain", "--", "todo", "README.md"],
        cwd=PROJECT_DIR,
        capture_output=True,
        text=True,
    )
    changed = [line for line in result.stdout.splitlines() if line.strip()]
    assert not changed, f"planning must not change the code, but git reports: {changed}"
