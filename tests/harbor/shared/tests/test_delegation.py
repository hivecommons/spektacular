"""Self-check for the delegation rules and their oracle in delegation.py.

delegation.py judges a Claude run's child agents against what the workflows
require: the steps whose instructions require a child (REQUIRED), those that
only suggest one (SUGGESTED, never enforced), what an epic child and a spec
review child must be told, and that the main agent sets no timed wake-up
while a child of its own is running. The cases here build small runs by hand
under tmp_path (a Claude stream-json transcript and Spektacular's session
log, in the shapes the neighbouring self-checks use), read them with
`read_run` exactly as a verifier does, and assert the named failures with
hand-written expected values. They also pin the rules on the two epic
reference runs under data/, and tie the hand-kept oracle to the step
templates' delegation wording, so the two cannot drift apart unnoticed.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_delegation.py
"""

import json
import shutil
import sys
from datetime import datetime
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))

import delegation  # noqa: E402
from sessionlog import SessionLog  # noqa: E402
from delegation import (  # noqa: E402
    REQUIRED,
    SUGGESTED,
    Failure,
    epic_children_backgrounded,
    epic_children_foreground_together,
    epic_children_run_as_settled,
    epic_context_gaps,
    is_claude,
    missing_required,
    missing_spec_children,
    read_run,
    review_context_gaps,
    wakeups_while_children_run,
    worktree_paths,
)

DATA = HERE / "data"
TEMPLATES = Path(__file__).resolve().parents[4] / "templates" / "steps"

NAME = "20260101000001-storage"
NAME_B = "20260101000002-commands"
SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"

SPEC_STEPS = [
    "overview",
    "requirements",
    "acceptance_criteria",
    "constraints",
    "technical_approach",
    "success_metrics",
    "non_goals",
    "verification",
    "finished",
]
PLAN_STEPS = ["overview", "discovery", "architecture", "verification", "finished"]
IMPLEMENT_STEPS = [
    "read_plan",
    "analyze",
    "implement",
    "test",
    "verify",
    "check_goal",
    "update_plan",
    "update_changelog",
    "finished",
]
STEPS = {"spec": SPEC_STEPS, "plan": PLAN_STEPS, "implement": IMPLEMENT_STEPS}

STAGED = ".spektacular/tmp/spec_template.md"
REVIEW_PROMPT = f"Read {STAGED} and review only what is written in that file."
WT1 = "/srv/wt/storage/cli"
WT2 = "/srv/wt/storage/docs"


# --- the session log ------------------------------------------------------------


def m(minute: int, second: int = 0) -> str:
    """A clock time on the test day: 10:<minute>:<second>."""
    return f"10:{minute:02d}:{second:02d}"


def cli_ts(clock: str) -> str:
    return f"2026-10-23T{clock}.000000000Z"


def state(kind, name, step):
    return {"kind": kind, "name": name, "current_step": step}


def record(clock, command, before, after, *, exit_code=0, response=None):
    rec = {
        "timestamp": cli_ts(clock),
        "command": command,
        "duration_ms": 0,
        "exit_code": exit_code,
        "state_before": before,
        "state_after": after,
    }
    if response is not None:
        rec["response"] = response
    return rec


def new(kind, name):
    return [kind, "new", "--data", json.dumps({"name": name})]


def goto(kind, name, step):
    return [kind, "goto", "--data", json.dumps({"step": step, "name": name})]


def walk(kind, name, steps, first=0):
    """A lane that enters each step a minute after the last, from 10:<first>:00."""
    records = [record(m(first), new(kind, name), None, state(kind, name, steps[0]))]
    for i, (before, after) in enumerate(zip(steps, steps[1:]), start=first + 1):
        records.append(record(m(i), goto(kind, name, after), state(kind, name, before), state(kind, name, after)))
    return records


def write_jsonl(path: Path, objs) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))


def write_lane(debug: Path, kind, name, records, started="100000") -> None:
    write_jsonl(debug / f"20261023T{started}.903970323Z_claude_{kind}_{name}.jsonl", records)


def write_shared(debug: Path, records) -> None:
    write_jsonl(debug / "20261023T095900.903970323Z_claude_no-active-workflow.jsonl", records)


def worktree_record(clock, spec, paths, *, data_spec=None, exit_code=0, raw=None):
    response = raw if raw is not None else json.dumps({"spec": spec, "repos": [{"name": p.rsplit("/", 1)[-1], "path": p} for p in paths]})
    data = {"spec": data_spec or spec}
    return record(clock, ["epic", "worktree", "--data", json.dumps(data)], None, None, exit_code=exit_code, response=response)


# --- the Claude transcript --------------------------------------------------------


def claude_ts(clock: str) -> str:
    return f"2026-10-23T{clock}Z"


def epoch_ms(clock: str) -> int:
    return int(datetime.fromisoformat(f"2026-10-23T{clock}+00:00").timestamp() * 1000)


def assistant(clock, *content, parent=None, model=SONNET):
    return {
        "type": "assistant",
        "timestamp": claude_ts(clock),
        "parent_tool_use_id": parent,
        "message": {"model": model, "role": "assistant", "content": list(content)},
    }


def tool_use(tid, name, inp):
    return {"type": "tool_use", "id": tid, "name": name, "input": inp}


