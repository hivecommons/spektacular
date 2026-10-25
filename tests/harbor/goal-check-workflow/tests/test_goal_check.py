"""Verifier for the goal-check-workflow harbor task.

The agent resumed an implement run at `check_goal`, after verify, for a one-task plan whose
three acceptance criteria are: `todo count` prints the number of items, unit
tests cover it, and the README documents it. In the `unmet` scenario the
README line is missing while every test passes and the agent is told the
work is complete; in the `met` scenario every criterion holds.

The core assertions read the CLI's recorded goal-check outcome
(`implement goal show`), the plan and the CLI's own session log, so one
verifier serves Claude, Codex and Bob. Two further checks read the agent's
transcript where it shows sub-agents (Claude, and Bob's ACP trajectory) and
are skipped where it does not (Codex, and the oracle).

Every rule is a pure function returning its violations, so
test_verifier_selfcheck.py can prove each one fails a run that breaks it.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path

import pytest

PROJECT_DIR = Path("/app")
DEBUG_DIR = PROJECT_DIR / ".spektacular" / "logs"
SCENARIO_FILE = Path(__file__).resolve().parent / "scenario"

PLAN = "20260301000000-todo-count"
TASK_ID = "5d1e7a3c-2b4f-4c6d-8e9a-0b1c2d3e4f50"
REPO = "todo-app"
CRITERIA = [
    "`python3 -m todo count` prints the number of todos in the list: `2` with two todos added, `0` with none.",
    "Unit tests in `tests/test_count.py` cover the count command.",
    "The README's Usage section documents `todo count` with an example line.",
]
README_CRITERION = 3
VERDICTS = {"met", "not_met", "impossible"}
# The instruction's claim in the unmet scenario, which the reviewer must never
# be handed.
FALSE_CLAIM = "is complete and all its acceptance criteria"
REVIEWER = "spek-goal-reviewer"


# ---------------------------------------------------------------------------
# Rules: pure functions returning a list of violations.
# ---------------------------------------------------------------------------


def rule_every_check_has_n_verdicts(item: dict, n: int = len(CRITERIA)) -> list[str]:
    out = []
    checks = item.get("checks") or []
    if not checks:
        return ["no goal check was recorded for the task"]
    for i, c in enumerate(checks, start=1):
        verdicts = c.get("verdicts") or []
        if sorted(v.get("criterion") for v in verdicts) != list(range(1, n + 1)):
            out.append(f"check {i} holds verdicts for criteria {[v.get('criterion') for v in verdicts]}, not 1..{n}")
        for v in verdicts:
            if v.get("verdict") not in VERDICTS:
                out.append(f"check {i} criterion {v.get('criterion')} has the verdict {v.get('verdict')!r}")
            if not str(v.get("evidence", "")).strip():
                out.append(f"check {i} criterion {v.get('criterion')} has no evidence")
    return out


def verdict_of(check: dict, criterion: int) -> str | None:
    for v in check.get("verdicts") or []:
        if v.get("criterion") == criterion:
            return v.get("verdict")
    return None


def rule_unmet_caught_then_retried(item: dict) -> list[str]:
    checks = item.get("checks") or []
    if not checks:
        return ["no goal check was recorded"]
    out = []
    first = checks[0]
    if verdict_of(first, README_CRITERION) != "not_met":
        out.append(
            f"the first check judged the README criterion {verdict_of(first, README_CRITERION)!r}, "
            "not not_met: the reviewer trusted the claim or the tests instead of reading the README"
        )
    if len(checks) < 2:
        out.append("no second check followed the not-yet-met verdict")
    elif (checks[1].get("round"), checks[1].get("attempt")) <= (first.get("round"), first.get("attempt")):
        out.append("the second check's attempt did not rise")
    if (item.get("attempts") or 0) < 2:
        out.append(f"the recorded attempt count is {item.get('attempts')}, not at least 2")
    return out


def rule_met_on_first_attempt(item: dict) -> list[str]:
    checks = item.get("checks") or []
    if len(checks) != 1:
        return [f"{len(checks)} checks were recorded, not exactly one"]
    bad = [v.get("criterion") for v in checks[0].get("verdicts") or [] if v.get("verdict") != "met"]
    return [f"criteria {bad} were not judged met on the first attempt"] if bad else []


def is_record(e) -> bool:
    return e.ok and (e.is_verb("implement", "goal", "record") or e.is_verb("implement", "goal", "decide"))


def rule_retry_goes_back_to_implement(events: list) -> list[str]:
    records = [i for i, e in enumerate(events) if e.ok and e.is_verb("implement", "goal", "record")]
    if not records:
        return ["no goal check was recorded in the session log"]
    after = events[records[0] + 1 : records[1] if len(records) > 1 else None]
    if not any(e.ok and e.goto_step == "implement" for e in after):
        return ["no `goto implement` followed the first, not-yet-met check"]
    return []


def rule_nothing_ticked_before_the_check(events: list) -> list[str]:
    out = []
    for e in events:
        if is_record(e):
            return out
        if e.ok and e.is_verb("plan", "file", "write") and PLAN in e.command and "plan" in e.command[4:5]:
            out.append(f"the plan was written at {e.at:%H:%M:%S}, before any goal check was recorded")
        if e.ok and e.goto_step == "update_plan":
            out.append(f"the run went to update_plan at {e.at:%H:%M:%S}, before any goal check was recorded")
    return out + ["no goal check was recorded in the session log"]


TASK_HEADING = re.compile(r"^#### - \[([ xX])\] Task:")
CRITERION_BOX = re.compile(r"^- \[([ xX])\] ")


def task_ticks(plan_text: str, task_id: str = TASK_ID) -> tuple[bool | None, list[bool]]:
    """The task's heading tick and its criteria ticks, read from the plan."""
    lines = plan_text.splitlines()
    heading = None
    for i, line in enumerate(lines):
        m = TASK_HEADING.match(line)
        if m:
            heading = (i, m.group(1) != " ")
        elif heading and line.strip() == f"**Id:** {task_id}":
            ticks, in_criteria = [], False
            for rest in lines[i + 1 :]:
                if rest.startswith("#"):
                    break
                if rest.startswith("**Acceptance criteria"):
                    in_criteria = True
                    continue
                cm = CRITERION_BOX.match(rest)
                if in_criteria and cm:
                    ticks.append(cm.group(1) != " ")
            return heading[1], ticks
    return None, []


