"""Self-check for the delegation rules and their oracle in delegation.py.

delegation.py judges a Claude run's child agents against what the workflows
require: the steps whose instructions require a child (REQUIRED), those that
only suggest one (SUGGESTED, never enforced), what an epic child and a spec
review child must be told, and that the main agent sets no timed wake-up
while a child of its own is running. The cases here build small runs by hand
(a Claude stream-json transcript under tmp_path, and what `spektacular stats`
answers for Spektacular's logs: lanes, step entries and helper spans), read
them with `read_run` exactly as a verifier does, and assert the named
failures with hand-written expected values. They also pin the transcript
rules on the two epic reference runs under data/, and tie the hand-kept
oracle and the worktree folder to the step templates and internal/worktree,
so they cannot drift apart unnoticed.

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
import runlog  # noqa: E402
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
    worktree_folder,
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
WT1 = f"/app/.spektacular/worktrees/{NAME}/cli"
WT2 = f"/app/.spektacular/worktrees/{NAME}/docs"


# --- Spektacular's logs, as `spektacular stats` answers for them ----------------------


def m(minute: int, second: int = 0) -> str:
    """A clock time on the test day: 10:<minute>:<second>."""
    return f"10:{minute:02d}:{second:02d}"


def cli_ts(clock: str) -> str:
    return f"2026-10-23T{clock}.000000000Z"


def walk(kind, name, steps, first=0):
    """A lane entering each step a minute after the last, from 10:<first>:00:
    its step entries as (step, start, end, end_reason). The last step is the
    one the lane ended on."""
    entries = []
    for i, step in enumerate(steps, start=first):
        last = i == first + len(steps) - 1
        end = m(i) if step == "finished" else m(i if last else i + 1)
        entries.append((step, m(i), end, "finished" if step == "finished" else ("open" if last else "moved")))
    return entries


def trace_of(kind, name) -> str:
    return f"{abs(hash((kind, name))) % 16**8:08x}" + "0" * 24


class Canned:
    """Answers `spektacular stats` for hand-built lanes and spans."""

    def __init__(self, lanes, spans):
        self.answers = {"list": {"runs": []}, "commands": {"commands": []}, "spans": {"spans": list(spans)}}
        for kind, name, entries in lanes:
            run_id = trace_of(kind, name)
            self.answers["list"]["runs"].append({
                "run_id": run_id, "kind": kind, "name": name,
                "started_at": cli_ts(entries[0][1]), "ended_at": cli_ts(entries[-1][2]), "status": "finished",
            })
            self.answers["commands"]["commands"].append({
                "run_id": run_id, "kind": kind, "name": name, "span_id": f"n{run_id}", "command": f"{kind} new",
                "args": [kind, "new", "--data", json.dumps({"name": name})], "step": None, "step_to": entries[0][0],
                "started_at": cli_ts(entries[0][1]), "ended_at": cli_ts(entries[0][1]), "exit_code": 0, "message": None,
            })
            self.answers[f"run {run_id}"] = {"step_entries": [
                {"step": step, "started_at": cli_ts(start), "ended_at": cli_ts(end), "end_reason": reason, "complete": reason != "open"}
                for step, start, end, reason in entries
            ]}

    def __call__(self, argv, **kwargs):
        sub = argv[2]
        key = f"run {argv[argv.index('--run') + 1]}" if sub == "run" else sub
        return type("Done", (), {"stdout": json.dumps({"error": False, **self.answers[key]})})()


def helper(kind, name, step, clock, *, span_id=None, parent=None, outside=False, end=None, complete=True):
    """A `sub-agent` span row, started at `clock` in a lane's step, or the
    orchestrator's outside any run."""
    start = cli_ts(clock)
    return {
        "span_id": span_id or f"h{clock}{step}", "parent_span_id": parent, "name": f"helper {step}", "type": "sub-agent",
        "trace_id": "t", "run_id": None if outside else trace_of(kind, name), "kind": None if outside else kind,
        "workflow": None if outside else name, "step": None if outside else step, "task_id": None,
        "started_at": start, "ended_at": cli_ts(end) if end else start, "end_reason": "stopped" if complete else "open",
        "status": "ok" if complete else None, "complete": complete,
    }


