"""Self-check for runlog.py, Harbor's only route to Spektacular's logs.

The reader is fed through its injectable runner with canned `spektacular
stats` answers: the Go golden files under internal/runstats/testdata, so a
field renamed in Go breaks these tests too, and the hand-built epic trial in
runlog_fixtures.py. No binary, container or wall clock is needed.
test_runlog_integration.py runs the same fixture through a real binary.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_runlog.py
"""

import json
import subprocess
import sys
from datetime import timedelta
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import runlog  # noqa: E402
import runlog_fixtures as fx  # noqa: E402
from runlog import Command, RunLog, Span, Unavailable  # noqa: E402

ROOT = HERE.parents[3]
GOLDEN = ROOT / "internal" / "runstats" / "testdata"
SOURCE = {"error": False, "log_dir": "/logs", "skipped_lines": 0}


class Done:
    def __init__(self, stdout: str):
        self.stdout = stdout


class FakeRunner:
    """Answers `stats <sub>` from canned JSON, and records every call."""

    def __init__(self, answers: dict[str, dict]):
        self.answers = answers
        self.calls: list[list[str]] = []

    def __call__(self, argv, **kwargs):
        self.calls.append(argv)
        assert argv[1] == "stats"
        assert argv[-4:-2] == ["--dir", argv[-3]] and argv[-2:] == ["--format", "json"]
        assert kwargs["timeout"] == runlog.TIMEOUT_S
        sub = argv[2]
        if sub == "run":
            run_id = argv[argv.index("--run") + 1]
            return Done(json.dumps(self.answers[f"run {run_id}"]))
        return Done(json.dumps(self.answers[sub]))


def reader(tmp_path, answers, **kw) -> RunLog:
    return RunLog(tmp_path, binary="spektacular", runner=FakeRunner(answers), **kw)


def golden(name: str) -> dict:
    return json.loads((GOLDEN / name).read_text())


@pytest.fixture
def epic(tmp_path) -> RunLog:
    return reader(tmp_path, fx.stats())


def at(minutes):
    return fx.at(minutes)


# Typed parsing, from the Go golden files.


def test_commands_parse_the_golden_listing(tmp_path):
    log = reader(tmp_path, {"commands": {**SOURCE, **golden("commands.golden.json")}})
    cmds = log.commands()
    assert [c.command for c in cmds] == ["epic order", "plan new", "implement new", "implement new", "plan goto", "plan file read", "epic merge"]
    order = cmds[0]
    assert order.run_id is None and order.kind is None and order.name is None
    assert order.args == ["epic", "order", "--data", '{"name":"e"}']
    assert order.data() == {"name": "e"}
    assert order.is_verb("epic", "order") and order.ok
    assert order.end - order.at == timedelta(seconds=30)
    goto = cmds[4]
    assert (goto.run_id, goto.kind, goto.name) == ("bbbbbbbb000000000000000000000002", "plan", "000001_alpha")
    assert (goto.step, goto.step_to, goto.goto_step, goto.entered) == ("discovery", "finished", "finished", "finished")
    refused = cmds[2]
    assert not refused.ok and refused.exit_code == 1
    assert refused.message == "dependencies are not met"
    assert log.outside() == [cmds[0], cmds[-1]]


def test_spans_parse_the_golden_listing(tmp_path):
    log = reader(tmp_path, {"spans": {**SOURCE, **golden("spans.golden.json")}})
    spans = log.spans()
    assert [s.span_id for s in spans] == ["oh1", "oh2", "ph", "pu", "pw", "ih"]
    oh1, oh2, ph = spans[0], spans[1], spans[2]
    assert (oh1.type, oh1.run_id, oh1.spec, oh1.step, oh1.complete) == ("sub-agent", None, None, None, True)
    assert oh1.trace_id == "0000000b000000000000000000000011"
    assert (oh2.end_reason, oh2.complete, oh2.end) == ("open", False, oh2.start)
    assert (ph.run_id, ph.kind, ph.spec, ph.step, ph.parent) == ("bbbbbbbb000000000000000000000002", "plan", "000001_alpha", "discovery", "d1")
    assert [s.type for s in spans[3:5]] == ["user-wait", "work"]
    assert log.orchestrator_spans() == [oh1, oh2]