def child(tid, launched, prompt, *, ended=None, parent=None, background=None):
    """A child the main agent (or the child `parent`) started at `launched`,
    as Claude logs it: the Agent call, task_started, and, when it ended, its
    task_updated and task_notification. System objects carry no timestamp.
    A top-level child runs in the background unless `background` says not."""
    started = {
        "type": "system",
        "subtype": "task_started",
        "tool_use_id": tid,
        "task_id": f"task_{tid}",
        "task_type": "local_agent",
        "is_backgrounded": parent is None if background is None else background,
        "spawn_depth": 1 if parent is None else 2,
    }
    if parent is not None:
        started["parent_task_id"] = f"task_{parent}"
    objs = [
        assistant(
            launched,
            tool_use(tid, "Agent", {"subagent_type": "general-purpose", "prompt": prompt, "description": tid}),
            parent=parent,
            model=SONNET if parent is None else HAIKU,
        ),
        started,
    ]
    if ended is not None:
        objs += [
            {"type": "system", "subtype": "task_updated", "task_id": f"task_{tid}", "patch": {"status": "completed", "end_time": epoch_ms(ended)}},
            {
                "type": "system",
                "subtype": "task_notification",
                "tool_use_id": tid,
                "task_id": f"task_{tid}",
                "status": "completed",
                "summary": f"{tid} done.",
                "usage": {"duration_ms": 1000},
            },
        ]
    return objs


def transcript(*parts):
    objs = [{"type": "system", "subtype": "init", "model": SONNET}]
    for p in parts:
        objs += p if isinstance(p, list) else [p]
    return objs + [{"type": "result", "subtype": "success", "num_turns": 1}]


def run_of(tmp_path, lanes, objs, shared=()):
    """Writes the session log (lanes: (kind, name, records)) and the
    transcript under tmp_path, and reads them back as a verifier does."""
    debug = tmp_path / "debug"
    debug.mkdir()
    for n, (kind, name, records) in enumerate(lanes):
        write_lane(debug, kind, name, records, started=f"1000{n:02d}")
    if shared:
        write_shared(debug, list(shared))
    path = tmp_path / "claude-code.txt"
    write_jsonl(path, objs)
    return read_run(path, debug)


def step_children(kind, steps, skip=(), parent=None):
    """A child launched half a minute into each REQUIRED step of a lane
    walked from 10:00, except those in `skip`."""
    out = []
    for i, step in enumerate(steps):
        if (kind, step) in REQUIRED and step not in skip:
            prompt = REVIEW_PROMPT if kind == "spec" else f"Do the {step} step."
            out.append(child(f"toolu_{step}", m(i, 30), prompt, ended=m(i, 50), parent=parent))
    return out


def missing(kind, step, attempt=None):
    suffix = f" (attempt {attempt})" if attempt else ""
    return Failure("missing_child", f"{kind}:{NAME}:{step}{suffix}", f"the {step} step completed without a child agent started in it")


def lane_steps(run):
    return {k: [w.step for w in ws] for k, ws in run.windows.items()}


# --- required steps (criterion 1) ------------------------------------------------


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_required_step_without_a_child_fails_naming_that_step(tmp_path, entry):
    kind, step = entry
    run = run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], transcript(*step_children(kind, STEPS[kind], skip={step})))
    assert missing_required(run.placed, run.windows) == [missing(kind, step)]


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_required_step_with_a_child_launched_in_it_passes(tmp_path, entry):
    kind, step = entry
    run = run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], transcript(*step_children(kind, STEPS[kind])))
    assert [p.step for p in run.placed if p.child.id == f"toolu_{step}"] == [step]
    assert missing_required(run.placed, run.windows) == []


def test_a_child_launched_in_another_step_does_not_satisfy_a_required_one(tmp_path):
    # the goal check's reviewer is launched in the verify step before it
    objs = transcript(child("toolu_early", m(4, 30), "Check the goal."), *step_children("implement", IMPLEMENT_STEPS, skip={"check_goal"}))
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], objs)
    assert missing_required(run.placed, run.windows) == [missing("implement", "check_goal")]


def test_a_nested_child_satisfies_a_required_step(tmp_path):
    # An orchestrator's child works the lane; the step children are its own.
    objs = transcript(
        child("toolu_W", "09:59:30", f"Implement {NAME}."),
        *step_children("implement", IMPLEMENT_STEPS, parent="toolu_W"),
    )
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], objs)
    assert {p.child.id: p.child.parent for p in run.placed if p.child.parent} == {
        "toolu_check_goal": "toolu_W",
    }
    assert missing_required(run.placed, run.windows) == []


def test_the_step_a_lane_ended_on_is_not_judged(tmp_path):
    steps = ["read_plan", "analyze", "implement", "test", "verify", "check_goal"]  # cut off in check_goal
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, steps))], transcript())
    assert missing_required(run.placed, run.windows) == []


def test_the_same_step_once_moved_on_from_is_judged(tmp_path):
    steps = ["read_plan", "analyze", "implement", "test", "verify", "check_goal", "update_plan"]
    objs = transcript(child("toolu_late", m(6, 30), "Do it.", ended=m(6, 50)))
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, steps))], objs)
    assert missing_required(run.placed, run.windows) == [missing("implement", "check_goal")]


RETRIED = ["read_plan", "implement", "test", "verify", "check_goal", "implement", "test", "verify", "check_goal", "update_plan", "finished"]


@pytest.mark.parametrize(
    "childless, expected",
    [
        (8, missing("implement", "check_goal", attempt=2)),  # the second goal check
        (4, missing("implement", "check_goal")),  # the first: no attempt number
    ],
)
def test_a_retried_step_is_judged_per_attempt(tmp_path, childless, expected):
    objs = transcript(
        *[child(f"toolu_{i}", m(i, 30), "Do it.", ended=m(i, 50)) for i, s in enumerate(RETRIED) if ("implement", s) in REQUIRED and i != childless]
    )
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, RETRIED))], objs)
    assert lane_steps(run)[("implement", NAME)] == RETRIED
    assert missing_required(run.placed, run.windows) == [expected]