def orchestrator(clock, end=None, *, span_id=None):
    """An epic orchestrator's helper span outside any run; never stopped when `end` is None."""
    return helper(None, None, None, clock, span_id=span_id, outside=True, end=end, complete=end is not None)


def reader(tmp_path, lanes, spans=()) -> runlog.RunLog:
    return runlog.RunLog(tmp_path, binary="spektacular", runner=Canned(lanes, spans))


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


def run_of(tmp_path, lanes, objs, spans=()):
    """Writes the transcript under tmp_path and reads it back with the
    canned logs (lanes: (kind, name, entries)), as a verifier does."""
    tmp_path.mkdir(parents=True, exist_ok=True)
    path = tmp_path / "claude-code.txt"
    write_jsonl(path, objs)
    real = delegation.runlog.RunLog
    delegation.runlog.RunLog = lambda log_dir: real(log_dir, binary="spektacular", runner=Canned(lanes, spans))
    try:
        return read_run(path, tmp_path)
    finally:
        delegation.runlog.RunLog = real


def write_jsonl(path: Path, objs) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))


def required(run):
    return missing_required(run.log, run.lanes, run.spans)


def step_helpers(kind, steps, skip=(), first=0, name=NAME):
    """A helper span started half a minute into each REQUIRED step of a lane
    walked from 10:<first>:00, except those in `skip`."""
    return [
        helper(kind, name, step, m(i, 30), end=m(i, 50))
        for i, step in enumerate(steps, start=first)
        if (kind, step) in REQUIRED and step not in skip
    ]


def missing(kind, step, attempt=None):
    suffix = f" (attempt {attempt})" if attempt else ""
    return Failure("missing_child", f"{kind}:{NAME}:{step}{suffix}", f"the {step} step completed without a child agent started in it")


# --- required steps (criterion 1), judged from helper spans -------------------------


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_required_step_without_a_helper_span_fails_naming_that_step(tmp_path, entry):
    kind, step = entry
    run = run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], transcript(), step_helpers(kind, STEPS[kind], skip={step}))
    assert required(run) == [missing(kind, step)]


@pytest.mark.parametrize("entry", sorted(REQUIRED), ids=lambda e: f"{e[0]}-{e[1]}")
def test_required_step_with_a_helper_span_started_in_it_passes(tmp_path, entry):
    kind, step = entry
    run = run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], transcript(), step_helpers(kind, STEPS[kind]))
    assert required(run) == []


def test_a_helper_started_in_another_step_does_not_satisfy_a_required_one(tmp_path):
    # the goal check's reviewer is launched in the verify step before it
    spans = [helper("implement", NAME, "verify", m(4, 30)), *step_helpers("implement", IMPLEMENT_STEPS, skip={"check_goal"})]
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], transcript(), spans)
    assert required(run) == [missing("implement", "check_goal")]


def test_a_helper_of_another_lane_does_not_satisfy_a_required_step(tmp_path):
    lanes = [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS)), ("implement", NAME_B, walk("implement", NAME_B, IMPLEMENT_STEPS))]
    run = run_of(tmp_path, lanes, transcript(), step_helpers("implement", IMPLEMENT_STEPS, name=NAME_B))
    assert required(run) == [missing("implement", "check_goal")]


def test_an_orchestrators_helper_does_not_satisfy_a_required_step(tmp_path):
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], transcript(), [orchestrator("09:59:00", m(9))])
    assert required(run) == [missing("implement", "check_goal")]


def test_a_nested_helper_satisfies_a_required_step(tmp_path):
    # The spec's own child works the lane; the reviewer span nests under its span.
    spans = [helper("implement", NAME, "check_goal", m(5, 30), parent="worker", end=m(5, 50))]
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], transcript(), spans)
    assert required(run) == []


def test_the_step_a_lane_ended_on_is_not_judged(tmp_path):
    steps = ["read_plan", "analyze", "implement", "test", "verify", "check_goal"]  # cut off in check_goal
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, steps))], transcript())
    assert required(run) == []


