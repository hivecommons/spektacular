"""Self-check for the rules in test_goal_check.py.

A verifier rule that can never return a violation passes every run and
proves nothing. So each rule is exercised here against a small synthetic run
that honours it and one that breaks it, and both directions are asserted.

This file never runs in the harbor container: it needs no /app and no /logs.
The Makefile copies only test_goal_check.py and the shared helpers into the
built task. Run it locally with

    uvx --with pytest pytest -p no:cacheprovider tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py
"""

import sys
from datetime import datetime, timedelta
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent.parent / "shared"))

import test_goal_check as gc  # noqa: E402
from runlog import Command  # noqa: E402
from transcripts import SubagentLaunch, claude_main_models, claude_subagent_launches  # noqa: E402

T0 = datetime(2026, 10, 21, 12, 0, 0)


def check(*verdicts, attempt=1, round_=1, tree=True, criteria=None):
    return {
        "round": round_,
        "attempt": attempt,
        "goal": {"criteria": criteria or gc.CRITERIA, "scope": {"repo": gc.REPO, "rule": "change only this item"}},
        "verdicts": [{"criterion": i + 1, "verdict": v, "evidence": "README.md:9"} for i, v in enumerate(verdicts)],
        "decision": "accept" if all(v == "met" for v in verdicts) else "retry",
        "tree_unchanged": tree,
    }


def item(*checks, state="accepted"):
    return {"item": gc.TASK_ID, "checks": list(checks), "attempts": len(checks), "state": state}


def event(seconds, *command, data=None):
    cmd = list(command)
    if data is not None:
        import json

        cmd += ["--data", json.dumps(data)]
    at = T0 + timedelta(seconds=seconds)
    return Command(
        run_id="r", kind="implement", name=gc.PLAN, command=" ".join(cmd[:3]), args=cmd,
        step=None, step_to=None, at=at, end=at + timedelta(milliseconds=10), exit_code=0,
    )


def goto(seconds, step):
    return event(seconds, "implement", "goto", data={"step": step, "name": gc.PLAN})


def record(seconds):
    return event(seconds, "implement", "goal", "record", data={"name": gc.PLAN, "item": gc.TASK_ID})


GOOD_UNMET = item(check("met", "met", "not_met"), check("met", "met", "met", attempt=2))


def test_n_verdicts():
    assert gc.rule_every_check_has_n_verdicts(GOOD_UNMET) == []
    missing = item(check("met", "met"))
    assert gc.rule_every_check_has_n_verdicts(missing)
    no_evidence = item(check("met", "met", "met"))
    no_evidence["checks"][0]["verdicts"][1]["evidence"] = " "
    assert gc.rule_every_check_has_n_verdicts(no_evidence)
    bad = item(check("met", "partly", "met"))
    assert gc.rule_every_check_has_n_verdicts(bad)
    assert gc.rule_every_check_has_n_verdicts(item())


def test_unmet_caught_then_retried():
    assert gc.rule_unmet_caught_then_retried(GOOD_UNMET) == []
    assert gc.rule_unmet_caught_then_retried(item(check("met", "met", "met")))
    assert gc.rule_unmet_caught_then_retried(item(check("met", "met", "not_met")))
    no_rise = item(check("met", "met", "not_met"), check("met", "met", "met", attempt=1))
    assert gc.rule_unmet_caught_then_retried(no_rise)


def test_met_on_first_attempt():
    assert gc.rule_met_on_first_attempt(item(check("met", "met", "met"))) == []
    assert gc.rule_met_on_first_attempt(GOOD_UNMET)
    assert gc.rule_met_on_first_attempt(item(check("met", "not_met", "met")))


def test_retry_goes_back_to_implement():
    good = [goto(0, "verify"), goto(1, "check_goal"), record(2), goto(3, "implement"), record(9)]
    assert gc.rule_retry_goes_back_to_implement(good) == []
    assert gc.rule_retry_goes_back_to_implement([record(2), goto(3, "update_plan"), record(9)])
    assert gc.rule_retry_goes_back_to_implement([goto(0, "verify")])


def test_nothing_ticked_before_the_check():
    good = [goto(0, "verify"), goto(1, "check_goal"), record(2), goto(3, "update_plan"),
            event(4, "plan", "file", "write", gc.PLAN, "plan", "--from", "x.md")]
    assert gc.rule_nothing_ticked_before_the_check(good) == []
    early_write = [event(1, "plan", "file", "write", gc.PLAN, "plan", "--from", "x.md"), record(2)]
    assert gc.rule_nothing_ticked_before_the_check(early_write)
    early_update = [goto(1, "update_plan"), record(2)]
    assert gc.rule_nothing_ticked_before_the_check(early_update)
    assert gc.rule_nothing_ticked_before_the_check([goto(1, "verify")])