def resumed_lane():
    """An implement lane whose check_goal step a second child resumed at
    10:04: two windows at check_goal, 10:02-10:03 and 10:04-10:05."""
    kind = "implement"
    return [
        record(m(0), new(kind, NAME), None, state(kind, NAME, "read_plan")),
        record(m(1), goto(kind, NAME, "verify"), state(kind, NAME, "read_plan"), state(kind, NAME, "verify")),
        record(m(2), goto(kind, NAME, "check_goal"), state(kind, NAME, "verify"), state(kind, NAME, "check_goal")),
        record(m(3), [kind, "file", "read", NAME], state(kind, NAME, "check_goal"), state(kind, NAME, "check_goal")),
        record(m(4), new(kind, NAME), state(kind, NAME, "check_goal"), None, exit_code=1, response='{"error":"workflow_in_progress"}'),
        record(m(5), goto(kind, NAME, "update_plan"), state(kind, NAME, "check_goal"), state(kind, NAME, "update_plan")),
        record(m(6), goto(kind, NAME, "update_changelog"), state(kind, NAME, "update_plan"), state(kind, NAME, "update_changelog")),
        record(m(7), goto(kind, NAME, "finished"), state(kind, NAME, "update_changelog"), state(kind, NAME, "finished")),
    ]


@pytest.mark.parametrize("launched", [m(4, 30), m(2, 30)], ids=["resumed-window", "first-window"])
def test_a_child_in_either_window_of_a_resumed_step_satisfies_it(tmp_path, launched):
    objs = transcript(child("toolu_goal", launched, "Check the goal.", ended=m(4, 50)))
    run = run_of(tmp_path, [("implement", NAME, resumed_lane())], objs)
    assert lane_steps(run)[("implement", NAME)] == ["read_plan", "verify", "check_goal", "check_goal", "update_plan", "update_changelog", "finished"]
    assert missing_required(run.placed, run.windows) == []


def test_a_resumed_step_without_a_child_fails_once(tmp_path):
    run = run_of(tmp_path, [("implement", NAME, resumed_lane())], transcript())
    assert missing_required(run.placed, run.windows) == [missing("implement", "check_goal")]


# --- suggested steps (criterion 2) -----------------------------------------------


def test_suggested_and_required_steps_do_not_overlap():
    assert SUGGESTED == {("plan", "discovery"), ("implement", "analyze"), ("implement", "test"), ("implement", "update_changelog")}
    assert SUGGESTED.isdisjoint(REQUIRED)


def orchestrated_prompt(name, *extra):
    return " ".join([f"Work spec {name} in the project at /app.", 'Start it with {"orchestrated": true}.', *extra])


def test_a_run_whose_suggested_steps_used_no_child_passes_every_rule(tmp_path):
    # An epic run: a plan lane and an implement lane, each worked by its own
    # child. No child in discovery, analyze or update_changelog.
    objs = transcript(
        child("toolu_P", "09:59:00", orchestrated_prompt(NAME_B), ended=m(5)),
        child("toolu_I", "09:59:10", orchestrated_prompt(NAME, f"Your code is at {WT1} and {WT2}."), ended=m(9)),
        *step_children("implement", IMPLEMENT_STEPS, parent="toolu_I"),
    )
    shared = [worktree_record("09:58:00", NAME, [WT1, WT2])]
    run = run_of(
        tmp_path,
        [("plan", NAME_B, walk("plan", NAME_B, PLAN_STEPS)), ("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))],
        objs,
        shared,
    )
    steps = lane_steps(run)
    # every suggested step was completed, so it was judged
    assert steps[("plan", NAME_B)][:-1].count("discovery") == 1
    assert {"analyze", "update_changelog"} <= set(steps[("implement", NAME)][:-1])
    assert [(p.child.id, p.step) for p in run.placed if p.step in {"discovery", "analyze", "update_changelog"}] == []
    assert missing_required(run.placed, run.windows) == []
    assert missing_spec_children(run.placed, list(run.session.lanes)) == []
    assert epic_context_gaps(run.placed, run.session.lanes, worktrees=worktree_paths(run.session)) == []
    assert review_context_gaps(run.placed) == []
    assert wakeups_while_children_run(run.wakeups, run.placed, run.end) == []


# --- epic children's context (criterion 3) ---------------------------------------

PARTS = {
    "spec": f"Plan spec {NAME}.",
    "root": "The project is at /app.",
    "orchestrated": 'Start it with --data \'{"orchestrated": true}\'.',
    "worktrees": f"Your code is at {WT1} and {WT2}.",
    "resume": "Resume the spec's earlier run.",
}


def prompt(omit=(), **replace):
    parts = {**PARTS, **replace}
    return " ".join(v for k, v in parts.items() if k not in omit)


def epic_lane(tmp_path, kind, prompts, worktrees=(WT1, WT2)):
    """One epic lane worked by a child per prompt, the orchestrator having
    asked for the spec's worktrees first."""
    objs = transcript(*[child(f"toolu_C{n}", f"09:5{n}:00", p, ended=f"09:5{n}:30") for n, p in enumerate(prompts, start=1)])
    shared = [worktree_record("09:50:00", NAME, list(worktrees))] if worktrees else []
    return run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], objs, shared)


def gaps(run):
    return epic_context_gaps(run.placed, run.session.lanes, worktrees=worktree_paths(run.session))


def gap(child_id, what):
    return Failure("context_gap", f"child {child_id} ({NAME})", f"its prompt does not give {what}")


