"""Self-check for the session-log rules the verifiers judge what ran by.

`sessionlog.plan_steps_entered` reads the steps a plan workflow entered from
Spektacular's own debug log: the step each successful `plan new` or
`plan goto` left the workflow at, in the order they ran. The plan verifier's
walkthrough-before-finished check now reads that, rather than the agent's
transcript, so a goto piped into another command or chained after a file
write counts the same as a bare one. The cases here pin the rule on the 5.5
plan reference run (whose verification, write and finished advances were
chained and piped), on small hand-built lanes, and through the verifier's own
test function. They also pin that a plan run whose discovery started no child
no longer fails a spawn check, and that its metrics record says so instead.

`sessionlog.changelog_writes_by_repo` counts the successful
`changelog file write <name>` calls per `--repo`, also from the debug log. The
implement verifier's three changelog-write checks read it, so the 5.5
implement reference run, which wrote all three records in one shell loop,
passes them, while a run with no successful write of a record still fails
that record's check alone.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_sessionlog_rules.py
"""

import json
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
SHARED = HERE.parent
PLAN_VERIFIER = SHARED.parent / "plan-workflow" / "tests"
IMPLEMENT_VERIFIER = SHARED.parent / "implement-workflow" / "tests"
sys.path.insert(0, str(SHARED))
sys.path.insert(0, str(PLAN_VERIFIER))
sys.path.insert(0, str(IMPLEMENT_VERIFIER))

import metrics  # noqa: E402
import test_implement_workflow  # noqa: E402  (the implement suite's verifier module)
import test_plan_workflow  # noqa: E402  (the plan suite's verifier module)
from sessionlog import SessionLog, changelog_writes_by_repo, plan_steps_entered, read_events  # noqa: E402

DATA = HERE / "data"
S55_PLAN = DATA / "s55-plan"
S55_IMPLEMENT = DATA / "s55-implement"
NONE = Path("/nonexistent")
NAME = "20260101000000-user-auth"
SONNET = "claude-sonnet-4-6"

# Every step the 5.5 plan run entered, read by hand from its plan lane file:
# the step each successful `plan new` / `plan goto` left it at. Its failed
# out_of_scope goto, its `plan file` reads and writes, and its other commands
# are not in the list.
S55_PLAN_STEPS = [
    "overview",
    "discovery",
    "architecture",
    "components",
    "data_structures",
    "implementation_detail",
    "dependencies",
    "testing_approach",
    "milestones",
    "tasks",
    "open_questions",
    "out_of_scope",
    "assemble",
    "verification",
    "write_plan",
    "write_context",
    "write_research",
    "walkthrough",
    "finished",
]

# The steps of a plan run up to its last document write, shared by the
# synthetic lanes below.
UP_TO_WRITE_RESEARCH = S55_PLAN_STEPS[: S55_PLAN_STEPS.index("write_research") + 1]


def walkthrough_before_finished(entered: list[str]) -> bool:
    """The verifier's rule: walkthrough and finished both entered, and
    walkthrough first."""
    return "walkthrough" in entered and "finished" in entered and entered.index("walkthrough") < entered.index("finished")


# --- synthetic session logs ----------------------------------------------------


def ts(seconds: int) -> str:
    """A fixed synthetic CLI timestamp (ns), seconds after 10:00:00 on the test day."""
    m, s = divmod(seconds, 60)
    return f"2026-10-23T10:{m:02d}:{s:02d}.000000000Z"


def state(step):
    return {"kind": "plan", "name": NAME, "current_step": step}


def new():
    return ["plan", "new", "--data", json.dumps({"name": NAME})]


def goto(step):
    return ["plan", "goto", "--data", json.dumps({"step": step, "name": NAME})]


def file_write(file):
    return ["plan", "file", "write", "--data", json.dumps({"name": NAME, "file": file})]


def record(seconds, command, before, after, *, exit_code=0):
    return {
        "timestamp": ts(seconds),
        "command": command,
        "duration_ms": 0,
        "exit_code": exit_code,
        "state_before": before,
        "state_after": after,
    }