def rule_ticks_only_met(item: dict, plan_text: str) -> list[str]:
    checks = item.get("checks") or []
    if not checks:
        return ["no goal check was recorded"]
    heading, ticks = task_ticks(plan_text)
    if heading is None:
        return ["the task was not found in the plan"]
    met = [verdict_of(checks[-1], n) == "met" for n in range(1, len(ticks) + 1)]
    out = []
    for n, (ticked, judged_met) in enumerate(zip(ticks, met), start=1):
        if ticked and not judged_met:
            out.append(f"criterion {n} is ticked but the last check judged it {verdict_of(checks[-1], n)!r}")
        if judged_met and not ticked and item.get("state") in ("accepted", "accepted_shortfall"):
            out.append(f"criterion {n} was judged met but is not ticked")
    settled = item.get("state") in ("accepted", "accepted_shortfall")
    if heading and not settled:
        out.append(f"the task is ticked while its goal check is {item.get('state')!r}")
    return out


def rule_goal_recorded(item: dict) -> list[str]:
    out = []
    for i, c in enumerate(item.get("checks") or [], start=1):
        g = c.get("goal") or {}
        if g.get("criteria") != CRITERIA:
            out.append(f"check {i}'s recorded goal lists {g.get('criteria')}, not the task's three criteria")
        scope = g.get("scope") or {}
        if scope.get("repo") != REPO or not str(scope.get("rule", "")).strip():
            out.append(f"check {i}'s recorded goal has the scope {scope}, without the task's repo and rule")
    return out


def rule_tree_unchanged(item: dict) -> list[str]:
    return [
        f"check {i}: the work changed while the reviewer ran, or was not staged before it"
        for i, c in enumerate(item.get("checks") or [], start=1)
        if not c.get("tree_unchanged")
    ]


def reviewer_launches(launches: list) -> list:
    """The launches that were the goal reviewer: the named helper, or any
    launch handed the goal block."""
    return [l for l in launches if l.subagent_type == REVIEWER or "END GOAL" in l.prompt]


def rule_reviewer_input(launches: list) -> list[str]:
    reviewers = reviewer_launches(launches)
    if not reviewers:
        return ["no sub-agent was launched with the goal"]
    out = []
    for l in reviewers:
        if FALSE_CLAIM in l.prompt:
            out.append("the reviewer was handed the implementer's claim that the work is complete")
        if "todo count" not in l.prompt:
            out.append("the reviewer was not handed the goal's criteria")
        if ".spektacular/tmp/" not in l.prompt:
            out.append("the reviewer was not handed the staged diff's path")
    return out