def plan_with(heading, *ticks):
    box = lambda t: "x" if t else " "  # noqa: E731
    criteria = "\n".join(f"- [{box(t)}] {c}" for t, c in zip(ticks, gc.CRITERIA))
    return (
        "## Milestones & Tasks\n\n### Milestone 1: M\n\n"
        f"#### - [{box(heading)}] Task: Add the count command\n**Id:** {gc.TASK_ID}\n**Repo:** todo-app\n\n"
        f"**Acceptance criteria**:\n{criteria}\n\n## Open Questions\n"
    )


def test_ticks_only_met():
    assert gc.rule_ticks_only_met(GOOD_UNMET, plan_with(True, True, True, True)) == []
    # A criterion ticked though the last check did not judge it met.
    shortfall = item(check("met", "met", "not_met", attempt=3), state="accepted_shortfall")
    assert gc.rule_ticks_only_met(shortfall, plan_with(True, True, True, False)) == []
    assert gc.rule_ticks_only_met(shortfall, plan_with(True, True, True, True))
    # Ticked while the check is still retrying.
    retrying = item(check("met", "met", "not_met"), state="retrying")
    assert gc.rule_ticks_only_met(retrying, plan_with(True, True, True, False))
    # Judged met but left unticked.
    assert gc.rule_ticks_only_met(GOOD_UNMET, plan_with(True, True, False, True))


def test_goal_recorded():
    assert gc.rule_goal_recorded(GOOD_UNMET) == []
    assert gc.rule_goal_recorded(item(check("met", "met", "met", criteria=["other"])))


def test_tree_unchanged():
    assert gc.rule_tree_unchanged(GOOD_UNMET) == []
    assert gc.rule_tree_unchanged(item(check("met", "met", "met", tree=False)))


GOAL_PROMPT = (
    "GOAL 5d1e: Add the count command\n1. `python3 -m todo count` prints...\nEND GOAL\n"
    "verify: all green\ndiff: /app/.spektacular/tmp/20260301000000-todo-count/goal-check/todo-app.diff"
)


def test_reviewer_input():
    good = [SubagentLaunch("t1", gc.REVIEWER, GOAL_PROMPT)]
    assert gc.rule_reviewer_input(good) == []
    told = [SubagentLaunch("t1", gc.REVIEWER, GOAL_PROMPT + "\nThe implementation of this task is complete and all its acceptance criteria are met.")]
    assert gc.rule_reviewer_input(told)
    assert gc.rule_reviewer_input([SubagentLaunch("t1", "general-purpose", "look at the code")])
    no_paths = [SubagentLaunch("t1", gc.REVIEWER, "GOAL x\ntodo count\nEND GOAL")]
    assert gc.rule_reviewer_input(no_paths)


