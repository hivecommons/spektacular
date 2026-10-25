"""The run log reader gives the same answers as the old session-log rules.

sessionlog.py, the debug-log reader the run log reader replaces, is read
here one last time, over the same epic trial recorded at the verbose level
whose responses and state snapshots the old rules needed. This file goes
with sessionlog.py when spec 000076 removes it. No suite verifier or
verifier self-check imports the old reader.

This file never runs in a harbor container. Run it locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_runlog_vs_sessionlog.py
"""

import json
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import runlog_fixtures as fx  # noqa: E402
from runlog import RunLog  # noqa: E402


class Done:
    def __init__(self, stdout: str):
        self.stdout = stdout


@pytest.fixture
def epic(tmp_path) -> RunLog:
    answers = fx.stats()
    return RunLog(tmp_path, binary="spektacular", runner=lambda argv, **kw: Done(json.dumps(answers[argv[2]])))


# The same answers as the old session-log rules (sessionlog.py, read until
# spec 000076 removes it), over the same trial recorded at the verbose level,
# whose responses and state snapshots the old rules needed.


def verbose_copy(log_dir: Path) -> Path:
    log_dir.mkdir(parents=True, exist_ok=True)
    for name, recs in fx.records().items():
        lines = []
        for r in recs:
            a = r["attributes"]
            if r["record"] == "span" and "spektacular.workflow.kind" in a:
                if a.get("spektacular.step"):
                    a["spektacular.state.before"] = {"current_step": a["spektacular.step"]}
                a["spektacular.state.after"] = {"current_step": a.get("spektacular.step.to") or a.get("spektacular.step")}
                if r["status"]["code"] == "error" and a["process.command_args"][1] == "new":
                    a["spektacular.response"] = '{"error":true,"code":"workflow_in_progress"}'
            lines.append(json.dumps(r) + "\n")
        (log_dir / name).write_text("".join(lines))
    return log_dir


def test_the_reader_agrees_with_the_old_session_log_rules(epic, tmp_path):
    import sessionlog

    old = sessionlog.SessionLog(verbose_copy(tmp_path / "verbose"))
    for key, lane in epic.lanes().items():
        was = old.lane(*key)
        assert lane.start.at == was.start.at and lane.finish.at == was.finish.at
        assert lane.children == was.children
        assert lane.stops == was.stops
        assert [c.at for c in lane.resumes] == [e.at for e in was.resumes]
        assert epic.merge(key[1]) is None if old.merge(key[1]) is None else epic.merge(key[1]).at == old.merge(key[1]).at
    events = old.all_events()
    assert epic.steps_entered("plan") == sessionlog.plan_steps_entered(events)
    for name in (fx.ALPHA, fx.BETA, None):
        assert epic.changelog_writes_by_repo(name) == sessionlog.changelog_writes_by_repo(events, name)
