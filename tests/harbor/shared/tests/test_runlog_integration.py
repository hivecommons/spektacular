"""Runs the run log reader against a real `spektacular` binary.

A throwaway binary is built from the working tree into a temp directory
(never installed, never `go run`), and the reader reads the epic trial of
runlog_fixtures.py, written as JSON Lines in Spektacular's own record
format. Its answers must equal the canned answers test_runlog.py uses for
the same trial, so the canned `stats` JSON cannot drift from the real one,
and a copy of the logs directory must give the same answers. The metrics
recorder is run over the same logs, and over a spec trial with a helper and a
user wait, through the same binary: its steps must equal `stats run`'s, each
split adding up to its duration, and its log sections must equal what the
canned answers give. Skipped when Go is not installed.

This file never runs in a harbor container. Run it locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_runlog_integration.py
"""

import json
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))

import metrics  # noqa: E402
import runlog_fixtures as fx  # noqa: E402
from runlog import RunLog  # noqa: E402

ROOT = HERE.parents[3]

pytestmark = pytest.mark.skipif(shutil.which("go") is None, reason="needs Go to build a spektacular binary")


@pytest.fixture(scope="module")
def binary(tmp_path_factory) -> str:
    out = tmp_path_factory.mktemp("bin") / "spektacular"
    subprocess.run(["go", "build", "-o", str(out), "."], cwd=ROOT, check=True, capture_output=True, timeout=600)
    return str(out)


@pytest.fixture(scope="module")
def logs(tmp_path_factory) -> Path:
    return fx.write(tmp_path_factory.mktemp("trial") / "logs")


@pytest.fixture(scope="module")
def spec_logs(tmp_path_factory) -> Path:
    return fx.write_spec(tmp_path_factory.mktemp("spec-trial") / "logs")


class Canned:
    def __init__(self):
        self.answers = fx.stats()

    def __call__(self, argv, **kw):
        return type("Done", (), {"stdout": json.dumps(self.answers[argv[2]])})()


def views(log: RunLog) -> dict:
    """Everything the harness asks of a reader, as plain values."""
    lanes = log.lanes()
    return {
        "commands": log.commands(),
        "spans": log.spans(),
        "runs": [(r["run_id"], r["kind"], r["name"], r["started_at"][:19]) for r in log.runs()],
        "lanes": {
            key: (l.start, l.finish, l.last, l.resumes, l.children, l.child_starts, l.stops)
            for key, l in lanes.items()
        },
        "child_spans": {key: [log.child_span(c) for c in l.child_starts] for key, l in lanes.items()},
        "reactions": {key: log.orchestrator_reaction(l) for key, l in lanes.items()},
        "merges": {name: log.merge(name) for _, name in lanes},
        "steps": log.steps_entered("plan"),
        "writes": log.changelog_writes_by_repo(fx.ALPHA),
    }


def test_the_binary_gives_the_canned_answers(binary, logs):
    real = views(RunLog(logs, binary=binary))
    canned = views(RunLog(logs, binary="spektacular", runner=Canned()))
    for key in canned:
        assert real[key] == canned[key], key
    assert real["lanes"][("plan", fx.BETA)][-1] == ["architecture"]
    assert [s.span_id for s in real["child_spans"][("plan", fx.BETA)]] == ["ob1", "ob2"]


def test_a_copied_logs_directory_gives_the_same_answers(binary, logs, tmp_path):
    copy = tmp_path / "collected" / "logs"
    shutil.copytree(logs, copy)
    assert views(RunLog(copy, binary=binary)) == views(RunLog(logs, binary=binary))


def test_the_binary_reads_a_run_report(binary, logs):
    log = RunLog(logs, binary=binary)
    rep = log.report(fx.TRACE_A)
    assert rep["run"]["name"] == fx.ALPHA
    assert [s["step"] for s in rep["steps"]] == ["discovery", "architecture", "finished"]
    assert rep["helpers"][0]["name"] == "research"


# The metrics recorder over the same logs, through the same binary.

NONE = Path("/nonexistent")
REPORT_STEP_FIELDS = ("step", "entries", "duration_ms", "commands", "helper_wait_ms", "user_wait_ms", "incomplete")
REPORT_TOTAL_FIELDS = ("wall_ms", "user_wait_ms", "helper_wait_ms", "gap_ms", "gaps_over_threshold")


def read_fields(rep: dict) -> dict:
    """The fields of a `stats run` report the metrics recorder reads."""
    return {
        "gap_threshold_ms": rep["gap_threshold_ms"],
        "totals": {k: rep["totals"][k] for k in REPORT_TOTAL_FIELDS},
        "steps": [{k: r[k] for k in REPORT_STEP_FIELDS} for r in rep["steps"]],
        "step_entries": [{"step": e["step"], "started_at": e["started_at"]} for e in rep["step_entries"]],
        "tasks": rep["tasks"],
        "helpers": [{"span_id": h["span_id"], "step": h["step"]} for h in rep["helpers"]],
    }