def test_the_same_step_once_moved_on_from_is_judged(tmp_path):
    steps = ["read_plan", "analyze", "implement", "test", "verify", "check_goal", "update_plan"]
    spans = [helper("implement", NAME, "update_plan", m(6, 30))]
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, steps))], transcript(), spans)
    assert required(run) == [missing("implement", "check_goal")]


RETRIED = ["read_plan", "implement", "test", "verify", "check_goal", "implement", "test", "verify", "check_goal", "update_plan", "finished"]


@pytest.mark.parametrize(
    "childless, expected",
    [
        (8, missing("implement", "check_goal", attempt=2)),  # the second goal check
        (4, missing("implement", "check_goal")),  # the first: no attempt number
    ],
)
def test_a_retried_step_is_judged_per_attempt(tmp_path, childless, expected):
    spans = [helper("implement", NAME, s, m(i, 30)) for i, s in enumerate(RETRIED) if ("implement", s) in REQUIRED and i != childless]
    run = run_of(tmp_path, [("implement", NAME, walk("implement", NAME, RETRIED))], transcript(), spans)
    assert required(run) == [expected]


def resumed_lane():
    """An implement lane whose check_goal step a second child resumed: two
    entries at check_goal, 10:02-10:03 (interrupted) and 10:04-10:05."""
    return [
        ("read_plan", m(0), m(1), "moved"),
        ("verify", m(1), m(2), "moved"),
        ("check_goal", m(2), m(3), "interrupted"),
        ("check_goal", m(4), m(5), "moved"),
        ("update_plan", m(5), m(6), "moved"),
        ("update_changelog", m(6), m(7), "moved"),
        ("finished", m(7), m(7), "finished"),
    ]


@pytest.mark.parametrize("launched", [m(4, 30), m(2, 30)], ids=["resumed-entry", "first-entry"])
def test_a_helper_in_either_entry_of_a_resumed_step_satisfies_it(tmp_path, launched):
    spans = [helper("implement", NAME, "check_goal", launched)]
    run = run_of(tmp_path, [("implement", NAME, resumed_lane())], transcript(), spans)
    assert required(run) == []


def test_a_resumed_step_without_a_helper_fails_once(tmp_path):
    run = run_of(tmp_path, [("implement", NAME, resumed_lane())], transcript())
    assert required(run) == [missing("implement", "check_goal")]


def test_attempts_group_consecutive_entries_of_one_step():
    entries = [("r", {"step": s}) for s in ["a", "b", "b", "c", "b"]]
    assert [(step, len(group)) for step, group in delegation.attempts(entries)] == [("a", 1), ("b", 2), ("c", 1), ("b", 1)]


def test_unreadable_logs_fail_the_span_rules_naming_why(tmp_path, monkeypatch):
    monkeypatch.delenv("SPEKTACULAR_BIN", raising=False)
    monkeypatch.setattr(runlog.shutil, "which", lambda name: None)
    path = tmp_path / "claude-code.txt"
    write_jsonl(path, transcript())
    run = read_run(path, tmp_path / "missing")
    assert run.unavailable == f"no Spektacular logs at {tmp_path / 'missing'}" or "SPEKTACULAR_BIN" in run.unavailable
    for failures in (required(run), missing_spec_children(run.log, run.lanes, run.spans)):
        assert [f.rule for f in failures] == ["logs_unavailable"]
        assert failures[0].detail == run.unavailable


# --- suggested steps (criterion 2) -----------------------------------------------


def test_suggested_and_required_steps_do_not_overlap():
    assert SUGGESTED == {("plan", "discovery"), ("implement", "analyze"), ("implement", "test"), ("implement", "update_changelog")}
    assert SUGGESTED.isdisjoint(REQUIRED)


def orchestrated_prompt(name, *extra):
    return " ".join([f"Work spec {name} in the project at /app.", 'Start it with {"orchestrated": true}.', *extra])


