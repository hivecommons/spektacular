"""Self-check for the epic suites' child checks (TestChildren).

The epic-workflow and implement-epic-workflow verifiers judge a Claude run's
child agents with the rules in delegation.py: every spec worked by its own
child, each child given its context, no wake-up set while children run, and,
for implement-epic, a child in every test, verify and goal-check step. The
rules themselves are pinned in test_delegation.py; this file pins the
verifiers' wiring of them. It calls each TestChildren method directly with a
`delegation.Run`, read from the two epic reference runs under data/ or from a
small implement-epic run built by hand under tmp_path, and asserts the
failure lines each method reports, with hand-written expected values. The
run's Spektacular logs are given as what `spektacular stats` answers for
them (lanes, step entries and sub-agent spans); the reference runs' logs
predate that format, so their lanes are given by hand. It
also checks that the child checks skip for other agents and that no other
check in either verifier reads the transcript, so Codex and Bob runs are
checked exactly as before.

The verifier modules are imported as modules, never their classes, so pytest
does not collect their tests here. This file never runs in a harbor
container. Run it locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_epic_children.py
"""

import ast
import json
import sys
from datetime import datetime
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
HARBOR = HERE.parents[1]
for path in (HARBOR / "shared", HARBOR / "epic-workflow" / "tests", HARBOR / "implement-epic-workflow" / "tests"):
    sys.path.insert(0, str(path))

import delegation  # noqa: E402
import runlog  # noqa: E402
import test_epic_workflow as epic_verifier  # noqa: E402
import test_implement_epic_workflow as implement_verifier  # noqa: E402

DATA = HERE / "data"
VERIFIERS = {"epic": epic_verifier, "implement-epic": implement_verifier}

A = "20260201000001-storage"
B = "20260201000002-format"
SPECS = [A, B]
LANE_STEPS = ["test", "verify", "check_goal", "update_plan"]
SONNET = "claude-sonnet-4-6"
HAIKU = "claude-haiku-4-5"
RULES = (
    "missing_child ",
    "missing_spec_child ",
    "context_gap ",
    "wakeup_while_children_run ",
    "child_in_background ",
    "turn_ended_while_child_ran ",
    "child_in_foreground ",
    "no_turn_ended_while_children_ran ",
    "no_notice_woken_turn ",
    "unmarked_launch ",
)


def verdict(test, run) -> list[str]:
    """The failure lines a TestChildren method reports on `run`; [] when it
    passes. pytest's assertion rewriting indents each continuation line of
    a multi-line message, so lines are read without their leading space."""
    try:
        test(run)
    except AssertionError as e:
        lines = [line.lstrip(" ~") for line in str(e).splitlines()]
        lines = [line for line in lines if line.startswith(RULES)]
        assert lines, f"failed without naming a rule: {e}"
        return lines
    return []


# --- Spektacular's logs, as `spektacular stats` answers for them ----------------------


def m(minute: int, second: int = 0) -> str:
    """A clock time on the test day: 10:<minute>:<second>."""
    return f"10:{minute:02d}:{second:02d}"


def ts(clock: str) -> str:
    return f"2026-10-23T{clock}.000000000Z"


def trace_of(spec: str) -> str:
    return f"{SPECS.index(spec) + 1:08x}" + "0" * 24


def lane_answers(spec: str) -> tuple[dict, dict, dict]:
    """An implement lane entering each of LANE_STEPS a minute after the last,
    from 10:00: its run summary, its `implement new` and its step entries."""
    run_id = trace_of(spec)
    summary = {"run_id": run_id, "kind": "implement", "name": spec, "started_at": ts(m(0)), "ended_at": ts(m(len(LANE_STEPS))), "status": "in_progress"}
    new = {
        "run_id": run_id, "kind": "implement", "name": spec, "span_id": f"n{run_id}", "command": "implement new",
        "args": ["implement", "new", "--data", json.dumps({"name": spec})], "step": None, "step_to": LANE_STEPS[0],
        "started_at": ts(m(0)), "ended_at": ts(m(0)), "exit_code": 0, "message": None,
    }
    entries = [
        {"step": step, "started_at": ts(m(i)), "ended_at": ts(m(i + 1)), "end_reason": "moved", "complete": True}
        for i, step in enumerate(LANE_STEPS)
    ]
    return summary, new, {"step_entries": entries}


