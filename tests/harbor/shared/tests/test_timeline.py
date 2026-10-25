"""Self-check for timeline.py, the epic run timeline.

The timeline is printed from Spektacular's logs through the run log reader,
here fed with the canned `stats` answers of runlog_fixtures.py's epic trial:
two plan lanes launched together, beta's first child cut off in
architecture and resumed by a second child, and the orchestrator's `epic`
and `trace` commands and one `sub-agent` span per child, the last never
stopped. The expected rows are worked out by hand from the fixture's times.

This file never runs in a harbor container. Run it locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_timeline.py
"""

import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import runlog_fixtures as fx  # noqa: E402
import timeline  # noqa: E402
from runlog import RunLog  # noqa: E402


def answers(drop_spans=(), drop_commands=()):
    st = fx.stats()
    st["spans"]["spans"] = [s for s in st["spans"]["spans"] if s["span_id"] not in drop_spans]
    st["commands"]["commands"] = [c for c in st["commands"]["commands"] if c["command"] not in drop_commands]
    for run_id, rep in fx.reports().items():
        st[f"run {run_id}"] = {"error": False, **rep}
    return st


def reader(tmp_path, st) -> RunLog:
    def runner(argv, **kw):
        key = f"run {argv[argv.index('--run') + 1]}" if argv[2] == "run" else argv[2]
        return type("Done", (), {"stdout": json.dumps(st[key])})()

    return RunLog(tmp_path, binary="spektacular", runner=runner)


def row(text: str, lane: str) -> list[str]:
    """The cells of a table row, split on runs of two or more spaces."""
    line = next(l for l in text.splitlines() if l.strip().startswith(f"plan {lane}"))
    return re.split(r"\s{2,}", line.strip())


def hand_offs(text: str) -> str:
    return text.split("Hand-offs between the orchestrator and each child:")[1].split("Resumed children")[0]


def lanes_table(text: str) -> str:
    return text.split("Lanes, one per child workflow")[1].split("Hand-offs")[0]


def test_each_lane_shows_its_start_finish_and_slowest_step(tmp_path):
    out = timeline.report(reader(tmp_path, answers()))
    assert "Run: 10:00:00 to 10:22:30 (22m30s), 18 spektacular calls, 3 orchestrator helper spans" in out
    # lane, start, finish, took, steps, children, slowest step
    assert row(lanes_table(out), fx.ALPHA) == [f"plan {fx.ALPHA}", "+2m00s", "+15m06s", "13m06s", "2", "1", "discovery 7m54s"]
    assert row(lanes_table(out), fx.BETA) == [f"plan {fx.BETA}", "+2m30s", "+16m06s", "13m36s", "2", "2", "architecture 6m54s"]


def test_the_hand_off_gaps_come_from_the_orchestrators_spans(tmp_path):
    out = hand_offs(timeline.report(reader(tmp_path, answers())))
    # alpha: its span oa ran 10:01 to 10:20, stopped on the hand-back; its
    # first command 10:02, its last ended 10:15:06, merged at 10:22.
    assert row(out, fx.ALPHA) == [f"plan {fx.ALPHA}", "1m00s", "4m54s", "6m54s", "hand-back"]
    # beta: its first child's span ob1 started 10:01:30, its first command
    # 10:02:30. Its last child's span ob2 was never stopped, so the gap runs
    # from its last command (ended 10:16:06) to the orchestrator's next
    # command naming another spec (10:17). It was never merged.
    assert row(out, fx.BETA) == [f"plan {fx.BETA}", "1m00s", "0m54s", "-", f"plan file read {fx.ALPHA} plan"]


def test_a_child_with_no_orchestrator_span_shows_no_span(tmp_path):
    out = hand_offs(timeline.report(reader(tmp_path, answers(drop_spans={"oa", "ob1", "ob2"}))))
    assert row(out, fx.ALPHA)[:3] == [f"plan {fx.ALPHA}", "no span", "no span"]
    assert row(out, fx.BETA)[:3] == [f"plan {fx.BETA}", "no span", "no span"]