def test_report_is_the_golden_run_report(tmp_path):
    rep = {**SOURCE, **golden("report.golden.json")}
    run_id = rep["run"]["run_id"]
    log = reader(tmp_path, {f"run {run_id}": rep})
    got = log.report(run_id)
    assert got["steps"][0]["step"] == rep["steps"][0]["step"]
    assert got["totals"]["helper_wait_ms"] == 180000
    assert log.runner.calls == [["spektacular", "stats", "run", "--run", run_id, "--dir", str(tmp_path), "--format", "json"]]


def test_each_question_is_asked_once(epic):
    epic.commands(), epic.commands(), epic.spans(), epic.lanes(), epic.lanes(), epic.runs()
    subs = [argv[2] for argv in epic.runner.calls]
    assert sorted(subs) == ["commands", "list", "spans"]


def test_parse_time_trims_nanoseconds():
    assert runlog.parse_time("2026-10-25T10:00:01.123456789Z") == at(0) + timedelta(seconds=1, microseconds=123456)
    assert runlog.parse_time("2026-10-25T10:00:00Z") == at(0)


# Every failure is a stated reason, never an empty answer.


def test_no_binary_is_unavailable(tmp_path, monkeypatch):
    monkeypatch.delenv("SPEKTACULAR_BIN", raising=False)
    monkeypatch.setattr(runlog.shutil, "which", lambda name: None)
    with pytest.raises(Unavailable, match="SPEKTACULAR_BIN"):
        RunLog(tmp_path).commands()


def test_the_binary_comes_from_spektacular_bin(tmp_path, monkeypatch):
    monkeypatch.setenv("SPEKTACULAR_BIN", "/opt/spek")
    runner = FakeRunner(fx.stats())
    RunLog(tmp_path, runner=runner).commands()
    assert runner.calls[0][0] == "/opt/spek"


def test_no_logs_directory_is_unavailable(tmp_path):
    with pytest.raises(Unavailable, match="no Spektacular logs at"):
        reader(tmp_path / "missing", fx.stats()).runs()


def test_a_refusal_is_unavailable_with_its_code(tmp_path):
    refusal = {"error": True, "code": "stats_log_dir_not_found", "message": "the logs directory does not exist"}
    with pytest.raises(Unavailable, match="stats list: stats_log_dir_not_found: the logs directory does not exist"):
        reader(tmp_path, {"list": refusal}).runs()


def test_a_timeout_is_unavailable(tmp_path):
    def runner(argv, **kw):
        raise subprocess.TimeoutExpired(argv, kw["timeout"])

    with pytest.raises(Unavailable, match="took longer than 60s"):
        RunLog(tmp_path, binary="spektacular", runner=runner).spans()


def test_a_binary_that_cannot_run_is_unavailable(tmp_path):
    def runner(argv, **kw):
        raise FileNotFoundError(2, "No such file or directory")

    with pytest.raises(Unavailable, match="cannot run spektacular"):
        RunLog(tmp_path, binary="spektacular", runner=runner).spans()


@pytest.mark.parametrize("stdout", ["", "Usage: spektacular stats [command]", "[1, 2]"])
def test_unreadable_output_is_unavailable(tmp_path, stdout):
    log = RunLog(tmp_path, binary="spektacular", runner=lambda argv, **kw: Done(stdout))
    with pytest.raises(Unavailable, match="stats commands"):
        log.commands()


def test_the_reader_runs_stats_outside_any_project(tmp_path):
    seen = {}

    def runner(argv, **kw):
        seen.update(kw)
        return Done(json.dumps(fx.stats()["list"]))

    RunLog(tmp_path, binary="spektacular", runner=runner).runs()
    assert seen["capture_output"] and seen["text"]
    assert Path(seen["cwd"]).is_dir()


# Lanes, from the epic trial.