def span_row(clock, span_id, spec=None, step=None, end=None):
    """A sub-agent span row: an orchestrator's outside any run when `spec` is
    None, else one started in the spec's lane at `step`."""
    return {
        "span_id": span_id, "parent_span_id": None, "name": span_id, "type": "sub-agent", "trace_id": "t",
        "run_id": trace_of(spec) if spec else None, "kind": "implement" if spec else None, "workflow": spec,
        "step": step, "task_id": None, "started_at": ts(clock), "ended_at": ts(end or clock),
        "end_reason": "stopped", "status": "ok", "complete": True,
    }


class Canned:
    def __init__(self, answers):
        self.answers = answers

    def __call__(self, argv, **kwargs):
        sub = argv[2]
        key = f"run {argv[argv.index('--run') + 1]}" if sub == "run" else sub
        return type("Done", (), {"stdout": json.dumps({"error": False, **self.answers[key]})})()


def write_jsonl(path: Path, objs) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))


def read_with(path: Path, log_dir: Path, answers: dict | None) -> delegation.Run:
    """delegation.read_run over canned `stats` answers (None: no readable logs)."""
    real = delegation.runlog.RunLog
    if answers is None:
        delegation.runlog.RunLog = lambda d: real(d, binary="spektacular", runner=lambda *a, **k: (_ for _ in ()).throw(OSError("no binary")))
    else:
        delegation.runlog.RunLog = lambda d: real(d, binary="spektacular", runner=Canned(answers))
    try:
        return delegation.read_run(path, log_dir)
    finally:
        delegation.runlog.RunLog = real


# --- the Claude transcript --------------------------------------------------------


def epoch_ms(clock: str) -> int:
    return int(datetime.fromisoformat(f"2026-10-23T{clock}+00:00").timestamp() * 1000)


def assistant(clock, *content, parent=None, model=SONNET):
    return {
        "type": "assistant",
        "timestamp": f"2026-10-23T{clock}Z",
        "parent_tool_use_id": parent,
        "message": {"model": model, "role": "assistant", "content": list(content)},
    }


def tool_use(tid, name, inp):
    return {"type": "tool_use", "id": tid, "name": name, "input": inp}


