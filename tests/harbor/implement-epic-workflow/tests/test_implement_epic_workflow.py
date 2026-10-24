"""Verifier for the implement-epic-workflow harbor task.

The agent ran the spek-implement-epic skill over a seeded, fully planned epic
of four specs: storage and format independent, commands depending on both,
export depending on commands.

Every assertion reads the project's state through the spektacular CLI, the
git repository, the finished app's behaviour, or the CLI's own debug session
log. None depends on which agent drove the run, so the same verifier serves
Claude, Codex and Bob without parsing a transcript, except the Claude-only
child checks (TestChildren), which read the transcript and skip for other
agents.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from sessionlog import SessionLog

PROJECT_DIR = Path("/app")
DEBUG_DIR = PROJECT_DIR / ".spektacular" / "debug"

STORAGE = "20260201000001-storage"
FORMAT = "20260201000002-format"
COMMANDS = "20260201000003-commands"
EXPORT = "20260201000004-export"
SPECS = [STORAGE, FORMAT, COMMANDS, EXPORT]


def cli(*args: str, parse: bool = True):
    """Run spektacular from the project root, kept out of the session log."""
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


def git(*args: str) -> str:
    result = subprocess.run(["git", *args], cwd=PROJECT_DIR, capture_output=True, text=True)
    assert result.returncode == 0, f"git {' '.join(args)} failed: {result.stderr}"
    return result.stdout


@pytest.fixture(scope="session")
def epic() -> str:
    names = [f["name"] for f in cli("epic", "list").get("files") or []]
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
# Every spec is implemented and merged.
# ---------------------------------------------------------------------------


class TestEverySpecImplemented:
    def test_every_spec_implement_done(self, status):
        states = {s["name"]: s["run"]["implement"]["state"] for s in status["specs"]}
        assert states == {name: "done" for name in SPECS}, (
            f"every spec must be implemented and merged; status reports {states}"
        )

    def test_epic_progress_counts_all_specs(self, status):
        assert status["epic"]["progress"]["specs_implemented"] == len(SPECS), status["epic"]["progress"]

    @pytest.mark.parametrize("spec", SPECS)
    def test_spec_has_changelog(self, spec):
        files = cli("changelog", "file", "list").get("files") or []
        names = [f.get("name", "") for f in files]
        assert any(spec in n for n in names), f"no changelog record for {spec} among {names}"


class TestMergedToMainLine:
    def test_no_spec_branches_left(self):
        branches = git("branch", "--list", "spek/*").split()
        assert not branches, f"a merged spec's branch must be removed, found {branches}"

    def test_no_worktrees_left(self):
        worktrees = [l for l in git("worktree", "list", "--porcelain").splitlines() if l.startswith("worktree ")]
        assert worktrees == [f"worktree {PROJECT_DIR}"], (
            f"a merged spec's worktrees must be removed, found {worktrees}"
        )

    @pytest.mark.parametrize("path", ["todo/storage.py", "todo/format.py", "todo/export.py"])
    def test_spec_module_on_main_line(self, path):
        tracked = git("ls-files", path).strip()
        assert tracked == path, f"{path} is not committed on the main line"

    def test_no_uncommitted_code(self):
        changed = [l for l in git("status", "--porcelain", "--", "todo", "tests", "README.md").splitlines() if l.strip()]
        assert not changed, f"every spec's code must arrive by its merge, but git reports: {changed}"


# ---------------------------------------------------------------------------
# The finished app does what the specs say.
# ---------------------------------------------------------------------------


@pytest.fixture
def todo(tmp_path):
    home = tmp_path / "home"
    home.mkdir()
    store = tmp_path / "todos.json"

    def run(*args: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, "-m", "todo", *args],
            cwd=PROJECT_DIR,
            capture_output=True,
            text=True,
            env={**os.environ, "TODO_FILE": str(store), "HOME": str(home)},
        )

    run.store = store
    run.home = home
    return run


class TestBehaviour:
    def test_empty_list(self, todo):
        r = todo("list")
        assert r.returncode == 0, r.stderr
        assert r.stdout.strip() == "No todos."

    def test_add_list_done(self, todo):
        r = todo("add", "buy milk")
        assert (r.returncode, r.stdout.strip()) == (0, "Added 1"), (r.stdout, r.stderr)
        assert todo("list").stdout.strip() == "1. [ ] buy milk"
        r = todo("done", "1")
        assert (r.returncode, r.stdout.strip()) == (0, "Done 1"), (r.stdout, r.stderr)
        assert todo("list").stdout.strip() == "1. [x] buy milk"

    def test_unknown_id(self, todo):
        r = todo("done", "99")
        assert r.returncode == 1, (r.stdout, r.stderr)
        assert "No todo 99" in r.stderr

    def test_storage_honours_todo_file(self, todo):
        todo("add", "buy milk")
        assert todo.store.exists(), "the todo list must be saved to TODO_FILE"
        assert not (todo.home / ".todo.json").exists(), "with TODO_FILE set, ~/.todo.json must not be touched"

    def test_export(self, todo):
        todo("add", "buy milk")
        todo("done", "1")
        todo("add", "walk dog")
        r = todo("export")
        assert r.returncode == 0, r.stderr
        assert r.stdout == "- [x] buy milk\n- [ ] walk dog\n"

    def test_export_empty(self, todo):
        r = todo("export")
        assert (r.returncode, r.stdout) == (0, ""), (r.stdout, r.stderr)

    def test_help_names_each_command(self, todo):
        r = todo("--help")
        assert r.returncode == 0, r.stderr
        for command in ("add", "list", "done"):
            assert command in r.stdout, f"`todo --help` does not name {command}:\n{r.stdout}"

    def test_readme_documents_usage(self):
        readme = (PROJECT_DIR / "README.md").read_text()
        assert "## Usage" in readme, "README.md has no `## Usage` section"
        usage = readme.split("## Usage", 1)[1]
        for command in ("todo add", "todo list", "todo done"):
            assert command in usage, f"the README's Usage section does not show `{command}`"

    def test_project_tests_pass(self):
        r = subprocess.run(
            [sys.executable, "-m", "unittest", "discover", "-s", "tests"],
            cwd=PROJECT_DIR,
            capture_output=True,
            text=True,
        )
        assert r.returncode == 0, f"the project's own tests fail:\n{r.stderr[-2000:]}"


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
    def test_worktree_created(self, log, spec):
        made = [e for e in log.shared if e.is_verb("epic", "worktree") and e.ok and e.data().get("spec") == spec]
        assert made, f"{spec} was never given its own worktrees with `epic worktree`"

    @pytest.mark.parametrize("spec", SPECS)
    def test_spec_in_orchestrated_lane(self, log, spec):
        lane = log.lane("implement", spec)
        assert lane is not None and lane.start is not None, (
            f"{spec} was never started in an orchestrated implement lane"
        )
        assert lane.start.data().get("orchestrated") is True, lane.start.command

    @pytest.mark.parametrize("spec", SPECS)
    def test_lane_reached_finished(self, log, spec):
        lane = log.lane("implement", spec)
        assert lane is not None and lane.finish is not None, f"{spec}'s implement lane never reached finished"

    @pytest.mark.parametrize("spec", SPECS)
    def test_one_child_per_spec(self, log, spec):
        lane = log.lane("implement", spec)
        assert lane is not None
        assert lane.children == 1, (
            f"{spec} needed {lane.children} children: each earlier one stopped "
            f"without finishing, at {', '.join(lane.stops)}. A child stopping at "
            "varied steps was cut off by its agent's limit on a sub-agent's run "
            "(for Bob, session.maxTurns), not stopped by the workflow."
        )

    def test_storage_and_format_ran_in_parallel(self, log):
        s, f = log.lane("implement", STORAGE), log.lane("implement", FORMAT)
        assert s and s.start and s.finish and f and f.start and f.finish, "storage and format must both run to finished"
        assert s.start.at < f.finish.end and f.start.at < s.finish.end, (
            f"storage ({s.start.at:%H:%M:%S} to {s.finish.end:%H:%M:%S}) and format "
            f"({f.start.at:%H:%M:%S} to {f.finish.end:%H:%M:%S}) depend on nothing and "
            "must be implemented at the same time"
        )

    def test_commands_started_after_both_merged(self, log):
        commands = log.lane("implement", COMMANDS)
        assert commands and commands.start, "commands was never started"
        for dep in (STORAGE, FORMAT):
            merge = log.merge(dep)
            assert merge, f"{dep} was never merged"
            assert commands.start.at >= merge.end, (
                f"{COMMANDS} started at {commands.start.at:%H:%M:%S}, before its "
                f"dependency {dep} was merged at {merge.end:%H:%M:%S}"
            )

    def test_export_started_after_commands_merged(self, log):
        export = log.lane("implement", EXPORT)
        merge = log.merge(COMMANDS)
        assert export and export.start, "export was never started"
        assert merge, f"{COMMANDS} was never merged"
        assert export.start.at >= merge.end, (
            f"{EXPORT} started at {export.start.at:%H:%M:%S}, before {COMMANDS} was "
            f"merged at {merge.end:%H:%M:%S}"
        )

    @pytest.mark.parametrize("spec", SPECS)
    def test_spec_merged(self, log, spec):
        assert log.merge(spec), f"{spec} was never merged with `epic merge`"

    def test_no_merge_refused(self, log):
        refused = [" ".join(e.command) for e in log.shared if e.is_verb("epic", "merge") and not e.ok]
        assert not refused, f"`epic merge` was refused: {refused}"


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

        failures = dl.epic_context_gaps(children_run.placed, children_run.session.lanes, worktrees=dl.worktree_paths(children_run.session))
        assert not failures, failed(failures)

    def test_no_wakeup_while_children_run(self, children_run):
        import delegation as dl

        failures = dl.wakeups_while_children_run(children_run.wakeups, children_run.placed, children_run.end)
        assert not failures, failed(failures)

    def test_children_run_as_the_probe_settled(self, children_run):
        """Headless Claude Code stops a background child once the turn ends
        (the background-child probe's recorded outcome), so each spec's child
        runs in the foreground and no turn ends while one is running."""
        import delegation as dl

        failures = dl.epic_children_run_as_settled(children_run.placed, children_run.turns, list(children_run.windows))
        assert not failures, failed(failures)

    def test_required_steps_each_started_a_child(self, children_run):
        """In every spec's lane, each goal-check step ran in a child agent of
        the spec's own child: checking the goal inline does not complete it."""
        import delegation as dl

        failures = dl.missing_required(children_run.placed, children_run.windows)
        assert not failures, failed(failures)


# ---------------------------------------------------------------------------
# Every task's goal was checked before it was ticked.
# ---------------------------------------------------------------------------


def plan_tasks(spec: str) -> list[dict]:
    out = cli("status", spec, "--format", "json")
    for s in out.get("specs") or []:
        if s.get("name") == spec:
            return (s.get("plan") or {}).get("tasks") or []
    return []


class TestGoalCheck:
    @pytest.mark.parametrize("spec", SPECS)
    def test_every_task_has_recorded_verdicts(self, spec):
        outcome = cli("implement", "goal", "show", spec, "--format", "json")
        assert not outcome.get("error"), outcome
        recorded = {item["item"]: item for item in outcome.get("items") or []}
        tasks = plan_tasks(spec)
        assert tasks, f"{spec}'s plan lists no tasks"
        for task in tasks:
            item = recorded.get(task["id"])
            assert item and item.get("checks"), (
                f"{spec} task {task['id']} ({task['title']}) has no recorded goal check: "
                "every task's criteria must be judged by the reviewer before it is ticked"
            )
            want = task["acceptance_criteria"]["total"]
            for check in item["checks"]:
                verdicts = check.get("verdicts") or []
                assert len(verdicts) == want, (
                    f"{spec} task {task['id']}: a check holds {len(verdicts)} verdicts for {want} criteria"
                )
                assert all(v.get("evidence", "").strip() for v in verdicts), (
                    f"{spec} task {task['id']}: a verdict has no evidence"
                )

    @pytest.mark.parametrize("spec", SPECS)
    def test_goal_record_precedes_update_plan(self, log, spec):
        lane = log.lane("implement", spec)
        assert lane is not None, f"{spec} has no implement lane in the session log"
        recorded = 0
        for e in lane.events:
            if e.ok and (e.is_verb("implement", "goal", "record") or e.is_verb("implement", "goal", "decide")):
                recorded += 1
            elif e.ok and e.goto_step == "update_plan" and (e.state_before or {}).get("current_step") != "update_plan":
                assert recorded > 0, (
                    f"{spec}'s lane reached update_plan at {e.at:%H:%M:%S} with no goal check "
                    "recorded since the previous plan update"
                )
                recorded = 0