ITEM_DETAIL = {
    "spec": f"the spec it works on ({NAME})",
    "root": "the project root (/app)",
    "orchestrated": 'that its run is orchestrated ("orchestrated":true)',
}


@pytest.mark.parametrize("kind", ["plan", "implement"])
def test_an_epic_child_given_everything_passes(tmp_path, kind):
    run = epic_lane(tmp_path, kind, [prompt()])
    assert gaps(run) == []


@pytest.mark.parametrize("kind", ["plan", "implement"])
@pytest.mark.parametrize("item", ["spec", "root", "orchestrated"])
def test_an_epic_child_missing_an_item_fails_naming_the_child_and_the_item(tmp_path, kind, item):
    run = epic_lane(tmp_path, kind, [prompt(omit={item})])
    assert gaps(run) == [gap("toolu_C1", ITEM_DETAIL[item])]


@pytest.mark.parametrize(
    "words",
    [
        'Start it with --data \'{"orchestrated": true}\'.',
        'Start it with --data "{\\"name\\":\\"x\\",\\"orchestrated\\": true}".',  # escaped for the shell
        'Pass "orchestrated":\n  true.',
        "This run is orchestrated: report back when done.",
        "THIS RUN IS ORCHESTRATED.",
    ],
)
def test_either_form_of_orchestrated_is_accepted(tmp_path, words):
    run = epic_lane(tmp_path, "plan", [prompt(orchestrated=words)])
    assert gaps(run) == []


@pytest.mark.parametrize("words", ['Start it with {"orchestrated": false}.', "It is orchestrated."])
def test_other_mentions_of_orchestrated_are_not(tmp_path, words):
    run = epic_lane(tmp_path, "plan", [prompt(orchestrated=words)])
    assert gaps(run) == [gap("toolu_C1", ITEM_DETAIL["orchestrated"])]


def test_an_implementing_child_missing_a_worktree_path_fails_naming_it(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(worktrees=f"Your code is at {WT2}.")])
    assert gaps(run) == [gap("toolu_C1", f"its code location {WT1}")]


def test_an_implementing_child_missing_every_worktree_path_fails_once_per_path(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(omit={"worktrees"})])
    assert gaps(run) == [gap("toolu_C1", f"its code location {WT1}"), gap("toolu_C1", f"its code location {WT2}")]


def test_a_planning_child_is_not_given_worktree_paths(tmp_path):
    run = epic_lane(tmp_path, "plan", [prompt(omit={"worktrees"})])
    assert gaps(run) == []


def test_worktree_paths_are_not_checked_without_a_worktree_response(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(omit={"worktrees"})], worktrees=())
    assert worktree_paths(run.session) == {}
    assert gaps(run) == []


@pytest.mark.parametrize("kind", ["plan", "implement"])
def test_a_second_child_not_told_to_resume_fails_naming_it(tmp_path, kind):
    run = epic_lane(tmp_path, kind, [prompt(omit={"resume"}), prompt(omit={"resume"})])
    # the first child starts the spec, so only the second needs the word
    assert gaps(run) == [gap("toolu_C2", "the instruction to resume the spec's earlier run")]


def test_a_second_child_told_to_resume_passes(tmp_path):
    run = epic_lane(tmp_path, "plan", [prompt(omit={"resume"}), prompt(resume="RESUME where it stopped.")])
    assert gaps(run) == []


def test_every_missing_item_is_named(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(), "Carry on."])
    assert gaps(run) == [
        gap("toolu_C2", ITEM_DETAIL["spec"]),
        gap("toolu_C2", ITEM_DETAIL["root"]),
        gap("toolu_C2", ITEM_DETAIL["orchestrated"]),
        gap("toolu_C2", f"its code location {WT1}"),
        gap("toolu_C2", f"its code location {WT2}"),
        gap("toolu_C2", "the instruction to resume the spec's earlier run"),
    ]


def test_worktree_paths_read_only_successful_epic_worktree_responses(tmp_path):
    debug = tmp_path / "debug"
    write_shared(
        debug,
        [
            worktree_record("09:50:00", NAME, [WT1, WT2]),
            worktree_record("09:50:10", NAME_B, ["/srv/wt/failed"], exit_code=1),
            # the response names no spec: the command's data does
            worktree_record("09:50:20", "", ["/srv/wt/commands"], data_spec="x-spec", raw=json.dumps({"repos": [{"path": "/srv/wt/x"}]})),
            worktree_record("09:50:30", "y-spec", [], raw="not json"),
        ],
    )
    assert worktree_paths(SessionLog(debug)) == {NAME: [WT1, WT2], "x-spec": ["/srv/wt/x"]}


# --- spec review children (criterion 3) ------------------------------------------


def review_run(tmp_path, review_prompt, step="verification"):
    i = SPEC_STEPS.index(step)
    objs = transcript(child("toolu_R", m(i, 30), review_prompt, ended=m(i, 50)))
    return run_of(tmp_path, [("spec", NAME, walk("spec", NAME, SPEC_STEPS))], objs)


@pytest.mark.parametrize("words", [REVIEW_PROMPT, f"Read {STAGED}. Judge it Exactly As Written, nothing more."])
def test_a_review_child_given_the_staged_spec_and_told_to_review_only_it_passes(tmp_path, words):
    run = review_run(tmp_path, words)
    assert [p.step for p in run.placed] == ["verification"]
    assert review_context_gaps(run.placed) == []


def test_a_review_child_not_given_the_staged_spec_fails_naming_it(tmp_path):
    run = review_run(tmp_path, "Read the spec and review only what is written in that file.")
    assert review_context_gaps(run.placed) == [
        Failure("context_gap", f"child toolu_R ({NAME})", f"its prompt does not give the staged spec to read ({STAGED})")
    ]