def test_lanes_start_finish_and_children(epic):
    lanes = epic.lanes()
    assert list(lanes) == [("plan", fx.ALPHA), ("plan", fx.BETA)]
    alpha, beta = lanes[("plan", fx.ALPHA)], lanes[("plan", fx.BETA)]
    assert [r["run_id"] for r in alpha.runs] == [fx.TRACE_A]
    assert alpha.start.at == at(2) and alpha.finish.at == at(15)
    assert alpha.last is alpha.finish
    assert alpha.resumes == [] and alpha.children == 1 and alpha.stops == []
    assert beta.start.at == at(2.5) and beta.finish.at == at(16)
    assert [c.at for c in beta.resumes] == [at(11)]
    assert beta.children == 2
    assert beta.stops == ["architecture"], "the refused `plan new` carries the step the run stood at"
    assert [c.at for c in beta.child_starts] == [at(2.5), at(11)]


def test_lanes_of_other_kinds_are_left_out(epic):
    assert epic.lanes(("implement",)) == {}


def test_a_restart_is_a_new_child_stopped_at_restart(tmp_path):
    st = fx.stats()
    later = dict(st["commands"]["commands"][1], started_at=fx.stamp(at(30)), ended_at=fx.stamp(at(30.1)), span_id="r1", run_id="ffffffff000000000000000000000001")
    st["commands"]["commands"].append(later)
    lane = reader(tmp_path, st).lanes()[("plan", fx.ALPHA)]
    assert lane.children == 2 and lane.stops == ["restart"]
    assert lane.resumes == []


def test_a_resume_with_no_step_stops_at_a_question_mark(tmp_path):
    st = fx.stats()
    for c in st["commands"]["commands"]:
        if c["exit_code"] and c["command"] == "plan new":
            c["step"] = None
    assert reader(tmp_path, st).lanes()[("plan", fx.BETA)].stops == ["?"]


def test_a_lane_never_started_has_no_children(tmp_path):
    st = fx.stats()
    st["commands"]["commands"] = [c for c in st["commands"]["commands"] if not (c["name"] == fx.ALPHA and c["command"] == "plan new")]
    lane = reader(tmp_path, st).lanes()[("plan", fx.ALPHA)]
    assert lane.start is None and lane.children == 0 and lane.child_starts == []


# The orchestrator span each child was launched under.


def test_each_child_takes_the_span_open_when_it_began(epic):
    lanes = epic.lanes()
    alpha, beta = lanes[("plan", fx.ALPHA)], lanes[("plan", fx.BETA)]
    assert epic.child_span(alpha.start).span_id == "oa"
    # Launched together: beta's first command came after alpha's, so beta
    # takes the next span, though alpha's is still open.
    assert epic.child_span(beta.start).span_id == "ob1"
    resumed = epic.child_span(beta.child_starts[1])
    assert resumed.span_id == "ob2"
    assert not resumed.complete, "a span never stopped counts as open from its start on"


def test_a_child_with_no_open_span_has_none(tmp_path):
    st = fx.stats()
    st["spans"]["spans"] = [s for s in st["spans"]["spans"] if s["span_id"] != "ob2"]
    log = reader(tmp_path, st)
    beta = log.lanes()[("plan", fx.BETA)]
    assert log.child_span(beta.child_starts[1]) is None, "oa is open, but already matched to alpha"


def test_spans_inside_runs_are_not_the_orchestrators(epic):
    assert [s.span_id for s in epic.orchestrator_spans()] == ["oa", "ob1", "ob2"]
    assert "ha1" in [s.span_id for s in epic.spans()]


# The orchestrator's next act after a lane.


def test_reaction_skips_commands_naming_only_the_finished_spec(epic):
    alpha = epic.lanes()[("plan", fx.ALPHA)]
    r = epic.orchestrator_reaction(alpha)
    assert isinstance(r, Command) and r.is_verb("trace", "stop") and r.at == at(20)


def test_reaction_takes_a_command_naming_another_spec(epic):
    beta = epic.lanes()[("plan", fx.BETA)]
    r = epic.orchestrator_reaction(beta)
    assert isinstance(r, Command) and r.args[:4] == ["plan", "file", "read", fx.ALPHA]