class CannedWithReports:
    """Canned answers for every `stats` subcommand, `run` by its --run id."""

    def __init__(self, answers: dict, reports: dict):
        self.answers, self.reports = answers, reports

    def __call__(self, argv, **kw):
        sub = argv[2]
        out = self.reports[argv[argv.index("--run") + 1]] if sub == "run" else self.answers[sub]
        return type("Done", (), {"stdout": json.dumps(out)})()


def record_with(log_dir: Path, monkeypatch, make_log) -> dict:
    """The record of an oracle trial over log_dir, every RunLog built by make_log."""
    monkeypatch.setattr(metrics.runlog, "RunLog", make_log)
    return metrics.build_record(
        metrics.Paths(build_info=NONE, agent_dir=NONE, log_dir=log_dir, bob_logs=NONE, junit=NONE, reward=NONE)
    )


def log_sections(record: dict) -> dict:
    sub = record["subagents"]
    return {
        "steps": record["steps"],
        "subagents": {k: sub[k] for k in ("started", "completed", "failed", "unfinished", "durations_s", "helpers")},
        "totals": {k: record["totals"][k] for k in
                   ("trace_calls", "trace_calls_per_task", "user_wait_s", "helper_wait_s", "gap_s", "gaps", "gap_threshold_s")},
    }  # fmt: skip


def test_the_binary_gives_the_canned_run_reports(binary, logs, spec_logs):
    assert read_fields(RunLog(spec_logs, binary=binary).report(fx.TRACE_S)) == fx.spec_report()
    log = RunLog(logs, binary=binary)
    for run_id, canned in fx.reports().items():
        assert read_fields(log.report(run_id)) == canned, run_id


def test_the_records_steps_equal_the_binarys_run_report(binary, spec_logs, monkeypatch):
    monkeypatch.setenv("SPEKTACULAR_BIN", binary)
    record = metrics.build_record(
        metrics.Paths(build_info=NONE, agent_dir=NONE, log_dir=spec_logs, bob_logs=NONE, junit=NONE, reward=NONE)
    )
    log = RunLog(spec_logs, binary=binary)
    (workflow,) = record["steps"]["workflows"]
    rep = log.report(workflow["run_id"])
    assert [s["step"] for s in workflow["steps"]] == [r["step"] for r in rep["steps"]]
    for step, row in zip(workflow["steps"], rep["steps"]):
        assert round(step["duration_s"] * 1000) == row["duration_ms"], step["step"]
        assert round(step["helper_wait_s"] * 1000) == row["helper_wait_ms"], step["step"]
        assert round(step["user_wait_s"] * 1000) == row["user_wait_ms"], step["step"]
        assert step["working_s"] + step["helper_wait_s"] + step["user_wait_s"] == step["duration_s"], step["step"]
    # the trial waits on the user in overview and on a helper in discovery
    by_step = {s["step"]: s for s in workflow["steps"]}
    assert by_step["overview"]["user_wait_s"] > 0 and by_step["discovery"]["helper_wait_s"] > 0
    assert by_step["discovery"]["helpers"] == 1
    assert [(h["name"], h["spec"], h["step"]) for h in record["subagents"]["helpers"]] == [("research", fx.GAMMA, "discovery")]
    assert record["totals"]["user_wait_s"] * 1000 == rep["totals"]["user_wait_ms"]
    assert record["totals"]["helper_wait_s"] * 1000 == rep["totals"]["helper_wait_ms"]
    assert not [u for u in record["unknown"] if not u["field"].startswith(("totals.", "run.", "result.", "subagents.briefs"))]


@pytest.mark.parametrize("trial", ["spec", "epic"])
def test_the_binary_and_the_canned_answers_give_the_same_record(binary, logs, spec_logs, monkeypatch, trial):
    log_dir, answers, reports = {
        "spec": (spec_logs, fx.spec_stats(), {fx.TRACE_S: fx.spec_report()}),
        "epic": (logs, fx.stats(), fx.reports()),
    }[trial]
    real_runlog = metrics.runlog.RunLog
    real = record_with(log_dir, monkeypatch, lambda d, **kw: real_runlog(d, binary=binary))
    canned = record_with(log_dir, monkeypatch, lambda d, **kw: real_runlog(d, binary="spektacular", runner=CannedWithReports(answers, reports)))
    assert real["steps"]["workflows"] and real["subagents"]["helpers"]
    assert log_sections(real) == log_sections(canned)