def test_a_run_whose_suggested_steps_used_no_child_passes_every_rule(tmp_path):
    # An epic run: a plan lane and an implement lane, each worked by its own
    # child. No helper in discovery, analyze or update_changelog.
    objs = transcript(
        child("toolu_P", "09:59:00", orchestrated_prompt(NAME_B), ended=m(5)),
        child("toolu_I", "09:59:10", orchestrated_prompt(NAME, f"Your code is at {WT1} and {WT2}."), ended=m(9)),
    )
    spans = [orchestrator("09:58:50", m(5)), orchestrator("09:59:05", m(9)), *step_helpers("implement", IMPLEMENT_STEPS)]
    lanes = [("plan", NAME_B, walk("plan", NAME_B, PLAN_STEPS)), ("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))]
    run = run_of(tmp_path, lanes, objs, spans)
    assert sorted(s.step for s in run.spans if s.run_id) == ["check_goal"]
    assert required(run) == []
    assert missing_spec_children(run.log, run.lanes, run.spans) == []
    assert epic_context_gaps(run.children, run.lanes) == []
    assert review_context_gaps(run.children, run.spans) == []
    assert wakeups_while_children_run(run.wakeups, run.children, run.end) == []


# --- epic children's context (criterion 3) ---------------------------------------

PARTS = {
    "spec": f"Plan spec {NAME}.",
    "root": "The project is at /app.",
    "orchestrated": 'Start it with --data \'{"orchestrated": true}\'.',
    "worktrees": f"Your code is at {WT1} and {WT2}.",  # both under the worktree folder
    "resume": "Resume the spec's earlier run.",
}


def prompt(omit=(), **replace):
    parts = {**PARTS, **replace}
    return " ".join(v for k, v in parts.items() if k not in omit)


def epic_lane(tmp_path, kind, prompts):
    """One epic lane worked by a child per prompt."""
    objs = transcript(*[child(f"toolu_C{n}", f"09:5{n}:00", p, ended=f"09:5{n}:30") for n, p in enumerate(prompts, start=1)])
    return run_of(tmp_path, [(kind, NAME, walk(kind, NAME, STEPS[kind]))], objs)


def gaps(run):
    return epic_context_gaps(run.children, run.lanes)


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
    # The worktree folder names both the spec and the project root, so a
    # child missing either is given no worktree folder either.
    omit = {item} | ({"worktrees"} if item in ("spec", "root") else set())
    run = epic_lane(tmp_path, kind, [prompt(omit=omit)])
    expected = [gap("toolu_C1", ITEM_DETAIL[item])]
    if kind == "implement" and "worktrees" in omit:
        expected.append(gap("toolu_C1", f"its worktree folder /app/.spektacular/worktrees/{NAME}"))
    assert gaps(run) == expected


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


def test_an_implementing_child_naming_one_worktree_under_the_folder_passes(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(worktrees=f"Your code is at {WT2}.")])
    assert gaps(run) == []


def test_an_implementing_child_missing_its_worktree_folder_fails_naming_it(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(omit={"worktrees"})])
    assert gaps(run) == [gap("toolu_C1", f"its worktree folder /app/.spektacular/worktrees/{NAME}")]


def test_an_implementing_child_given_another_specs_folder_fails(tmp_path):
    run = epic_lane(tmp_path, "implement", [prompt(worktrees=f"Your code is at /app/.spektacular/worktrees/{NAME_B}/cli.")])
    assert gaps(run) == [gap("toolu_C1", f"its worktree folder /app/.spektacular/worktrees/{NAME}")]


def test_a_planning_child_is_not_given_a_worktree_folder(tmp_path):
    run = epic_lane(tmp_path, "plan", [prompt(omit={"worktrees"})])
    assert gaps(run) == []


def test_the_worktree_folder_is_the_specs_under_the_project_root():
    assert worktree_folder(NAME) == f"/app/.spektacular/worktrees/{NAME}"
    assert worktree_folder(NAME, "/srv/p") == f"/srv/p/.spektacular/worktrees/{NAME}"