def walk(steps):
    """One record per step: `plan new` onto the first, a goto onto each next,
    ten seconds apart."""
    out = [record(0, new(), None, state(steps[0]))]
    for i, step in enumerate(steps[1:], start=1):
        out.append(record(10 * i, goto(step), state(steps[i - 1]), state(step)))
    return out


def write_lane(debug_dir: Path, records, *, session=f"plan_{NAME}") -> Path:
    """One debug-log file named as the CLI names it."""
    debug_dir.mkdir(parents=True, exist_ok=True)
    path = debug_dir / f"20261023T100000.903970323Z_claude_{session}.jsonl"
    path.write_text("".join(json.dumps(r) + "\n" for r in records))
    return path


def lane_events(debug_dir: Path):
    session = SessionLog(debug_dir, kinds=("plan",))
    return [e for lane in session.lanes.values() for e in lane.events]


# --- plan_steps_entered ----------------------------------------------------------


def test_s55_plan_run_enters_walkthrough_before_finished():
    entered = plan_steps_entered(lane_events(S55_PLAN / "debug"))
    assert entered == S55_PLAN_STEPS
    assert walkthrough_before_finished(entered)


def test_lane_going_straight_from_writes_to_finished_has_no_walkthrough(tmp_path):
    write_lane(tmp_path, walk([*UP_TO_WRITE_RESEARCH, "finished"]))
    entered = plan_steps_entered(lane_events(tmp_path))
    assert entered == [*UP_TO_WRITE_RESEARCH, "finished"]
    assert "walkthrough" not in entered
    assert not walkthrough_before_finished(entered)


def test_lane_finishing_before_its_walkthrough_has_the_wrong_order(tmp_path):
    write_lane(tmp_path, walk([*UP_TO_WRITE_RESEARCH, "finished", "walkthrough"]))
    entered = plan_steps_entered(lane_events(tmp_path))
    assert entered == [*UP_TO_WRITE_RESEARCH, "finished", "walkthrough"]
    assert not walkthrough_before_finished(entered)


def test_failed_gotos_and_file_commands_are_not_steps_entered(tmp_path):
    write_lane(
        tmp_path,
        [
            record(0, new(), None, state("overview")),
            # refused: the workflow stays at overview, though the log names a step
            record(10, goto("finished"), state("overview"), state("overview"), exit_code=1),
            # a file write carries a state too, but enters nothing
            record(20, file_write("plan.md"), state("overview"), state("write_plan")),
            record(30, ["knowledge", "search", "auth"], state("overview"), state("tasks")),
            record(40, goto("discovery"), state("overview"), state("discovery")),
        ],
    )
    assert plan_steps_entered(read_events(next(tmp_path.glob("*.jsonl")))) == ["overview", "discovery"]


def test_goto_without_a_state_after_falls_back_to_its_data_step(tmp_path):
    write_lane(
        tmp_path,
        [
            record(0, new(), None, state("overview")),
            record(10, goto("discovery"), state("overview"), None),
        ],
    )
    assert plan_steps_entered(read_events(next(tmp_path.glob("*.jsonl")))) == ["overview", "discovery"]


def test_steps_entered_follow_the_time_they_ran_not_the_list_order(tmp_path):
    write_lane(tmp_path, walk(["overview", "discovery", "architecture"]))
    events = read_events(next(tmp_path.glob("*.jsonl")))
    assert plan_steps_entered(list(reversed(events))) == ["overview", "discovery", "architecture"]


# --- the plan verifier's own check -------------------------------------------------


def run_walkthrough_check(monkeypatch, debug_dir: Path) -> None:
    monkeypatch.setattr(test_plan_workflow, "DEBUG_DIR", debug_dir)
    test_plan_workflow.TestInstructionNextStepValidity().test_walkthrough_rendered_before_finished()


def test_verifier_passes_the_s55_plan_run(monkeypatch):
    run_walkthrough_check(monkeypatch, S55_PLAN / "debug")