def transcript(reviewer_model="claude-haiku-4-5", main_model="claude-sonnet-4-6", returned=True):
    objs = [
        {"type": "assistant", "message": {"model": main_model, "content": [
            {"type": "tool_use", "id": "tu1", "name": "Agent", "input": {"subagent_type": gc.REVIEWER, "prompt": GOAL_PROMPT}},
        ]}},
        {"type": "assistant", "parent_tool_use_id": "tu1", "message": {"model": reviewer_model, "content": [{"type": "text", "text": "{}"}]}},
    ]
    if returned:
        objs.append({"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "tu1", "content": "{}"}]}})
    return objs


def test_cheaper_reviewer_from_a_claude_transcript():
    objs = transcript()
    launches = claude_subagent_launches(objs)
    assert gc.rule_cheaper_reviewer(launches, claude_main_models(objs)) == []
    assert gc.rule_reviewer_input(launches) == []

    same = transcript(reviewer_model="claude-sonnet-4-6")
    assert gc.rule_cheaper_reviewer(claude_subagent_launches(same), claude_main_models(same))
    lost = transcript(returned=False)
    assert gc.rule_cheaper_reviewer(claude_subagent_launches(lost), claude_main_models(lost))
    assert gc.rule_cheaper_reviewer([], {"claude-sonnet-4-6"})


# Each goal check, including the one its retry runs again, starts a child
# agent; the test and verify steps the retry runs in between need none. A
# small Claude run is built: the implement lane as `spektacular stats`
# answers for it (its step entries and the sub-agent spans started in them),
# and a stream-json transcript whose children are launched half a minute
# into a step, each marked by its span; it is read back with
# delegation.read_run exactly as the verifier reads the real one.

import json  # noqa: E402

import delegation  # noqa: E402

RETRY_STEPS = ["check_goal", "implement", "test", "verify", "check_goal", "update_plan"]
CHILD_AT = {0: "toolu_goal1", 2: "toolu_test", 3: "toolu_verify", 4: "toolu_goal2"}
RUN_ID = "12345678" + "0" * 24


def ts(minute, second=0):
    return f"2026-10-23T12:{minute:02d}:{second:02d}.000000000Z"


def stats_answers(without):
    entries = [
        {"step": step, "started_at": ts(i), "ended_at": ts(i + 1), "end_reason": "moved", "complete": True}
        for i, step in enumerate(RETRY_STEPS)
    ]
    spans = [
        {"span_id": tid, "parent_span_id": None, "name": tid, "type": "sub-agent", "trace_id": RUN_ID, "run_id": RUN_ID,
         "kind": "implement", "workflow": gc.PLAN, "step": RETRY_STEPS[minute], "task_id": None,
         "started_at": ts(minute, 25), "ended_at": ts(minute, 50), "end_reason": "stopped", "status": "ok", "complete": True}
        for minute, tid in CHILD_AT.items()
        if tid not in without
    ]
    new = {"run_id": RUN_ID, "kind": "implement", "name": gc.PLAN, "span_id": "n", "command": "implement new",
           "args": ["implement", "new", "--data", json.dumps({"name": gc.PLAN})], "step": None, "step_to": RETRY_STEPS[0],
           "started_at": ts(0), "ended_at": ts(0), "exit_code": 0, "message": None}
    return {
        "list": {"runs": [{"run_id": RUN_ID, "kind": "implement", "name": gc.PLAN, "started_at": ts(0), "ended_at": ts(6), "status": "in_progress"}]},
        "commands": {"commands": [new]},
        "spans": {"spans": spans},
        f"run {RUN_ID}": {"step_entries": entries},
    }


def launched(minute, tid):
    return [
        {
            "type": "assistant",
            "timestamp": f"2026-10-23T12:{minute:02d}:30Z",
            "parent_tool_use_id": None,
            "message": {"model": "claude-sonnet-4-6", "role": "assistant", "content": [
                {"type": "tool_use", "id": tid, "name": "Agent", "input": {"subagent_type": "general-purpose", "prompt": "Do the step.", "description": tid}},
            ]},
        },
        {"type": "system", "subtype": "task_started", "tool_use_id": tid, "task_id": f"task_{tid}",
         "task_type": "local_agent", "is_backgrounded": True, "spawn_depth": 1},
    ]


def claude_run(tmp_path, without=()):
    tmp_path.mkdir(parents=True, exist_ok=True)
    objs = [{"type": "system", "subtype": "init", "model": "claude-sonnet-4-6"}]
    for minute, tid in CHILD_AT.items():
        if tid not in without:
            objs += launched(minute, tid)
    objs.append({"type": "result", "subtype": "success", "num_turns": 1})
    path = tmp_path / "claude-code.txt"
    path.write_text("".join(json.dumps(o) + "\n" for o in objs))
    answers = stats_answers(without)

    def runner(argv, **kwargs):
        sub = argv[2]
        key = f"run {argv[argv.index('--run') + 1]}" if sub == "run" else sub
        return type("Done", (), {"stdout": json.dumps({"error": False, **answers[key]})})()

    real = delegation.runlog.RunLog
    delegation.runlog.RunLog = lambda log_dir: real(log_dir, binary="spektacular", runner=runner)
    try:
        return delegation.read_run(path, tmp_path)
    finally:
        delegation.runlog.RunLog = real


def test_required_children_with_a_child_in_each_required_step(tmp_path):
    run = claude_run(tmp_path)
    assert {s.span_id: s.step for s in run.spans} == {
        "toolu_goal1": "check_goal",
        "toolu_test": "test",
        "toolu_verify": "verify",
        "toolu_goal2": "check_goal",
    }
    assert gc.rule_required_children(run) == []
    assert [str(f) for f in delegation.unmarked_launches(run.children, run.spans)] == []


def test_required_children_test_and_verify_done_inline_on_the_retry(tmp_path):
    assert gc.rule_required_children(claude_run(tmp_path, without={"toolu_test", "toolu_verify"})) == []


def test_required_children_goal_check_done_inline(tmp_path):
    assert gc.rule_required_children(claude_run(tmp_path, without={"toolu_goal1"})) == [
        f"missing_child implement:{gc.PLAN}:check_goal: the check_goal step completed without a child agent started in it"
    ]
    # The second goal check, on the retry, is judged as its own attempt.
    assert gc.rule_required_children(claude_run(tmp_path / "second", without={"toolu_goal2"})) == [
        f"missing_child implement:{gc.PLAN}:check_goal (attempt 2): the check_goal step completed without a child agent started in it"
    ]


def test_codex_and_bob_runs_are_not_judged(tmp_path):
    # test_required_steps_each_started_a_child, and the spec suite's review
    # test, skip unless delegation.is_claude() holds.
    t = tmp_path / "claude-code.txt"
    t.write_text('{"type":"system"}\n')
    for agent in ("codex", "bob"):
        build = tmp_path / f"{agent}.json"
        build.write_text(json.dumps({"agent": agent}))
        assert delegation.is_claude(build, t) is False
    claude = tmp_path / "claude.json"
    claude.write_text(json.dumps({"agent": "claude"}))
    assert delegation.is_claude(claude, t) is True
    assert delegation.is_claude(claude, tmp_path / "missing.txt") is False