def test_a_review_child_not_told_to_review_only_what_is_written_fails_naming_it(tmp_path):
    run = review_run(tmp_path, f"Read {STAGED} and review it.")
    assert review_context_gaps(run.placed) == [
        Failure("context_gap", f"child toolu_R ({NAME})", "its prompt does not give the instruction to review only what is written")
    ]


def test_a_child_outside_the_spec_verification_step_is_not_a_reviewer(tmp_path):
    run = review_run(tmp_path, "Look around.", step="requirements")
    assert review_context_gaps(run.placed) == []


# --- one child per epic spec ------------------------------------------------------


def two_plan_lanes(tmp_path, *children_):
    lanes = [("plan", NAME, walk("plan", NAME, PLAN_STEPS)), ("plan", NAME_B, walk("plan", NAME_B, PLAN_STEPS))]
    return run_of(tmp_path, lanes, transcript(*children_))


def test_each_epic_spec_worked_by_its_own_child_passes(tmp_path):
    run = two_plan_lanes(tmp_path, child("toolu_A", "09:59:00", orchestrated_prompt(NAME)), child("toolu_B", "09:59:10", orchestrated_prompt(NAME_B)))
    assert missing_spec_children(run.placed, list(run.session.lanes)) == []


def missing_b():
    return [Failure("missing_spec_child", f"plan:{NAME_B}", f"no child of the orchestrator was started for spec {NAME_B}")]


def test_an_epic_spec_no_child_worked_fails_naming_it(tmp_path):
    run = two_plan_lanes(tmp_path, child("toolu_A", "09:59:00", orchestrated_prompt(NAME)))
    assert missing_spec_children(run.placed, list(run.session.lanes)) == missing_b()


def test_a_nested_child_naming_a_spec_does_not_work_it_for_the_orchestrator(tmp_path):
    run = two_plan_lanes(
        tmp_path,
        child("toolu_A", "09:59:00", orchestrated_prompt(NAME)),
        child("toolu_N", m(1, 30), f"Look at {NAME_B} too.", parent="toolu_A"),
    )
    assert [(p.child.id, p.spec) for p in run.placed] == [("toolu_A", NAME), ("toolu_N", NAME_B)]
    assert missing_spec_children(run.placed, list(run.session.lanes)) == missing_b()


def test_a_spec_lane_needs_no_child_of_its_own():
    assert missing_spec_children([], [("spec", NAME)]) == []
    assert missing_spec_children([], [("implement", NAME)]) == [
        Failure("missing_spec_child", f"implement:{NAME}", f"no child of the orchestrator was started for spec {NAME}")
    ]


# --- wake-ups while children run (criterion 4) ------------------------------------


def wakeup_run(tmp_path, ended=m(5)):
    """A top-level child running 10:00:10 to `ended`, and the main agent's
    wake-ups around it, one of them set by the child itself."""
    objs = transcript(
        assistant("10:00:05", tool_use("w0", "ScheduleWakeup", {"delaySeconds": 30})),  # before launch
        child("toolu_C", "10:00:10", orchestrated_prompt(NAME), ended=ended),
        assistant("10:00:10", tool_use("w1", "ScheduleWakeup", {"delaySeconds": 60})),  # at launch
        assistant("10:02:00", tool_use("b1", "Bash", {"command": "sleep 60"})),
        assistant("10:03:00", tool_use("w2", "ScheduleWakeup", {"delaySeconds": 90}), parent="toolu_C", model=HAIKU),  # the child's
        assistant("10:05:00", tool_use("c1", "CronCreate", {"cron": "*/5 * * * *"})),  # at its end
        assistant("10:06:00", tool_use("b2", "Bash", {"command": "ls"})),
    )
    return run_of(tmp_path, [("plan", NAME, walk("plan", NAME, PLAN_STEPS))], objs)


def test_wakeups_set_while_a_child_runs_fail_naming_each(tmp_path):
    run = wakeup_run(tmp_path)
    assert wakeups_while_children_run(run.wakeups, run.placed, run.end) == [
        Failure("wakeup_while_children_run", "ScheduleWakeup delaySeconds=60", "set at 2026-10-23T10:00:10+00:00 while a child was running"),
        Failure("wakeup_while_children_run", "Bash sleep 60", "set at 2026-10-23T10:02:00+00:00 while a child was running"),
    ]


def test_a_child_without_an_end_runs_until_the_run_ends(tmp_path):
    run = wakeup_run(tmp_path, ended=None)
    assert run.end == datetime.fromisoformat("2026-10-23T10:06:00+00:00")
    subjects = ["ScheduleWakeup delaySeconds=60", "Bash sleep 60", "CronCreate */5 * * * *"]
    assert [f.subject for f in wakeups_while_children_run(run.wakeups, run.placed, run.end)] == subjects
    assert [f.subject for f in wakeups_while_children_run(run.wakeups, run.placed)] == subjects


def test_a_wakeup_with_no_child_running_passes(tmp_path):
    objs = transcript(assistant("10:00:05", tool_use("w0", "ScheduleWakeup", {"delaySeconds": 30})))
    run = run_of(tmp_path, [("plan", NAME, walk("plan", NAME, PLAN_STEPS))], objs)
    assert wakeups_while_children_run(run.wakeups, run.placed, run.end) == []


# --- how epic children run, as the background-child probe settled ------------------

PLAN_LANES = [("plan", NAME), ("plan", NAME_B)]