@pytest.mark.parametrize(
    "steps",
    [
        [*UP_TO_WRITE_RESEARCH, "finished"],
        [*UP_TO_WRITE_RESEARCH, "finished", "walkthrough"],
        [*UP_TO_WRITE_RESEARCH, "walkthrough"],
    ],
    ids=["skips-walkthrough", "finished-first", "never-finished"],
)
def test_verifier_fails_a_lane_without_walkthrough_before_finished(monkeypatch, tmp_path, steps):
    write_lane(tmp_path, walk(steps))
    with pytest.raises(AssertionError):
        run_walkthrough_check(monkeypatch, tmp_path)


def test_verifier_fails_when_no_plan_lane_was_logged(monkeypatch, tmp_path):
    # the same walk filed in the shared log is no plan lane
    write_lane(tmp_path, walk(S55_PLAN_STEPS), session="no-active-workflow")
    with pytest.raises(AssertionError):
        run_walkthrough_check(monkeypatch, tmp_path)


def test_verifier_no_longer_checks_discovery_spawns_a_child():
    assert not hasattr(test_plan_workflow, "TestSubAgentSpawning")
    assert not hasattr(test_plan_workflow, "EXPECTED_SPAWN_STEPS")


# --- the record says how many children discovery started -------------------------------


def record_steps(record_):
    return [[(s["step"], s["children"]) for s in w["steps"]] for w in record_["steps"]["workflows"]]


def test_s55_plan_record_counts_the_child_discovery_started():
    rec = metrics.build_record(
        metrics.Paths(
            build_info=S55_PLAN / "build.json",
            agent_dir=S55_PLAN,
            debug_dir=S55_PLAN / "debug",
            bob_logs=NONE,
            junit=NONE,
            reward=NONE,
        )
    )
    # one Explore child, launched during discovery; no other step started one
    assert record_steps(rec) == [[(step, 1 if step == "discovery" else 0) for step in S55_PLAN_STEPS]]
    discovery = next(s for s in rec["steps"]["workflows"][0]["steps"] if s["step"] == "discovery")
    assert type(discovery["children"]) is int


def claude_assistant(clock, content):
    return {
        "type": "assistant",
        "timestamp": f"2026-10-23T{clock}Z",
        "parent_tool_use_id": None,
        "message": {"model": SONNET, "role": "assistant", "content": [{"type": "text", "text": content}]},
    }


def claude_plan_log_without_children():
    """A single-agent Claude plan run that launches no sub-agent: one message
    in each step of the lane `walk` writes for overview, discovery,
    architecture, finished (10:00:00, :10, :20, :30)."""
    return [
        {"type": "system", "subtype": "init", "model": SONNET},
        claude_assistant("10:00:05", "overview"),
        claude_assistant("10:00:15", "discovery, worked alone"),
        claude_assistant("10:00:25", "architecture"),
        claude_assistant("10:00:30.500", "done"),
        {
            "type": "result",
            "subtype": "success",
            "num_turns": 4,
            "duration_ms": 40000,
            "duration_api_ms": 20000,
            "total_cost_usd": 0.5,
            "modelUsage": {SONNET: {"inputTokens": 10, "outputTokens": 20, "cacheReadInputTokens": 0, "cacheCreationInputTokens": 0, "costUSD": 0.5}},
            "subagent_stats": {"spawned": 0, "completed": 0, "failed": 0, "killed": {"parent": 0, "user": 0, "system": 0}},
        },
    ]


def test_plan_run_whose_discovery_started_no_child_records_zero(tmp_path):
    agent_dir = tmp_path / "agent"
    agent_dir.mkdir()
    (agent_dir / "claude-code.txt").write_text("".join(json.dumps(o) + "\n" for o in claude_plan_log_without_children()))
    build = tmp_path / "build.json"
    build.write_text(json.dumps({"suite": "plan-workflow", "scenario": "default", "agent": "claude"}))
    debug_dir = tmp_path / "debug"
    write_lane(debug_dir, walk(["overview", "discovery", "architecture", "finished"]))

    rec = metrics.build_record(
        metrics.Paths(build_info=build, agent_dir=agent_dir, debug_dir=debug_dir, bob_logs=NONE, junit=NONE, reward=NONE)
    )
    # zero, not unknown: the run's children are known and none was started
    assert record_steps(rec) == [[("overview", 0), ("discovery", 0), ("architecture", 0), ("finished", 0)]]
    assert rec["subagents"]["children"] == []
    assert not [u for u in rec["unknown"] if u["field"].startswith(("steps.children", "subagents.children"))]