def test_reaction_takes_an_orchestrator_span_when_it_comes_first(tmp_path):
    st = fx.stats()
    st["commands"]["commands"] = [c for c in st["commands"]["commands"] if c["run_id"] is not None]
    log = reader(tmp_path, st)
    alpha = log.lanes()[("plan", fx.ALPHA)]
    r = log.orchestrator_reaction(alpha)
    assert isinstance(r, Span) and r.span_id == "oa"
    assert runlog.event_at(r, alpha.last.end) == at(20), "the span's stop"


def test_event_at_is_a_span_start_when_it_began_after(epic):
    span = epic.orchestrator_spans()[2]
    assert runlog.event_at(span, at(10)) == at(10.5)


# Merges, steps entered and changelog writes.


def test_merge_is_the_successful_epic_merge_of_the_spec(epic):
    assert epic.merge(fx.ALPHA).at == at(22)
    assert epic.merge(fx.BETA) is None


def test_steps_entered_in_the_order_they_ran(epic):
    assert epic.steps_entered("plan") == ["discovery", "discovery", "architecture", "architecture", "finished", "finished"]
    assert epic.steps_entered("implement") == []


def plan_log(tmp_path, rows) -> RunLog:
    return reader(tmp_path, {"commands": {**SOURCE, "commands": rows}})


def row(minute, args, *, step_to=None, exit_code=0):
    return {
        "run_id": fx.TRACE_A, "kind": "plan", "name": fx.ALPHA, "span_id": f"s{minute}", "command": fx.command_name(args),
        "args": args, "step": None, "step_to": step_to, "started_at": fx.stamp(at(minute)),
        "ended_at": fx.stamp(at(minute)), "exit_code": exit_code, "message": None,
    }


def test_failed_gotos_and_file_commands_are_not_steps_entered(tmp_path):
    log = plan_log(tmp_path, [
        row(0, fx.new(fx.ALPHA), step_to="overview"),
        row(1, fx.goto("finished", fx.ALPHA), exit_code=1),
        row(2, ["plan", "file", "write", fx.ALPHA, "plan", "--from", "p.md"]),
        row(3, ["knowledge", "search", "auth"]),
        row(4, fx.goto("discovery", fx.ALPHA), step_to="discovery"),
    ])
    assert log.steps_entered("plan") == ["overview", "discovery"]


def test_a_goto_that_moves_nowhere_falls_back_to_its_data_step(tmp_path):
    # A goto to the current step records no step_to; it still entered it.
    log = plan_log(tmp_path, [row(0, fx.new(fx.ALPHA), step_to="overview"), row(1, fx.goto("overview", fx.ALPHA))])
    assert log.steps_entered("plan") == ["overview", "overview"]


def test_steps_entered_follow_the_time_they_ran_not_the_list_order(tmp_path):
    rows = [row(i, fx.goto(s, fx.ALPHA), step_to=s) for i, s in enumerate(["overview", "discovery", "architecture"])]
    assert plan_log(tmp_path, list(reversed(rows))).steps_entered("plan") == ["overview", "discovery", "architecture"]


def test_changelog_writes_by_repo(epic):
    assert epic.changelog_writes_by_repo(fx.ALPHA) == {None: 1, "docs": 1, "web": 1}
    assert epic.changelog_writes_by_repo(fx.BETA) == {}


def write(minute, *args, exit_code=0):
    return row(minute, ["changelog", "file", "write", *args], exit_code=exit_code)


def test_a_failed_write_is_not_counted(tmp_path):
    log = plan_log(tmp_path, [write(0, fx.ALPHA, "--from", "a"), write(1, fx.ALPHA, "--repo", "docs", "--from", "b", exit_code=1)])
    assert log.changelog_writes_by_repo(fx.ALPHA) == {None: 1}


@pytest.mark.parametrize("args", [
    [fx.ALPHA, "--repo", "docs", "--from", "b"],
    ["--repo", "docs", fx.ALPHA, "--from", "b"],
    [fx.ALPHA, "--repo=docs", "--from", "b"],
    ["--from", "b", "--repo=docs", fx.ALPHA],
])
def test_a_write_is_read_however_its_flags_are_spelled(tmp_path, args):
    assert plan_log(tmp_path, [write(0, *args)]).changelog_writes_by_repo(fx.ALPHA) == {"docs": 1}