def test_worktree_dir_matches_go():
    go = (Path(__file__).resolve().parents[4] / "internal" / "worktree" / "worktree.go").read_text()
    import re

    m_ = re.search(r'^const Dir = "([^"]+)"', go, re.M)
    assert m_, "internal/worktree/worktree.go no longer declares `const Dir`"
    assert delegation.WORKTREE_DIR == ".spektacular/" + m_[1]


def test_a_drifted_worktree_dir_fails_the_tie(tmp_path, monkeypatch):
    monkeypatch.setattr(delegation, "WORKTREE_DIR", ".spektacular/trees")
    import pytest as _pytest

    with _pytest.raises(AssertionError):
        test_worktree_dir_matches_go()


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
        gap("toolu_C2", f"its worktree folder /app/.spektacular/worktrees/{NAME}"),
        gap("toolu_C2", "the instruction to resume the spec's earlier run"),
    ]


# --- spec review children (criterion 3) ------------------------------------------


def review_run(tmp_path, review_prompt, step="verification", span_step=None, kind="spec"):
    """A spec lane whose `step` launched a child at half a minute in, its
    helper span started ten seconds before the launch."""
    i = SPEC_STEPS.index(step)
    objs = transcript(child("toolu_R", m(i, 30), review_prompt, ended=m(i, 50)))
    spans = [helper(kind, NAME, span_step or step, m(i, 20))]
    return run_of(tmp_path, [(kind, NAME, walk(kind, NAME, SPEC_STEPS))], objs, spans)


@pytest.mark.parametrize("words", [REVIEW_PROMPT, f"Read {STAGED}. Judge it Exactly As Written, nothing more."])
def test_a_review_child_given_the_staged_spec_and_told_to_review_only_it_passes(tmp_path, words):
    run = review_run(tmp_path, words)
    assert delegation.match_spans(run.children, run.spans)["toolu_R"].step == "verification"
    assert review_context_gaps(run.children, run.spans) == []


def test_a_review_child_not_given_the_staged_spec_fails_naming_it(tmp_path):
    run = review_run(tmp_path, "Read the spec and review only what is written in that file.")
    assert review_context_gaps(run.children, run.spans) == [
        Failure("context_gap", f"child toolu_R ({NAME})", f"its prompt does not give the staged spec to read ({STAGED})")
    ]


def test_a_review_child_not_told_to_review_only_what_is_written_fails_naming_it(tmp_path):
    run = review_run(tmp_path, f"Read {STAGED} and review it.")
    assert review_context_gaps(run.children, run.spans) == [
        Failure("context_gap", f"child toolu_R ({NAME})", "its prompt does not give the instruction to review only what is written")
    ]


def test_a_child_outside_the_spec_verification_step_is_not_a_reviewer(tmp_path):
    run = review_run(tmp_path, "Look around.", step="requirements")
    assert review_context_gaps(run.children, run.spans) == []


def test_a_child_with_no_span_is_not_joined_to_a_step(tmp_path):
    i = SPEC_STEPS.index("verification")
    objs = transcript(child("toolu_R", m(i, 30), "Look around.", ended=m(i, 50)))
    # the only span started three minutes before the launch: too early
    run = run_of(tmp_path, [("spec", NAME, walk("spec", NAME, SPEC_STEPS))], objs, [helper("spec", NAME, "verification", m(i - 3, 30))])
    assert delegation.match_spans(run.children, run.spans) == {}
    assert review_context_gaps(run.children, run.spans) == []


# --- one child per epic spec ------------------------------------------------------


def two_plan_lanes(tmp_path, *spans):
    """Two plan lanes, the first started at 10:00 and the second at 10:10."""
    lanes = [("plan", NAME, walk("plan", NAME, PLAN_STEPS)), ("plan", NAME_B, walk("plan", NAME_B, PLAN_STEPS, first=10))]
    return run_of(tmp_path, lanes, transcript(), spans)


def spec_children_of(run):
    return missing_spec_children(run.log, run.lanes, run.spans)


def test_each_epic_spec_started_under_an_orchestrator_span_passes(tmp_path):
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00", m(5)), orchestrator("10:09:30", m(15)))
    assert spec_children_of(run) == []