# --- changelog_writes_by_repo ------------------------------------------------------

JWT = "20260101000000-jwt-auth"


def changelog_write(*args):
    return ["changelog", "file", "write", *args]


def implement_events(debug_dir: Path):
    return SessionLog(debug_dir, kinds=("implement",)).all_events()


def write_implement_lane(debug_dir: Path, commands) -> None:
    """One implement lane holding the given (command, exit code) pairs, a
    second apart."""
    write_lane(
        debug_dir,
        [record(i, cmd, None, None, exit_code=code) for i, (cmd, code) in enumerate(commands)],
        session=f"implement_{JWT}",
    )


def test_s55_implement_run_wrote_each_record_once():
    # Read by hand from the lane file: the loop's first pass and a /dev/null
    # probe failed (exit 1), `changelog file write --help` succeeded but names
    # no record, then the loop's second pass wrote all three.
    events = implement_events(S55_IMPLEMENT / "debug")
    failed = [e for e in events if e.is_verb("changelog", "file", "write") and not e.ok]
    assert len(failed) == 4
    assert changelog_writes_by_repo(events, JWT) == {None: 1, "auth": 1, "docs": 1}
    # with no name, every record the run wrote counts, the same three here
    assert changelog_writes_by_repo(events) == {None: 1, "auth": 1, "docs": 1}


def test_a_failed_write_is_not_counted(tmp_path):
    write_implement_lane(
        tmp_path,
        [
            (changelog_write(JWT, "--from", "p.md"), 1),
            (changelog_write(JWT, "--repo", "docs", "--from", "d.md"), 1),
            (changelog_write(JWT, "--repo", "auth", "--from", "a.md"), 0),
        ],
    )
    assert changelog_writes_by_repo(implement_events(tmp_path), JWT) == {"auth": 1}


@pytest.mark.parametrize(
    "command",
    [
        changelog_write(JWT, "--repo=docs", "--from", "d.md"),
        changelog_write("--repo", "docs", JWT, "--from", "d.md"),
        changelog_write("--from", "d.md", "--repo", "docs", JWT),
        changelog_write("--data", "{}", "--from=d.md", JWT, "--repo", "docs"),
    ],
    ids=["repo-equals", "flags-before-name", "from-before-repo", "data-and-from-equals"],
)
def test_a_write_is_read_however_its_flags_are_spelled(tmp_path, command):
    write_implement_lane(tmp_path, [(command, 0)])
    assert changelog_writes_by_repo(implement_events(tmp_path), JWT) == {"docs": 1}


def test_a_write_of_another_record_is_left_out_when_a_name_is_given(tmp_path):
    write_implement_lane(
        tmp_path,
        [
            (changelog_write("20251231000000-other", "--repo", "docs", "--from", "d.md"), 0),
            (changelog_write(JWT, "--from", "p.md"), 0),
        ],
    )
    events = implement_events(tmp_path)
    assert changelog_writes_by_repo(events, JWT) == {None: 1}
    assert changelog_writes_by_repo(events) == {None: 1, "docs": 1}


def test_a_help_request_is_not_a_write(tmp_path):
    write_implement_lane(
        tmp_path,
        [
            (changelog_write("--help"), 0),
            (changelog_write("--repo", "docs", "--help"), 0),
            (changelog_write(JWT, "--help"), 0),
            (changelog_write(JWT, "--repo", "auth", "-h"), 0),
        ],
    )
    events = implement_events(tmp_path)
    assert changelog_writes_by_repo(events, JWT) == {}
    assert changelog_writes_by_repo(events) == {}