def test_a_write_of_another_record_is_left_out_when_a_name_is_given(tmp_path):
    log = plan_log(tmp_path, [write(0, fx.ALPHA, "--from", "a"), write(1, fx.BETA, "--from", "b")])
    assert log.changelog_writes_by_repo(fx.ALPHA) == {None: 1}
    assert log.changelog_writes_by_repo() == {None: 2}


def test_a_help_request_is_not_a_write(tmp_path):
    log = plan_log(tmp_path, [write(0, fx.ALPHA, "--help"), write(1, "-h"), write(2, "--repo", "docs", "-h")])
    assert log.changelog_writes_by_repo() == {}


def test_repeated_writes_of_a_record_each_count(tmp_path):
    log = plan_log(tmp_path, [write(i, fx.ALPHA, "--repo", "docs", "--from", "a") for i in range(3)])
    assert log.changelog_writes_by_repo(fx.ALPHA) == {"docs": 3}


def test_names_reads_arguments_and_data(epic):
    cmds = epic.commands()
    merge = next(c for c in cmds if c.is_verb("epic", "merge"))
    read = next(c for c in cmds if c.run_id is None and c.is_verb("plan", "file", "read"))
    assert epic.names(merge) == {fx.ALPHA}
    assert epic.names(read) == {fx.ALPHA}
    assert epic.names(cmds[0]) == set()


def test_the_reader_never_opens_a_log_file_itself():
    source = (HERE.parent / "runlog.py").read_text()
    for forbidden in ("open(", "read_text", "read_bytes", "glob(", "iterdir", ".jsonl\""):
        assert forbidden not in source, forbidden


# The suite verifiers read the logs through the reader: the plan suite's
# walkthrough check and the implement suite's changelog-write checks, run
# against readers built from canned `stats` answers.

HARBOR = HERE.parents[1]
for _suite in ("plan-workflow", "implement-workflow", "epic-workflow", "implement-epic-workflow", "standing-rules", "goal-check-workflow", "spec-workflow"):
    sys.path.insert(0, str(HARBOR / _suite / "tests"))

import test_implement_workflow as implement_verifier  # noqa: E402
import test_plan_workflow as plan_verifier  # noqa: E402

PLAN_STEPS = ["overview", "discovery", "architecture", "write_plan", "write_research", "walkthrough", "finished"]


def canned_module_reader(monkeypatch, module, rows):
    """Points a verifier module's RunLog at canned `stats commands` rows."""
    answers = {"commands": {**SOURCE, "commands": rows}, "list": {**SOURCE, "runs": []}, "spans": {**SOURCE, "spans": []}}
    real = runlog.RunLog
    monkeypatch.setattr(module, "RunLog", lambda d: real(HERE, binary="spektacular", runner=FakeRunner(answers)))


def plan_walk(steps):
    rows = [row(0, fx.new(fx.ALPHA), step_to=steps[0])]
    rows += [row(i, fx.goto(s, fx.ALPHA), step_to=s) for i, s in enumerate(steps[1:], start=1)]
    return rows


def test_plan_verifier_passes_a_walkthrough_before_finished(monkeypatch):
    canned_module_reader(monkeypatch, plan_verifier, plan_walk(PLAN_STEPS))
    plan_verifier.TestInstructionNextStepValidity().test_walkthrough_rendered_before_finished()


@pytest.mark.parametrize(
    "steps",
    [PLAN_STEPS[:5] + ["finished"], PLAN_STEPS[:5] + ["finished", "walkthrough"], PLAN_STEPS[:6]],
    ids=["skips-walkthrough", "finished-first", "never-finished"],
)
def test_plan_verifier_fails_a_lane_without_walkthrough_before_finished(monkeypatch, steps):
    canned_module_reader(monkeypatch, plan_verifier, plan_walk(steps))
    with pytest.raises(AssertionError):
        plan_verifier.TestInstructionNextStepValidity().test_walkthrough_rendered_before_finished()


def test_plan_verifier_fails_when_no_plan_command_was_logged(monkeypatch):
    canned_module_reader(monkeypatch, plan_verifier, [])
    with pytest.raises(AssertionError):
        plan_verifier.TestInstructionNextStepValidity().test_walkthrough_rendered_before_finished()