def missing_b():
    return [Failure("missing_spec_child", f"plan:{NAME_B}", f"no child of the orchestrator was started for spec {NAME_B}")]


def test_an_epic_spec_started_with_no_orchestrator_span_open_fails_naming_it(tmp_path):
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00", m(5)))
    assert spec_children_of(run) == missing_b()


def test_a_span_never_stopped_counts_as_open_from_its_start(tmp_path):
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00"), orchestrator("10:09:00"))
    assert spec_children_of(run) == []


def test_two_lanes_need_a_span_each(tmp_path):
    # one span, never stopped, open at both starts: the first lane takes it
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00"))
    assert spec_children_of(run) == missing_b()


def test_a_span_started_after_the_lane_does_not_count(tmp_path):
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00", m(5)), orchestrator("10:10:30", m(15)))
    assert spec_children_of(run) == missing_b()


def test_a_helper_inside_a_run_does_not_work_a_spec_for_the_orchestrator(tmp_path):
    run = two_plan_lanes(tmp_path, orchestrator("09:59:00", m(5)), helper("plan", NAME, "verification", "10:09:30", end=m(12)))
    assert spec_children_of(run) == missing_b()


def test_a_spec_lane_needs_no_child_of_its_own(tmp_path):
    run = run_of(tmp_path / "spec", [("spec", NAME, walk("spec", NAME, SPEC_STEPS))], transcript())
    assert spec_children_of(run) == []
    run = run_of(tmp_path / "implement", [("implement", NAME, walk("implement", NAME, IMPLEMENT_STEPS))], transcript())
    assert spec_children_of(run) == [
        Failure("missing_spec_child", f"implement:{NAME}", f"no child of the orchestrator was started for spec {NAME}")
    ]


def test_epic_children_are_found_by_the_spec_their_prompt_names():
    a = delegation.Child("a", None, None, None, None, None, None, None, None, None, None, f"plan {NAME} then {NAME_B}")
    b = delegation.Child("b", None, None, None, None, None, None, None, None, None, None, f"plan {NAME_B}")
    n = delegation.Child("n", "a", None, None, None, None, None, None, None, None, None, f"look at {NAME_B}")
    names = [NAME, NAME_B]
    assert [c.id for c in delegation.spec_children([a, b, n], ("plan", NAME), names)] == ["a"]
    assert [c.id for c in delegation.spec_children([a, b, n], ("plan", NAME_B), names)] == ["b"]
    # a run that worked one spec alone: every top-level child is its
    assert [c.id for c in delegation.spec_children([a, b, n], ("plan", NAME), [NAME])] == ["a", "b"]


def test_lane_for_takes_the_spec_the_prompt_names_first():
    assert delegation.lane_for(f"plan {NAME_B}, after {NAME}", [NAME, NAME_B]) == NAME_B
    assert delegation.lane_for("look around", [NAME, NAME_B]) is None


def test_lane_for_takes_the_longer_name_where_one_contains_another():
    assert delegation.lane_for(f"plan {NAME}-api", [NAME, f"{NAME}-api"]) == f"{NAME}-api"


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
    assert wakeups_while_children_run(run.wakeups, run.children, run.end) == [
        Failure("wakeup_while_children_run", "ScheduleWakeup delaySeconds=60", "set at 2026-10-23T10:00:10+00:00 while a child was running"),
        Failure("wakeup_while_children_run", "Bash sleep 60", "set at 2026-10-23T10:02:00+00:00 while a child was running"),
    ]


def test_a_child_without_an_end_runs_until_the_run_ends(tmp_path):
    run = wakeup_run(tmp_path, ended=None)
    assert run.end == datetime.fromisoformat("2026-10-23T10:06:00+00:00")
    subjects = ["ScheduleWakeup delaySeconds=60", "Bash sleep 60", "CronCreate */5 * * * *"]
    assert [f.subject for f in wakeups_while_children_run(run.wakeups, run.children, run.end)] == subjects
    assert [f.subject for f in wakeups_while_children_run(run.wakeups, run.children)] == subjects