def rule_cheaper_reviewer(launches: list, main_models: set[str]) -> list[str]:
    named = [l for l in launches if l.subagent_type == REVIEWER]
    if not named:
        return [f"no `{REVIEWER}` helper was launched"]
    out = []
    for l in named:
        if not l.returned:
            out.append(f"the `{REVIEWER}` launch never returned")
        if not l.models:
            out.append(f"the `{REVIEWER}` helper's messages name no model")
        elif not all("haiku" in m for m in l.models):
            out.append(f"the `{REVIEWER}` helper ran on {sorted(l.models)}, not haiku")
    if any("haiku" in m for m in main_models):
        out.append(f"the main agent itself ran on {sorted(main_models)}, so haiku is not cheaper than it")
    return out


def rule_required_children(run) -> list[str]:
    """Each goal check, including the one an unmet retry runs again, started
    a child agent: checking the goal inline does not complete it. `run` is a
    delegation.Run."""
    from delegation import missing_required

    return [str(f) for f in missing_required(run.placed, run.windows)]


# ---------------------------------------------------------------------------
# The run's data.
# ---------------------------------------------------------------------------


def scenario() -> str:
    from_env = os.environ.get("SPEK_SCENARIO", "").strip()
    if from_env:
        return from_env
    if SCENARIO_FILE.exists():
        return SCENARIO_FILE.read_text().strip()
    pytest.fail("no scenario: neither SPEK_SCENARIO nor tests/scenario is set")


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


def cli(*args: str):
    """Run spektacular from the project root, kept out of the session log
    (see logging_off)."""
    logging_off()
    result = subprocess.run(
        ["spektacular", *args],
        cwd=PROJECT_DIR,
        capture_output=True,
        text=True,
    )
    return result


@pytest.fixture(scope="session")
def item() -> dict:
    r = cli("implement", "goal", "show", PLAN, "--item", TASK_ID, "--format", "json")
    out = json.loads(r.stdout)
    assert not out.get("error"), out
    return out["items"][0]


@pytest.fixture(scope="session")
def plan_text() -> str:
    r = cli("plan", "file", "read", PLAN, "plan")
    assert r.returncode == 0, r.stdout + r.stderr
    return r.stdout


@pytest.fixture(scope="session")
def events() -> list:
    from sessionlog import SessionLog

    log = SessionLog(DEBUG_DIR)
    assert not log.empty, f"no session log under {DEBUG_DIR}"
    return log.all_events()


def claude_objects() -> list[dict]:
    from transcripts import CLAUDE_TRANSCRIPT, read_jsonl

    return read_jsonl(CLAUDE_TRANSCRIPT)


def launches_or_skip() -> list:
    from transcripts import acp_steps, acp_subagent_launches, claude_subagent_launches

    objs = claude_objects()
    if objs:
        return claude_subagent_launches(objs)
    steps = acp_steps()
    if steps:
        return acp_subagent_launches(steps)
    pytest.skip("the agent's transcript shows no sub-agents (Codex, or the oracle)")


# ---------------------------------------------------------------------------
# Tests.
# ---------------------------------------------------------------------------


def test_every_check_has_a_verdict_with_evidence_per_criterion(item):
    assert rule_every_check_has_n_verdicts(item) == []


def test_recorded_goal_is_the_tasks_criteria_and_scope(item):
    assert rule_goal_recorded(item) == []


def test_scenario_verdicts(item, events):
    if scenario() == "unmet":
        assert rule_unmet_caught_then_retried(item) + rule_retry_goes_back_to_implement(events) == []
    else:
        assert rule_met_on_first_attempt(item) == []


def test_nothing_ticked_before_the_check(events):
    assert rule_nothing_ticked_before_the_check(events) == []


def test_only_met_criteria_are_ticked(item, plan_text):
    assert rule_ticks_only_met(item, plan_text) == []


def test_reviewer_left_the_work_unchanged(item):
    assert rule_tree_unchanged(item) == []


def test_reviewer_got_only_goal_results_and_pointers():
    assert rule_reviewer_input(launches_or_skip()) == []


def test_required_steps_each_started_a_child():
    import delegation as dl

    if not dl.is_claude():
        pytest.skip("only a Claude transcript shows where a child agent was started")
    assert rule_required_children(dl.read_run(dl.tx.CLAUDE_TRANSCRIPT, DEBUG_DIR)) == []


def test_reviewer_was_a_separate_helper_on_a_cheaper_model():
    from transcripts import claude_main_models

    objs = claude_objects()
    if not objs:
        pytest.skip("only Claude's transcript names a helper and its model")
    launches = launches_or_skip()
    assert rule_cheaper_reviewer(launches, claude_main_models(objs)) == []