CHANGELOG_CHECKS = {
    "project": "test_project_level_changelog_was_committed_via_cli",
    "auth": "test_colocated_repo_changelog_was_committed_via_cli",
    "docs": "test_external_repo_changelog_was_committed_via_cli",
}


def changelog_results(monkeypatch, rows) -> dict[str, bool]:
    canned_module_reader(monkeypatch, implement_verifier, rows)
    results = {}
    for record_, check in CHANGELOG_CHECKS.items():
        try:
            getattr(implement_verifier, check)()
        except AssertionError:
            results[record_] = False
        else:
            results[record_] = True
    return results


def implement_writes(skip="-", failed="-"):
    plan = implement_verifier.PLAN_NAME
    rows = []
    for i, repo in enumerate((None, "auth", "docs")):
        if repo == skip:
            continue
        args = ["changelog", "file", "write", plan, "--from", f"{repo}.md"] + (["--repo", repo] if repo else [])
        rows.append(row(i, args, exit_code=1 if repo == failed else 0))
    return rows


def test_implement_verifier_passes_a_run_that_wrote_every_record(monkeypatch):
    assert changelog_results(monkeypatch, implement_writes(skip="-")) == {"project": True, "auth": True, "docs": True}


@pytest.mark.parametrize("how", ["removed", "failed"])
@pytest.mark.parametrize("repo,record_", [("docs", "docs"), ("auth", "auth"), (None, "project")])
def test_implement_verifier_fails_only_the_record_never_written(monkeypatch, how, repo, record_):
    rows = implement_writes(skip=repo) if how == "removed" else implement_writes(skip="-", failed=repo)
    expected = {name: name != record_ for name in CHANGELOG_CHECKS}
    assert changelog_results(monkeypatch, rows) == expected


def test_implement_verifier_fails_every_record_when_nothing_was_logged(monkeypatch):
    assert changelog_results(monkeypatch, []) == {"project": False, "auth": False, "docs": False}


def test_no_verifier_reads_the_old_session_log():
    for path in sorted(p for p in HARBOR.glob("*/tests/*.py") if p.parts[-3] != "shared"):
        text = path.read_text()
        assert "session" + "log" not in text and "Session" + "Log" not in text, path
        assert ".spektacular/debug" not in text and '"debug"' not in text, path
        assert "from " + "spans import" not in text and "import " + "spans" not in text, path


def test_no_verifier_reads_a_status_command_from_the_logs():
    # `status` is never logged (spec 000073), so no check may look for it.
    for path in sorted(p for p in HARBOR.glob("*/tests/test_*.py") if p.parts[-3] != "shared"):
        text = path.read_text()
        assert 'is_verb("status"' not in text, path


# The epic suites' orchestration checks over readers built from canned
# `stats` answers: lanes, the one child per spec (a resume is a refused
# `<kind> new` in the lane), ordering, merges and the goal record before
# each plan update, read from the step each command ran in.

import test_epic_workflow as epic_verifier  # noqa: E402
import test_implement_epic_workflow as implement_epic_verifier  # noqa: E402


def lane_rows(kind, name, start, steps, *, resume_at=None, extra=()):
    """A lane's commands: an orchestrated `<kind> new` at `start` minutes,
    a goto to each step a minute apart, and, at `resume_at`, a refused
    `<kind> new` filed in the lane carrying the step it stood at."""
    run_id = f"{abs(hash((kind, name))) % 16**8:08x}" + "0" * 24

    def cmd(minute, args, *, step=None, step_to=None, exit_code=0):
        return {
            "run_id": run_id, "kind": kind, "name": name, "span_id": f"{name}{minute}", "command": fx.command_name(args),
            "args": args, "step": step, "step_to": step_to, "started_at": fx.stamp(at(minute)),
            "ended_at": fx.stamp(at(minute + 0.1)), "exit_code": exit_code, "message": None,
        }

    data = json.dumps({"name": name, "orchestrated": True})
    rows = [cmd(start, [kind, "new", "--data", data], step_to=steps[0])]
    for i, (before, after) in enumerate(zip(steps, steps[1:]), start=1):
        rows.append(cmd(start + i, [kind, "goto", "--data", json.dumps({"step": after, "name": name})], step=before, step_to=after))
    if resume_at is not None:
        rows.append(cmd(resume_at, [kind, "new", "--data", data], step=steps[1], exit_code=1))
    rows += [cmd(start + m_, args, step=step) for m_, args, step in extra]
    return run_id, rows