def test_a_wakeup_with_no_child_running_passes(tmp_path):
    objs = transcript(assistant("10:00:05", tool_use("w0", "ScheduleWakeup", {"delaySeconds": 30})))
    run = run_of(tmp_path, [("plan", NAME, walk("plan", NAME, PLAN_STEPS))], objs)
    assert wakeups_while_children_run(run.wakeups, run.children, run.end) == []


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
    names = [NAME, NAME_B]
    for run, background in ((fg, False), (bg, True)):
        assert [(c.id, c.background) for c in delegation.spec_children(run.children, ("plan", NAME), names)] == [("toolu_A", background)]
        assert [(c.id, c.background) for c in delegation.spec_children(run.children, ("plan", NAME_B), names)] == [("toolu_B", background)]
    at = lambda clock: datetime.fromisoformat(f"2026-10-23T{clock}+00:00")  # noqa: E731
    assert [(t.ended_at, t.woken_by) for t in fg.turns] == [(at("10:05:10"), "start")]
    assert [(t.ended_at, t.woken_by) for t in bg.turns] == [(at("10:00:10"), "start"), (at("10:05:10"), "task-notification")]


def test_foreground_children_held_inside_the_turn_pass_the_foreground_rule(tmp_path):
    run = epic_children_run(tmp_path, background=False)
    assert epic_children_foreground_together(run.children, run.turns, PLAN_LANES) == []