def test_a_never_stopped_span_with_no_reaction_says_so(tmp_path):
    st = answers()
    st["commands"]["commands"] = [c for c in st["commands"]["commands"] if c["run_id"] is not None or c["started_at"] < fx.stamp(fx.at(16))]
    st["spans"]["spans"] = [s for s in st["spans"]["spans"] if s["span_id"] != "oa"]
    out = hand_offs(timeline.report(reader(tmp_path, st)))
    # beta's last child ran under ob2, never stopped, and the orchestrator
    # recorded no command and no span start or stop after beta's last command
    assert row(out, fx.BETA)[2] == "no reaction"


def test_a_resumed_lane_shows_its_extra_child_and_where_the_first_stopped(tmp_path):
    out = timeline.report(reader(tmp_path, answers()))
    resumed = out.split("Resumed children")[1]
    assert f"plan {fx.BETA}" in resumed and "1 resume(s); stopped at: architecture" in resumed
    assert f"plan {fx.ALPHA}" not in resumed


def test_missing_logs_are_reported_and_the_script_exits_successfully(tmp_path, capsys, monkeypatch):
    out = timeline.report(RunLog(tmp_path / "missing", binary="spektacular"))
    assert out == f"== Epic run timeline ==\nNo Spektacular logs under {tmp_path / 'missing'}: no Spektacular logs at {tmp_path / 'missing'}\n"
    monkeypatch.setenv("SPEKTACULAR_BIN", str(tmp_path / "no-such-binary"))
    assert timeline.main(["timeline.py", str(tmp_path), str(tmp_path / "no-bob")]) == 0
    printed = capsys.readouterr().out
    assert printed.startswith("== Epic run timeline ==\nNo Spektacular logs under ")
    assert "cannot run" in printed


def test_bob_turn_limit_hits_are_counted(tmp_path):
    bob = tmp_path / "bob"
    bob.mkdir()
    (bob / "a.log").write_text(f"{timeline.BOB_TURN_LIMIT}\nx\n{timeline.BOB_TURN_LIMIT}\n")
    assert timeline.bob_turn_limit_hits(bob) == 2
    assert timeline.bob_turn_limit_hits(tmp_path / "none") is None
    out = timeline.report(reader(tmp_path, answers()), bob)
    assert f'Bob tasks ended at the turn limit ("{timeline.BOB_TURN_LIMIT}" in its logs): 2' in out


def test_the_timeline_reads_the_logs_only_through_the_reader():
    source = (HERE.parent / "timeline.py").read_text()
    assert "import runlog" in source or "from runlog import" in source
    assert "session" + "log" not in source
    assert ".jsonl" not in source and "read_text()" not in source.replace("path.read_text(errors=", "")


def as_implement(st):
    """The same trial with both lanes implement lanes: an implement-epic
    trial, whose orchestrator merges each spec it finishes."""
    for c in st["commands"]["commands"]:
        if c["kind"] == "plan":
            c["kind"] = "implement"
        if c["args"][:1] == ["plan"] and c["args"][1] in ("new", "goto"):
            c["args"] = ["implement", *c["args"][1:]]
            c["command"] = "implement " + c["args"][1]
    for r in st["list"]["runs"]:
        r["kind"] = "implement"
    for s in st["spans"]["spans"]:
        if s["kind"] == "plan":
            s["kind"] = "implement"
    return st


def test_an_implement_epic_trial_with_no_debug_directory(tmp_path):
    assert not (tmp_path / "debug").exists()
    out = timeline.report(reader(tmp_path, as_implement(answers())))
    lanes = out.split("Lanes, one per child workflow")[1].split("Hand-offs")[0]
    alpha = next(l for l in lanes.splitlines() if fx.ALPHA in l)
    assert re.split(r"\s{2,}", alpha.strip()) == [f"implement {fx.ALPHA}", "+2m00s", "+15m06s", "13m06s", "2", "1", "discovery 7m54s"]
    gaps = hand_offs(out)
    alpha = next(l for l in gaps.splitlines() if fx.ALPHA in l and "implement" in l)
    assert re.split(r"\s{2,}", alpha.strip()) == [f"implement {fx.ALPHA}", "1m00s", "4m54s", "6m54s", "hand-back"]