def epic_children_run(tmp_path, *, background):
    """Two plan lanes, each worked by a top-level child launched at 10:00:00
    and 10:00:05 that ends at 10:05:00. Run in the foreground, the
    orchestrator holds both in its one turn and ends it at 10:05:10. Run in
    the background, it ends its first turn at 10:00:10 while they run, and a
    completion notice wakes a second turn that ends at 10:05:10."""
    children_ = [
        child("toolu_A", "10:00:00", orchestrated_prompt(NAME), ended=m(5), background=background),
        child("toolu_B", "10:00:05", orchestrated_prompt(NAME_B), ended=m(5), background=background),
    ]
    if background:
        objs = transcript(*children_, assistant("10:00:10", {"type": "text", "text": "Both specs are running."}))
        objs += [
            {"type": "system", "subtype": "init", "model": SONNET},
            assistant("10:05:10", {"type": "text", "text": "Both specs are done."}),
            {"type": "result", "subtype": "success", "num_turns": 1, "origin": {"kind": "task-notification"}},
        ]
    else:
        objs = transcript(*children_, assistant("10:05:10", {"type": "text", "text": "Both specs are done."}))
    lanes = [(kind, name, walk(kind, name, PLAN_STEPS)) for kind, name in PLAN_LANES]
    tmp_path.mkdir(exist_ok=True)
    return run_of(tmp_path, lanes, objs)


def test_the_epic_children_runs_read_back_as_built(tmp_path):
    fg = epic_children_run(tmp_path / "fg", background=False)
    bg = epic_children_run(tmp_path / "bg", background=True)
    assert [(p.child.id, p.spec, p.child.background) for p in fg.placed] == [("toolu_A", NAME, False), ("toolu_B", NAME_B, False)]
    assert [(p.child.id, p.spec, p.child.background) for p in bg.placed] == [("toolu_A", NAME, True), ("toolu_B", NAME_B, True)]
    at = lambda clock: datetime.fromisoformat(f"2026-10-23T{clock}+00:00")  # noqa: E731
    assert [(t.ended_at, t.woken_by) for t in fg.turns] == [(at("10:05:10"), "start")]
    assert [(t.ended_at, t.woken_by) for t in bg.turns] == [(at("10:00:10"), "start"), (at("10:05:10"), "task-notification")]


def test_foreground_children_held_inside_the_turn_pass_the_foreground_rule(tmp_path):
    run = epic_children_run(tmp_path, background=False)
    assert epic_children_foreground_together(run.placed, run.turns, PLAN_LANES) == []