def test_background_children_fail_the_foreground_rule_naming_each_child(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    turn_end = "a turn ended at 2026-10-23T10:00:10+00:00 while it was running"
    assert epic_children_foreground_together(run.children, run.turns, PLAN_LANES) == [
        Failure("child_in_background", f"child toolu_A ({NAME})", "it was started in the background"),
        Failure("turn_ended_while_child_ran", f"child toolu_A ({NAME})", turn_end),
        Failure("child_in_background", f"child toolu_B ({NAME_B})", "it was started in the background"),
        Failure("turn_ended_while_child_ran", f"child toolu_B ({NAME_B})", turn_end),
    ]


def test_background_children_woken_by_their_notice_pass_the_background_rule(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    assert epic_children_backgrounded(run.children, run.turns, PLAN_LANES) == []


def test_foreground_children_fail_the_background_rule(tmp_path):
    run = epic_children_run(tmp_path, background=False)
    assert epic_children_backgrounded(run.children, run.turns, PLAN_LANES) == [
        Failure("child_in_foreground", f"child toolu_A ({NAME})", "it was started in the foreground"),
        Failure("child_in_foreground", f"child toolu_B ({NAME_B})", "it was started in the foreground"),
        Failure("no_turn_ended_while_children_ran", "orchestrator", "no turn ended while a child was running"),
        Failure("no_notice_woken_turn", "orchestrator", "no turn was started by a child's completion notice"),
    ]


def test_spec_lanes_are_not_judged_by_either_epic_children_rule(tmp_path):
    run = epic_children_run(tmp_path, background=True)
    assert epic_children_foreground_together(run.children, run.turns, [("spec", NAME)]) == []
    assert epic_children_backgrounded(run.children, run.turns, [("spec", NAME)]) == []


def test_the_settled_rule_follows_the_probe_outcome(tmp_path, monkeypatch):
    fg = epic_children_run(tmp_path / "fg", background=False)
    bg = epic_children_run(tmp_path / "bg", background=True)
    monkeypatch.setattr(delegation, "PROBE_OUTCOME", "stopped")
    assert epic_children_run_as_settled(fg.children, fg.turns, PLAN_LANES) == []
    assert {f.rule for f in epic_children_run_as_settled(bg.children, bg.turns, PLAN_LANES)} == {"child_in_background", "turn_ended_while_child_ran"}
    monkeypatch.setattr(delegation, "PROBE_OUTCOME", "completed")
    assert epic_children_run_as_settled(bg.children, bg.turns, PLAN_LANES) == []
    assert {f.rule for f in epic_children_run_as_settled(fg.children, fg.turns, PLAN_LANES)} == {
        "child_in_foreground",
        "no_turn_ended_while_children_ran",
        "no_notice_woken_turn",
    }


def test_the_probe_outcome_matches_the_recorded_probe():
    line = (DATA / "probe-background-child" / "OUTCOME").read_text()
    assert line.split()[0].rstrip(":") == delegation.PROBE_OUTCOME == "stopped"


# --- the reference runs (criterion 4) ---------------------------------------------


REFERENCE_LANES = [("plan", "20260101000001-storage"), ("plan", "20260101000002-commands"), ("plan", "20260101000003-help")]


def reference(name):
    """A reference run's transcript rules. Its Spektacular logs predate the
    format `stats` reads, so its lanes are given by hand."""
    run = read_run(DATA / name / "claude-code.txt", DATA / name / "missing-logs")
    run.lanes = dict.fromkeys(REFERENCE_LANES)
    return run


def test_the_55_epic_run_fails_the_wakeup_rule_naming_the_scheduled_wakeup():
    run = reference("s55-epic")
    failures = wakeups_while_children_run(run.wakeups, run.children, run.end)
    assert [(f.rule, f.subject) for f in failures] == [("wakeup_while_children_run", "ScheduleWakeup delaySeconds=1200")]


def test_the_46_epic_run_passes_the_wakeup_rule():
    run = reference("m70-epic")
    assert wakeups_while_children_run(run.wakeups, run.children, run.end) == []


def test_the_46_epic_run_passes_the_settled_children_rule():
    run = reference("m70-epic")
    assert epic_children_run_as_settled(run.children, run.turns, REFERENCE_LANES) == []


def test_the_55_epic_run_fails_the_settled_children_rule_naming_its_background_children():
    run = reference("s55-epic")
    failures = epic_children_run_as_settled(run.children, run.turns, REFERENCE_LANES)
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
def test_both_epic_reference_runs_pass_the_other_transcript_rules(name):
    run = reference(name)
    # plan epics: three plan lanes, one top-level child each
    names = [n for _, n in REFERENCE_LANES]
    assert [len(delegation.spec_children(run.children, lane, names)) for lane in REFERENCE_LANES] == [1, 1, 1]
    assert epic_context_gaps(run.children, run.lanes) == []


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


def launched(cid, clock):
    from transcripts import Child

    return Child(
        id=cid, parent=None, kind="general-purpose", description=f"child {cid}", background=False,
        launched_at=datetime.fromisoformat(f"2026-10-23T{clock}+00:00"), ended_at=None, duration_s=None,
        ending=None, hand_back=None, models=None, prompt=None,
    )


def sub_agent(span_id, clock, span_type="sub-agent"):
    at = datetime.fromisoformat(f"2026-10-23T{clock}+00:00")
    return runlog.Span(
        span_id=span_id, parent=None, type=span_type, name=span_id, run_id="r", kind="implement", spec=NAME,
        step="check_goal", task_id=None, start=at, end=at, end_reason="open", complete=False,
    )


def test_each_launch_needs_its_own_sub_agent_span_just_before_it():
    children = [launched("a", "10:05:00"), launched("b", "10:05:00"), launched("c", "10:20:00")]
    spans = [
        sub_agent("s1", "10:04:30"),  # 30 s before both a and b: marks one of them
        sub_agent("s2", "10:05:03"),  # within the slack after a launch
        sub_agent("w1", "10:19:50", "user-wait"),  # not a sub-agent span
        sub_agent("s3", "10:10:00"),  # ten minutes before c: too early
    ]
    assert sorted(delegation.match_spans(children, spans)) == ["a", "b"]
    assert [c.id for c in delegation.unmarked(children, spans)] == ["c"]
    assert [str(f) for f in delegation.unmarked_launches(children, spans)] == [
        "unmarked_launch child c: child child c launched at 10:20:00 has no sub-agent span (`trace start <helper> --type sub-agent`)"
    ]
    assert delegation.marked_rate(children, spans) == 2 / 3
    assert delegation.LAUNCH_SPAN_WINDOW_S == 120


def test_no_timed_launches_has_no_rate():
    assert delegation.marked_rate([], [sub_agent("s1", "10:00:00")]) is None