def child(tid, launched, prompt, ended, parent=None, background=False):
    """A child the main agent (or the child `parent`) started at `launched`
    and that ended at `ended`, as Claude logs it, in the background when
    `background`."""
    started = {
        "type": "system",
        "subtype": "task_started",
        "tool_use_id": tid,
        "task_id": f"task_{tid}",
        "task_type": "local_agent",
        "is_backgrounded": background,
        "spawn_depth": 1 if parent is None else 2,
    }
    if parent is not None:
        started["parent_task_id"] = f"task_{parent}"
    return [
        assistant(
            launched,
            tool_use(tid, "Agent", {"subagent_type": "general-purpose", "prompt": prompt, "description": tid}),
            parent=parent,
            model=SONNET if parent is None else HAIKU,
        ),
        started,
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


def worktree(spec):
    return f"/app/.spektacular/worktrees/{spec}/app"


def spec_prompt(spec, worktrees):
    return " ".join(
        [
            f"Implement spec {spec} in the project at /app.",
            'Start it with --data \'{"orchestrated": true}\'.',
            f"Your code is at {' and '.join(worktrees)}.",
        ]
    )


def implement_epic_run(tmp_path, *, skip_spec=None, skip_step=None, short_prompt=None, extra=(), background=False, marked="all") -> delegation.Run:
    """Two implement lanes walked side by side from 10:00, each worked by a
    top-level child of its own launched before the lanes start, which starts
    a child half a minute into each check_goal step. The spec children run
    in the foreground and end at 10:05, and the orchestrator's one turn ends
    at 10:05:30 after them; with `background`, they run in the background and
    the turn ends at 09:59:10, once both are launched.

    skip_spec: the spec no top-level child works. skip_step: (spec, step)
    whose nested child is never started. short_prompt: the spec whose child
    is not given its worktree folder. extra: more transcript objects.
    marked: the launch clocks that get a sub-agent span (every launch when
    "all"): the orchestrator's span outside any run for a spec child, a span
    in the lane's step for a nested one.
    """
    answers = {"list": {"runs": []}, "commands": {"commands": []}, "spans": {"spans": []}}
    for spec in SPECS:
        summary, new, report = lane_answers(spec)
        answers["list"]["runs"].append(summary)
        answers["commands"]["commands"].append(new)
        answers[f"run {summary['run_id']}"] = report

    objs = [{"type": "system", "subtype": "init", "model": SONNET}]
    launches = []  # (clock, spec, step) of each launch
    for n, spec in enumerate(SPECS):
        if spec == skip_spec:
            continue
        parent = f"toolu_{n}"
        worktrees = [] if spec == short_prompt else [worktree(spec)]
        objs += child(parent, f"09:59:{n}0", spec_prompt(spec, worktrees), ended=m(5), background=background)
        launches.append((f"09:59:{n}0", None, None))
        for i, step in enumerate(LANE_STEPS):
            if ("implement", step) in delegation.REQUIRED and (spec, step) != skip_step:
                objs += child(f"{parent}_{step}", m(i, 30), f"Do the {step} step.", ended=m(i, 50), parent=parent)
                launches.append((m(i, 30), spec, step))
    if not background:
        objs.append(assistant(m(5, 30), {"type": "text", "text": "Both specs are implemented."}))
    objs += list(extra)
    objs.append({"type": "result", "subtype": "success", "num_turns": 1})

    clocks = [c for c, _, _ in launches] if marked == "all" else list(marked)
    for k, (clock, spec, step) in enumerate(launches):
        if clock in clocks:
            clocks.remove(clock)
            answers["spans"]["spans"].append(span_row(clock, f"s{k}", spec, step, end=m(5) if spec is None else None))

    path = tmp_path / "claude-code.txt"
    write_jsonl(path, objs)
    return read_with(path, tmp_path, answers)


# --- the epic verifier on the reference runs (criteria 1, 2 and 4) -----------------


REFERENCE_LANES = [("plan", "20260101000001-storage"), ("plan", "20260101000002-commands"), ("plan", "20260101000003-help")]


def reference(name):
    """A reference run's transcript, with its lanes given by hand: its
    Spektacular logs predate the format `stats` reads."""
    run = read_with(DATA / name / "claude-code.txt", DATA / name, None)
    run.lanes = dict.fromkeys(REFERENCE_LANES)
    return run


EPIC_TESTS = [
    "test_every_spec_has_its_own_child",
    "test_children_are_given_their_context",
    "test_no_wakeup_while_children_run",
    "test_children_run_as_the_probe_settled",
]
TRANSCRIPT_TESTS = EPIC_TESTS[1:]


@pytest.mark.parametrize("test", TRANSCRIPT_TESTS)
def test_the_46_epic_run_passes_every_transcript_child_check(test):
    assert verdict(getattr(epic_verifier.TestChildren(), test), reference("m70-epic")) == []


def test_the_55_epic_run_gives_every_spec_its_context():
    assert verdict(epic_verifier.TestChildren().test_children_are_given_their_context, reference("s55-epic")) == []


def test_a_reference_run_without_readable_logs_fails_the_span_checks_naming_why():
    lines = []
    try:
        epic_verifier.TestChildren().test_every_spec_has_its_own_child(read_with(DATA / "m70-epic" / "claude-code.txt", DATA / "m70-epic", None))
    except AssertionError as e:
        lines = [line.lstrip(" ~") for line in str(e).splitlines() if "logs_unavailable" in line]
    assert lines and all("cannot run spektacular" in line for line in lines)


def test_the_55_epic_run_fails_naming_its_scheduled_wakeup():
    assert verdict(epic_verifier.TestChildren().test_no_wakeup_while_children_run, reference("s55-epic")) == [
        "wakeup_while_children_run ScheduleWakeup delaySeconds=1200: set at 2026-10-23T13:36:38.091000+00:00 while a child was running"
    ]


def test_the_55_epic_run_fails_naming_its_background_children():
    storage = "child toolu_01LsCrL1DDkTZGc6kMPVwn5D (20260101000001-storage)"
    help_ = "child toolu_01Fcx3c9hqXdQYxakexacWST (20260101000003-help)"
    turn_end = "a turn ended at 2026-10-23T13:36:39.179000+00:00 while it was running"
    assert verdict(epic_verifier.TestChildren().test_children_run_as_the_probe_settled, reference("s55-epic")) == [
        f"child_in_background {storage}: it was started in the background",
        f"turn_ended_while_child_ran {storage}: {turn_end}",
        f"child_in_background {help_}: it was started in the background",
        f"turn_ended_while_child_ran {help_}: {turn_end}",
    ]


# --- the implement-epic verifier on a run built by hand ---------------------------

IMPLEMENT_TESTS = EPIC_TESTS + ["test_required_steps_each_started_a_child"]


@pytest.mark.parametrize("test", IMPLEMENT_TESTS)
def test_a_well_delegated_implement_epic_run_passes_every_child_check(tmp_path, test):
    run = implement_epic_run(tmp_path)
    assert sorted(run.lanes) == [("implement", A), ("implement", B)]
    assert verdict(getattr(implement_verifier.TestChildren(), test), run) == []


def test_a_step_with_no_child_fails_naming_the_spec_and_step(tmp_path):
    # criterion 3
    run = implement_epic_run(tmp_path, skip_step=(B, "check_goal"))
    assert verdict(implement_verifier.TestChildren().test_required_steps_each_started_a_child, run) == [
        f"missing_child implement:{B}:check_goal: the check_goal step completed without a child agent started in it"
    ]


@pytest.mark.parametrize("step", ["test", "verify"])
def test_a_test_or_verify_step_done_inline_passes(tmp_path, step):
    # Tests for a task that is not High complexity, and every verification,
    # are done in the agent's own context.
    run = implement_epic_run(tmp_path, skip_step=(A, step))
    assert verdict(implement_verifier.TestChildren().test_required_steps_each_started_a_child, run) == []


def test_a_child_not_given_its_worktree_fails_naming_the_child_and_the_folder(tmp_path):
    # criterion 2
    run = implement_epic_run(tmp_path, short_prompt=A)
    assert verdict(implement_verifier.TestChildren().test_children_are_given_their_context, run) == [
        f"context_gap child toolu_0 ({A}): its prompt does not give its worktree folder /app/.spektacular/worktrees/{A}"
    ]


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_a_spec_with_no_child_of_its_own_fails_naming_the_spec(tmp_path, verifier):
    # criterion 1
    run = implement_epic_run(tmp_path, skip_spec=B)
    assert verdict(VERIFIERS[verifier].TestChildren().test_every_spec_has_its_own_child, run) == [
        f"missing_spec_child implement:{B}: no child of the orchestrator was started for spec {B}"
    ]


@pytest.mark.parametrize(
    "call, subject",
    [
        (tool_use("w1", "ScheduleWakeup", {"delaySeconds": 60}), "ScheduleWakeup delaySeconds=60"),
        (tool_use("b1", "Bash", {"command": "sleep 60"}), "Bash sleep 60"),
    ],
    ids=["schedule-wakeup", "bash-sleep"],
)
@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_a_wakeup_while_children_run_fails_naming_the_call(tmp_path, verifier, call, subject):
    # criterion 4: set by the main agent at 10:01, while both spec children run
    run = implement_epic_run(tmp_path, extra=[assistant(m(1), call)])
    assert verdict(VERIFIERS[verifier].TestChildren().test_no_wakeup_while_children_run, run) == [
        f"wakeup_while_children_run {subject}: set at 2026-10-23T10:01:00+00:00 while a child was running"
    ]


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_background_spec_children_fail_naming_each_child(tmp_path, verifier):
    run = implement_epic_run(tmp_path, background=True)
    turn_end = "a turn ended at 2026-10-23T09:59:10+00:00 while it was running"
    assert verdict(VERIFIERS[verifier].TestChildren().test_children_run_as_the_probe_settled, run) == [
        f"child_in_background child toolu_0 ({A}): it was started in the background",
        f"turn_ended_while_child_ran child toolu_0 ({A}): {turn_end}",
        f"child_in_background child toolu_1 ({B}): it was started in the background",
        f"turn_ended_while_child_ran child toolu_1 ({B}): {turn_end}",
    ]


# --- both epic tasks still say the session is headless ------------------------------

HEADLESS = ("This session is headless: no one replies between your turns", "Never end your turn waiting for a reply")


@pytest.mark.parametrize("suite", ["epic-workflow", "implement-epic-workflow"])
@pytest.mark.parametrize("words", HEADLESS)
def test_the_epic_task_says_the_session_is_headless(suite, words):
    assert words in (HARBOR / suite / "instruction.md").read_text()


# --- other agents are checked as before (criterion 5) -----------------------------


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_the_child_checks_skip_for_other_agents(monkeypatch, verifier):
    monkeypatch.setattr(delegation, "is_claude", lambda: False)
    with pytest.raises(pytest.skip.Exception):
        VERIFIERS[verifier].children_run.__wrapped__()


@pytest.mark.parametrize("agent", ["codex", "bob"])
def test_codex_and_bob_runs_are_not_claude_runs(tmp_path, agent):
    build = tmp_path / "build.json"
    build.write_text(json.dumps({"agent": agent}))
    transcript = tmp_path / "claude-code.txt"
    transcript.write_text('{"type":"system"}\n')
    assert delegation.is_claude(build, transcript) is False


def reads_delegation(node) -> bool:
    for n in ast.walk(node):
        if isinstance(n, ast.Import) and any(a.name == "delegation" for a in n.names):
            return True
        if isinstance(n, ast.ImportFrom) and n.module == "delegation":
            return True
        if isinstance(n, ast.Name) and n.id in {"dl", "delegation"}:
            return True
    return False


def requests(fn, name) -> bool:
    return name in [a.arg for a in fn.args.args]


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_only_the_child_checks_read_the_transcript(verifier):
    tree = ast.parse(Path(VERIFIERS[verifier].__file__).read_text())
    defs = [n for n in tree.body if isinstance(n, (ast.FunctionDef, ast.ClassDef))]
    assert not any(reads_delegation(n) for n in tree.body if n not in defs)
    assert [n.name for n in defs if reads_delegation(n)] == ["children_run", "TestChildren"]
    # only TestChildren's tests use the fixture, so only they skip
    users = [
        f"{cls.name}.{fn.name}" if cls else fn.name
        for cls, fn in [(None, n) for n in defs if isinstance(n, ast.FunctionDef)]
        + [(c, f) for c in defs if isinstance(c, ast.ClassDef) for f in c.body if isinstance(f, ast.FunctionDef)]
        if requests(fn, "children_run")
    ]
    children = next(n for n in defs if n.name == "TestChildren")
    assert users == [f"TestChildren.{f.name}" for f in children.body if isinstance(f, ast.FunctionDef)]
    assert all(f.name.startswith("test_") for f in children.body if isinstance(f, ast.FunctionDef))


# --- every launch is a sub-agent span (spec 000074) -------------------------------


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_a_run_whose_every_launch_is_marked_passes(tmp_path, verifier):
    run = implement_epic_run(tmp_path)
    assert len(run.spans) == 4
    assert verdict(VERIFIERS[verifier].TestChildren().test_every_launch_is_a_sub_agent_span, run) == []


@pytest.mark.parametrize("verifier", sorted(VERIFIERS))
def test_an_unmarked_launch_fails_naming_it(tmp_path, verifier):
    # the two spec children and one of the two goal reviewers, launched at
    # the same moment, are marked
    run = implement_epic_run(tmp_path, marked=["09:59:00", "09:59:10", m(2, 30)])
    lines = verdict(VERIFIERS[verifier].TestChildren().test_every_launch_is_a_sub_agent_span, run)
    assert len(lines) == 1
    assert lines[0].startswith("unmarked_launch child toolu_")
    assert "_check_goal launched at 10:02:30 has no sub-agent span (`trace start <helper> --type sub-agent`)" in lines[0]

def test_a_run_with_no_spans_fails_every_launch(tmp_path):
    run = implement_epic_run(tmp_path, marked=[])
    assert run.spans == []
    assert len(verdict(implement_verifier.TestChildren().test_every_launch_is_a_sub_agent_span, run)) == 4