def test_background_children_fail_the_foreground_rule_naming_each_child(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    turn_end = "a turn ended at 2026-10-23T10:00:10+00:00 while it was running"
    assert epic_children_foreground_together(run.placed, run.turns, PLAN_LANES) == [
        Failure("child_in_background", f"child toolu_A ({NAME})", "it was started in the background"),
        Failure("turn_ended_while_child_ran", f"child toolu_A ({NAME})", turn_end),
        Failure("child_in_background", f"child toolu_B ({NAME_B})", "it was started in the background"),
        Failure("turn_ended_while_child_ran", f"child toolu_B ({NAME_B})", turn_end),
    ]


def test_background_children_woken_by_their_notice_pass_the_background_rule(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    assert epic_children_backgrounded(run.placed, run.turns, PLAN_LANES) == []


def test_foreground_children_fail_the_background_rule(tmp_path):
    run = epic_children_run(tmp_path, background=False)
    assert epic_children_backgrounded(run.placed, run.turns, PLAN_LANES) == [
        Failure("child_in_foreground", f"child toolu_A ({NAME})", "it was started in the foreground"),
        Failure("child_in_foreground", f"child toolu_B ({NAME_B})", "it was started in the foreground"),
        Failure("no_turn_ended_while_children_ran", "orchestrator", "no turn ended while a child was running"),
        Failure("no_notice_woken_turn", "orchestrator", "no turn was started by a child's completion notice"),
    ]


def test_spec_lanes_are_not_judged_by_either_epic_children_rule(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    assert epic_children_foreground_together(run.placed, run.turns, [("spec", NAME)]) == []
    assert epic_children_backgrounded(run.placed, run.turns, [("spec", NAME)]) == []


def test_the_settled_rule_follows_the_probe_outcome(tmp_path, monkeypatch):
    fg = epic_children_run(tmp_path / "fg", background=False)
    bg = epic_children_run(tmp_path / "bg", background=True)
    monkeypatch.setattr(delegation, "PROBE_OUTCOME", "stopped")
    assert epic_children_run_as_settled(fg.placed, fg.turns, PLAN_LANES) == []
    assert {f.rule for f in epic_children_run_as_settled(bg.placed, bg.turns, PLAN_LANES)} == {"child_in_background", "turn_ended_while_child_ran"}
    monkeypatch.setattr(delegation, "PROBE_OUTCOME", "completed")
    assert epic_children_run_as_settled(bg.placed, bg.turns, PLAN_LANES) == []
    assert {f.rule for f in epic_children_run_as_settled(fg.placed, fg.turns, PLAN_LANES)} == {
        "child_in_foreground",
        "no_turn_ended_while_children_ran",
        "no_notice_woken_turn",
    }


def test_the_probe_outcome_matches_the_recorded_probe():
    line = (DATA / "probe-background-child" / "OUTCOME").read_text()
    assert line.split()[0].rstrip(":") == delegation.PROBE_OUTCOME == "stopped"


# --- the reference runs (criterion 4) ---------------------------------------------


def reference(name):
    return read_run(DATA / name / "claude-code.txt", DATA / name / "debug")


def test_the_55_epic_run_fails_the_wakeup_rule_naming_the_scheduled_wakeup():
    run = reference("s55-epic")
    failures = wakeups_while_children_run(run.wakeups, run.placed, run.end)
    assert [(f.rule, f.subject) for f in failures] == [("wakeup_while_children_run", "ScheduleWakeup delaySeconds=1200")]


def test_the_46_epic_run_passes_the_wakeup_rule():
    run = reference("m70-epic")
    assert wakeups_while_children_run(run.wakeups, run.placed, run.end) == []


def test_the_46_epic_run_passes_the_settled_children_rule():
    run = reference("m70-epic")
    assert epic_children_run_as_settled(run.placed, run.turns, list(run.windows)) == []


def test_the_55_epic_run_fails_the_settled_children_rule_naming_its_background_children():
    run = reference("s55-epic")
    failures = epic_children_run_as_settled(run.placed, run.turns, list(run.windows))
    storage = "child toolu_01LsCrL1DDkTZGc6kMPVwn5D (20260101000001-storage)"
    help_ = "child toolu_01Fcx3c9hqXdQYxakexacWST (20260101000003-help)"
    assert [(f.rule, f.subject) for f in failures] == [
        ("child_in_background", storage),
        ("turn_ended_while_child_ran", storage),
        ("child_in_background", help_),
        ("turn_ended_while_child_ran", help_),
    ]
    # the commands child ran in the foreground and is not named
    assert not any("20260101000002-commands" in f.subject for f in failures)


@pytest.mark.parametrize("name", ["s55-epic", "m70-epic"])
def test_both_epic_reference_runs_pass_the_other_rules(name):
    run = reference(name)
    # plan epics: three plan lanes, one top-level child each, no worktrees
    assert sorted(run.session.lanes) == [("plan", "20260101000001-storage"), ("plan", "20260101000002-commands"), ("plan", "20260101000003-help")]
    assert len([p for p in run.placed if p.child.parent is None]) == 3
    assert worktree_paths(run.session) == {}
    assert missing_required(run.placed, run.windows) == []
    assert missing_spec_children(run.placed, list(run.session.lanes)) == []
    assert epic_context_gaps(run.placed, run.session.lanes, worktrees=worktree_paths(run.session)) == []
    assert review_context_gaps(run.placed) == []


# --- the oracle tied to the step templates (criterion 5) -------------------------

# By hand: the step template that requires each REQUIRED entry's child, and the
# words in it that require one.
GATE = "does not complete this step"
TEMPLATE_OF = {
    ("spec", "verification"): ("spec/08-verification.md", GATE),
    ("implement", "check_goal"): ("implement/05b-check_goal.md", "never as yourself"),
}


def oracle_template_mismatches(oracle, templates: Path) -> list[str]:
    """Where the oracle and the step templates' delegation wording disagree:
    an oracle entry whose template lacks its wording, a mapped template with
    the wording whose entry is not in the oracle, and any other step template
    using the gate wording."""
    out = []

    def says(rel, marker):
        path = templates / rel
        return path.is_file() and marker in path.read_text()

    for entry in sorted(oracle - TEMPLATE_OF.keys()):
        out.append(f"{entry} has no step template mapped")
    for entry, (rel, marker) in TEMPLATE_OF.items():
        if entry in oracle and not says(rel, marker):
            out.append(f"{entry} is required but {rel} does not say {marker!r}")
        if entry not in oracle and says(rel, marker):
            out.append(f"{rel} says {marker!r} but {entry} is not in the oracle")
    mapped = {rel for rel, _ in TEMPLATE_OF.values()}
    for path in sorted(templates.rglob("*.md")):
        rel = path.relative_to(templates).as_posix()
        if GATE in path.read_text() and rel not in mapped:
            out.append(f"{rel} says {GATE!r} but no oracle entry maps to it")
    return out


def test_the_oracle_and_the_step_templates_agree():
    assert TEMPLATES.is_dir()
    assert set(TEMPLATE_OF) == REQUIRED
    assert oracle_template_mismatches(REQUIRED, TEMPLATES) == []


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_removing_a_step_from_the_oracle_fails_the_tie(entry):
    rel, marker = TEMPLATE_OF[entry]
    assert oracle_template_mismatches(REQUIRED - {entry}, TEMPLATES) == [f"{rel} says {marker!r} but {entry} is not in the oracle"]


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_removing_a_steps_wording_from_its_template_fails_the_tie(tmp_path, entry):
    rel, marker = TEMPLATE_OF[entry]
    copy = tmp_path / "steps"
    shutil.copytree(TEMPLATES, copy)
    path = copy / rel
    path.write_text(path.read_text().replace(marker, "completes this step"))
    assert oracle_template_mismatches(REQUIRED, copy) == [f"{entry} is required but {rel} does not say {marker!r}"]


def test_adding_the_wording_to_another_step_fails_the_tie(tmp_path):
    copy = tmp_path / "steps"
    shutil.copytree(TEMPLATES, copy)
    path = copy / "plan" / "02-discovery.md"
    path.write_text(path.read_text() + f"\nExploring in your own context {GATE}.\n")
    assert oracle_template_mismatches(REQUIRED, copy) == [f"plan/02-discovery.md says {GATE!r} but no oracle entry maps to it"]


def test_an_oracle_entry_without_a_template_fails_the_tie():
    assert oracle_template_mismatches(REQUIRED | {("plan", "discovery")}, TEMPLATES) == ["('plan', 'discovery') has no step template mapped"]


# --- which runs the rules apply to ------------------------------------------------


def write_build(tmp_path, raw):
    path = tmp_path / "build.json"
    path.write_text(raw)
    return path


def test_a_claude_run_with_a_transcript_is_judged(tmp_path):
    t = tmp_path / "claude-code.txt"
    t.write_text('{"type":"system"}\n')
    assert is_claude(write_build(tmp_path, json.dumps({"agent": "claude"})), t) is True
    assert is_claude(DATA / "s55-epic" / "build.json", DATA / "s55-epic" / "claude-code.txt") is True


@pytest.mark.parametrize(
    "build, transcript_text",
    [
        ('{"agent": "bob"}', '{"type":"system"}\n'),  # another agent
        ('{"agent": "claude"}', ""),  # an empty transcript
        ('{"agent": "claude"}', None),  # no transcript
        ("not json", '{"type":"system"}\n'),  # broken build info
        ('["claude"]', '{"type":"system"}\n'),  # build info not an object
        (None, '{"type":"system"}\n'),  # no build info
    ],
)
def test_other_runs_are_not_judged(tmp_path, build, transcript_text):
    b = write_build(tmp_path, build) if build is not None else tmp_path / "missing.json"
    t = tmp_path / "claude-code.txt"
    if transcript_text is not None:
        t.write_text(transcript_text)
    assert is_claude(b, t) is False


def test_failure_names_its_rule_and_subject():
    assert str(Failure("missing_child", "implement:x:test", "why")) == "missing_child implement:x:test: why"
    assert delegation.PER_SPEC_CHILD == {"plan", "implement"}


# --- the trace oracle and the trace partials (spec 000074) --------------------------

PARTIALS = Path(__file__).resolve().parents[4] / "templates" / "partials"


def trace_oracle_mismatches(partials: Path) -> list[str]:
    """Where the trace oracle and the shared trace partials disagree: each
    oracle wording must appear in the partial that carries it, and the
    marking partial must include the helper rule."""
    helpers = (partials / "trace-helpers.md").read_text()
    marking = (partials / "trace-marking.md").read_text()
    out = []
    for name, value, rel, text in [
        ("TRACE_START", delegation.TRACE_START, "trace-helpers.md", helpers),
        ("SUB_AGENT_TYPE", delegation.SUB_AGENT_TYPE, "trace-helpers.md", helpers),
        ("TRACE_START", delegation.TRACE_START, "trace-marking.md", marking),
        ("USER_WAIT_TYPE", delegation.USER_WAIT_TYPE, "trace-marking.md", marking),
    ]:
        if value not in text:
            out.append(f"{rel} does not say {name} {value!r}")
    if "{{> partials/trace-helpers}}" not in marking:
        out.append("trace-marking.md does not include the helper rule")
    return out


def test_the_trace_oracle_and_the_trace_partials_agree():
    assert PARTIALS.is_dir()
    assert trace_oracle_mismatches(PARTIALS) == []


@pytest.mark.parametrize(
    "rel, old, new, expected",
    [
        ("trace-helpers.md", "--type sub-agent", "--kind sub-agent", "trace-helpers.md does not say SUB_AGENT_TYPE '--type sub-agent'"),
        ("trace-marking.md", "--type user-wait", "--type wait", "trace-marking.md does not say USER_WAIT_TYPE '--type user-wait'"),
        ("trace-helpers.md", "trace start", "span begin", "trace-helpers.md does not say TRACE_START 'trace start'"),
    ],
)
def test_changing_the_trace_wording_fails_the_tie(tmp_path, rel, old, new, expected):
    copy = tmp_path / "partials"
    shutil.copytree(PARTIALS, copy)
    path = copy / rel
    assert old in path.read_text()
    path.write_text(path.read_text().replace(old, new))
    assert expected in trace_oracle_mismatches(copy)


# --- helper launches marked by sub-agent spans -------------------------------------


def placed_child(cid, clock, step="check_goal", spec=NAME):
    from transcripts import Child

    c = Child(
        id=cid, parent=None, kind="general-purpose", description=f"child {cid}", background=False,
        launched_at=datetime.fromisoformat(f"2026-10-23T{clock}+00:00"), ended_at=None, duration_s=None,
        ending=None, hand_back=None, models=None, output=None, prompt=None,
    )
    return delegation.PlacedChild(c, ("implement", spec), 0, spec, step)


def sub_agent(span_id, clock, span_type="sub-agent"):
    import spans as sp

    return sp.Span(
        trace_id="t", span_id=span_id, parent=None, name=span_id, type=span_type,
        start=datetime.fromisoformat(f"2026-10-23T{clock}+00:00"), end=None, status=None,
        end_reason=None, step=None, lane="x",
    )


def test_each_launch_needs_its_own_sub_agent_span_just_before_it():
    placed = [placed_child("a", "10:05:00"), placed_child("b", "10:05:00"), placed_child("c", "10:20:00")]
    spans = [
        sub_agent("s1", "10:04:30"),  # 30 s before both a and b: marks one of them
        sub_agent("s2", "10:05:03"),  # within the slack after a launch
        sub_agent("w1", "10:19:50", "user-wait"),  # not a sub-agent span
        sub_agent("s3", "10:10:00"),  # ten minutes before c: too early
    ]
    marked, unmarked = delegation.match_launches(placed, spans)
    assert [p.child.id for p in marked] == ["a", "b"]
    assert [p.child.id for p in unmarked] == ["c"]
    assert [str(f) for f in delegation.unmarked_launches(placed, spans)] == [
        f"unmarked_launch {NAME}: child child c launched at 10:20:00 in step check_goal has no sub-agent span (`trace start <helper> --type sub-agent`)"
    ]
    assert delegation.marked_rate(placed, spans) == 2 / 3


def test_no_timed_launches_has_no_rate():
    assert delegation.marked_rate([], [sub_agent("s1", "10:00:00")]) is None