def epic_answers(lanes, outside=()):
    runs, rows = [], []
    for lane in lanes:
        kind, name, start, steps, opts = lane
        run_id, lane_rows_ = lane_rows(kind, name, start, steps, **opts)
        runs.append({"run_id": run_id, "kind": kind, "name": name, "started_at": fx.stamp(at(start)), "ended_at": fx.stamp(at(start + 9)), "status": "finished"})
        rows += lane_rows_
    rows += [
        {"run_id": None, "kind": None, "name": None, "span_id": f"o{minute}", "command": fx.command_name(args), "args": args,
         "step": None, "step_to": None, "started_at": fx.stamp(at(minute)), "ended_at": fx.stamp(at(minute + 0.5)),
         "exit_code": code, "message": None}
        for minute, args, code in outside
    ]
    return {"list": {**SOURCE, "runs": runs}, "commands": {**SOURCE, "commands": rows}, "spans": {**SOURCE, "spans": []}}


def verdicts(cls, log, *names_and_args) -> dict[str, bool]:
    out = {}
    for name, *args in names_and_args:
        try:
            getattr(cls(), name)(log, *args)
        except AssertionError:
            out[(name, *args)] = False
        else:
            out[(name, *args)] = True
    return out


PLAN = ["overview", "discovery", "architecture", "finished"]


def plan_epic(tmp_path, *, resume=None, commands_start=6):
    v = epic_verifier
    lanes = [
        ("plan", v.STORAGE, 0, PLAN, {}),
        ("plan", v.HELP, 0.5, PLAN, {"resume_at": 2.5} if resume == v.HELP else {}),
        ("plan", v.COMMANDS, commands_start, PLAN, {}),
    ]
    return RunLog(tmp_path, binary="spektacular", runner=FakeRunner(epic_answers(lanes, [(-1, ["epic", "order", "e"], 0)])))


def test_epic_verifier_passes_a_well_orchestrated_trial(tmp_path):
    log = plan_epic(tmp_path)
    cls = epic_verifier.TestOrchestration
    checks = [("test_session_log_recorded",), ("test_commands_planned_after_storage",), ("test_epic_order_run", "e")]
    for spec in epic_verifier.SPECS:
        checks += [("test_spec_planned_in_orchestrated_lane", spec), ("test_lane_reached_finished", spec), ("test_one_child_per_spec", spec)]
    assert all(verdicts(cls, log, *checks).values())


def test_epic_verifier_fails_a_resumed_lane_and_an_early_start(tmp_path):
    v = epic_verifier
    log = plan_epic(tmp_path, resume=v.HELP, commands_start=1)
    got = verdicts(v.TestOrchestration, log, ("test_one_child_per_spec", v.HELP), ("test_one_child_per_spec", v.STORAGE), ("test_commands_planned_after_storage",))
    assert got == {("test_one_child_per_spec", v.HELP): False, ("test_one_child_per_spec", v.STORAGE): True, ("test_commands_planned_after_storage",): False}
    assert log.lane("plan", v.HELP).stops == ["discovery"]


def test_epic_verifier_finds_no_logs(tmp_path):
    log = RunLog(tmp_path / "missing", binary="spektacular", runner=FakeRunner({}))
    assert verdicts(epic_verifier.TestOrchestration, log, ("test_session_log_recorded",)) == {("test_session_log_recorded",): False}


IMPLEMENT = ["read_plan", "analyze", "implement", "test", "verify", "check_goal", "update_plan", "update_changelog", "finished"]