def test_repeated_writes_of_a_record_each_count(tmp_path):
    write_implement_lane(
        tmp_path,
        [
            (changelog_write(JWT, "--repo", "auth", "--from", "a.md"), 0),
            (changelog_write(JWT, "--repo", "auth", "--from", "a2.md"), 0),
        ],
    )
    assert changelog_writes_by_repo(implement_events(tmp_path), JWT) == {"auth": 2}


def test_removing_the_docs_write_leaves_only_the_docs_key_missing(tmp_path):
    copy_s55_implement(tmp_path, drop("docs"))
    assert changelog_writes_by_repo(implement_events(tmp_path), JWT) == {None: 1, "auth": 1}


# --- the implement verifier's own checks -------------------------------------------

CHANGELOG_CHECKS = {
    "project": "test_project_level_changelog_was_committed_via_cli",
    "auth": "test_colocated_repo_changelog_was_committed_via_cli",
    "docs": "test_external_repo_changelog_was_committed_via_cli",
}


def is_successful_write(raw: dict, repo: str | None) -> bool:
    """Whether a raw log record is the 5.5 run's successful write of the
    given repo's record (None for the project's own)."""
    cmd = raw["command"]
    if raw["exit_code"] != 0 or cmd[:3] != ["changelog", "file", "write"] or JWT not in cmd:
        return False
    return (cmd[cmd.index("--repo") + 1] if "--repo" in cmd else None) == repo


def drop(repo):
    return lambda raws: [r for r in raws if not is_successful_write(r, repo)]


def fail(repo):
    return lambda raws: [{**r, "exit_code": 1} if is_successful_write(r, repo) else r for r in raws]


def copy_s55_implement(debug_dir: Path, edit) -> None:
    """The 5.5 implement run's debug log, its records passed through edit."""
    debug_dir.mkdir(parents=True, exist_ok=True)
    for src in (S55_IMPLEMENT / "debug").glob("*.jsonl"):
        raws = [json.loads(line) for line in src.read_text().splitlines() if line.strip()]
        edited = edit(raws)
        (debug_dir / src.name).write_text("".join(json.dumps(r) + "\n" for r in edited))


def changelog_check_results(monkeypatch, debug_dir: Path) -> dict[str, bool]:
    """Which of the verifier's changelog-write checks pass on debug_dir."""
    monkeypatch.setattr(test_implement_workflow, "DEBUG_DIR", debug_dir)
    results = {}
    for record_, check in CHANGELOG_CHECKS.items():
        try:
            getattr(test_implement_workflow, check)()
        except AssertionError:
            results[record_] = False
        else:
            results[record_] = True
    return results


def test_implement_verifier_passes_the_s55_implement_run(monkeypatch):
    assert changelog_check_results(monkeypatch, S55_IMPLEMENT / "debug") == {"project": True, "auth": True, "docs": True}


@pytest.mark.parametrize("edit", [drop, fail], ids=["removed", "failed"])
@pytest.mark.parametrize("repo,record_", [("docs", "docs"), ("auth", "auth"), (None, "project")])
def test_implement_verifier_fails_only_the_record_never_written(monkeypatch, tmp_path, edit, repo, record_):
    copy_s55_implement(tmp_path, edit(repo))
    expected = {name: name != record_ for name in CHANGELOG_CHECKS}
    assert changelog_check_results(monkeypatch, tmp_path) == expected


def test_implement_verifier_fails_every_record_when_no_lane_was_logged(monkeypatch, tmp_path):
    assert changelog_check_results(monkeypatch, tmp_path / "missing") == {"project": False, "auth": False, "docs": False}


def test_implement_verifier_changelog_checks_take_no_transcript():
    import inspect

    for check in CHANGELOG_CHECKS.values():
        assert not inspect.signature(getattr(test_implement_workflow, check)).parameters
    assert not [n for n in vars(test_implement_workflow) if n.endswith("_WRITE_SUBSTR")]