def implement_epic(tmp_path, *, record=True, export_start=30):
    v = implement_epic_verifier  # the implement-epic suite's verifier module
    goal = [(5.5, ["implement", "goal", "record", "--data", "{}"], "check_goal")] if record else []
    lanes = [
        ("implement", v.STORAGE, 0, IMPLEMENT, {"extra": goal}),
        ("implement", v.FORMAT, 0.5, IMPLEMENT, {"extra": goal}),
        ("implement", v.COMMANDS, 15, IMPLEMENT, {"extra": goal}),
        ("implement", v.EXPORT, export_start, IMPLEMENT, {"extra": goal}),
    ]
    outside = [(-2 + i * 0.1, ["epic", "worktree", "--data", json.dumps({"spec": s})], 0) for i, s in enumerate(v.SPECS)]
    outside += [(12, ["epic", "merge", "--data", json.dumps({"spec": v.STORAGE})], 0), (13, ["epic", "merge", "--data", json.dumps({"spec": v.FORMAT})], 0)]
    outside += [(27, ["epic", "merge", "--data", json.dumps({"spec": v.COMMANDS})], 0), (42, ["epic", "merge", "--data", json.dumps({"spec": v.EXPORT})], 0)]
    return RunLog(tmp_path, binary="spektacular", runner=FakeRunner(epic_answers(lanes, outside)))


def test_implement_epic_verifier_passes_a_well_orchestrated_trial(tmp_path):
    v = implement_epic_verifier  # the implement-epic suite's verifier module
    log = implement_epic(tmp_path)
    checks = [("test_session_log_recorded",), ("test_storage_and_format_ran_in_parallel",), ("test_commands_started_after_both_merged",),
              ("test_export_started_after_commands_merged",), ("test_no_merge_refused",)]
    for spec in v.SPECS:
        checks += [("test_worktree_created", spec), ("test_spec_in_orchestrated_lane", spec), ("test_lane_reached_finished", spec),
                   ("test_one_child_per_spec", spec), ("test_spec_merged", spec)]
    got = verdicts(v.TestOrchestration, log, *checks)
    assert all(got.values()), [k for k, ok in got.items() if not ok]
    goal = verdicts(v.TestGoalCheck, log, *[("test_goal_record_precedes_update_plan", s) for s in v.SPECS])
    assert all(goal.values())


def test_implement_epic_verifier_fails_an_update_plan_with_no_goal_record_before_it(tmp_path):
    v = implement_epic_verifier  # the implement-epic suite's verifier module
    log = implement_epic(tmp_path, record=False, export_start=20)
    assert verdicts(v.TestGoalCheck, log, ("test_goal_record_precedes_update_plan", v.STORAGE)) == {("test_goal_record_precedes_update_plan", v.STORAGE): False}
    assert verdicts(v.TestOrchestration, log, ("test_export_started_after_commands_merged",)) == {("test_export_started_after_commands_merged",): False}


def test_no_verifier_self_check_reads_the_old_session_log():
    for path in [HERE / "test_runlog.py", HERE / "test_epic_children.py", HERE / "test_delegation.py", *HARBOR.glob("*/tests/test_verifier_selfcheck.py")]:
        text = path.read_text()
        old = "session" + "log"  # spelt apart, so this file does not match itself
        assert f"import {old}" not in text and f"from {old}" not in text, path
        assert "import " + "spans" not in text and "from " + "spans" not in text, path


# Nothing a verifier reads from Spektacular's logs needs the verbose level:
# no verifier asks for a recorded response or state snapshot, and every
# verifier that reads the logs reads them through the run log reader.

VERBOSE_ONLY = ("state_" + "before", "state_" + "after", "spektacular." + "response", "spektacular." + "state", ".response")


def test_no_verifier_reads_a_verbose_only_log_fact():
    for path in sorted(p for p in HARBOR.glob("*/tests/test_*.py") if p.parts[-3] != "shared"):
        text = path.read_text()
        for attr in VERBOSE_ONLY:
            assert attr not in text, (path, attr)


def test_every_verifier_reading_the_logs_reads_them_through_the_reader():
    readers = {}
    for path in sorted(p for p in HARBOR.glob("*/tests/test_*.py") if p.parts[-3] != "shared"):
        text = path.read_text()
        if ".spektacular\" / \"logs\"" in text or '/ "logs"' in text:
            readers[path.parts[-3]] = "RunLog" in text or "read_run" in text
    assert readers and all(readers.values()), readers
