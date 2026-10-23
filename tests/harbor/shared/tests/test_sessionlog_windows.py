"""Self-check for the step windows sessionlog.py reads out of a lane.

`Lane.step_windows()` turns one orchestrated lane's command log into the
steps the workflow stood at, each with a start and an end. A successful
command that moves the workflow onto another step closes the open window and
opens the next at the same instant; a failed command moves nothing; the last
window of each lane file ends with that file's last command. A new lane file,
or a child resuming the lane through a refused `<kind> new`, always opens a
fresh window, so the hand-off between children is never counted. Each case
here is a small hand-built lane with hand-worked expected windows, and the
lane readers the epic timeline and the verifiers already use (`stops`,
`finish`, `starts`, `children`) are pinned on the same lanes so the new code
cannot have changed them.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider tests/harbor/shared/tests/test_sessionlog_windows.py
"""

import json
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from sessionlog import Event, Lane, SessionLog, StepWindow  # noqa: E402

NAME = "20260101000000-user-auth"
RESUME_RESPONSE = (
    '{"error":"workflow_in_progress","message":"plan 20260101000000-user-auth '
    'is already in progress at step tasks; resuming"}'
)


def ts(clock: str) -> str:
    """A fixed synthetic timestamp on the test day, as the CLI writes it (ns)."""
    return f"2026-10-23T{clock}.000000000Z"


def at(hour: int, minute: int, second: int, micro: int = 0) -> datetime:
    return datetime(2026, 10, 23, hour, minute, second, micro, tzinfo=timezone.utc)


def state(step):
    return {"kind": "plan", "name": NAME, "current_step": step}


def goto(step):
    return ["plan", "goto", "--data", json.dumps({"step": step, "name": NAME})]


NEW = ["plan", "new", "--data", json.dumps({"name": NAME})]
FILE_READ = ["plan", "file", "read", "--data", json.dumps({"name": NAME, "file": "plan.md"})]
FILE_WRITE = ["plan", "file", "write", "--data", json.dumps({"name": NAME, "file": "plan.md"})]


def record(clock, command, before, after, *, exit_code=0, duration_ms=0, response=None):
    rec = {
        "timestamp": ts(clock),
        "command": command,
        "duration_ms": duration_ms,
        "exit_code": exit_code,
        "state_before": before,
        "state_after": after,
    }
    if response is not None:
        rec["response"] = response
    return rec


def write_lane(debug_dir: Path, started: str, records, *, session=f"plan_{NAME}"):
    """Write one debug-log file named as the CLI names it; started is HHMMSS."""
    path = debug_dir / f"20261023T{started}.903970323Z_claude_{session}.jsonl"
    path.write_text("".join(json.dumps(r) + "\n" for r in records))
    return path


def event(hour, minute, second, command, before, after, *, exit_code=0, duration_ms=0, response=""):
    return Event(
        at=at(hour, minute, second),
        command=command,
        duration_ms=duration_ms,
        exit_code=exit_code,
        state_before=before,
        state_after=after,
        response=response,
    )


def shape(windows):
    return [(w.step, w.duration_s) for w in windows]


def span(lane: Lane) -> float:
    first = min(e.at for e in lane.events)
    last = max(e.end for e in lane.events)
    return (last - first).total_seconds()


# --- 1. a straight run through every step --------------------------------------


def straight_run(tmp_path) -> Lane:
    write_lane(
        tmp_path,
        "100000",
        [
            record("10:00:00", NEW, None, state("overview")),
            # non-step commands while at overview: one reports the step, one does not
            record("10:00:05", FILE_READ, state("overview"), state("overview"), duration_ms=800),
            record("10:00:20", ["plan", "status", NAME], None, None),
            # a nonzero duration on a goto does not move the window edge
            record("10:01:00", goto("discovery"), state("overview"), state("discovery"), duration_ms=1500),
            record("10:02:00", FILE_WRITE, state("discovery"), state("discovery")),
            record("10:03:00", goto("architecture"), state("discovery"), state("architecture")),
            record("10:06:00", goto("assemble"), state("architecture"), state("assemble")),
            record("10:07:30", goto("finished"), state("assemble"), state("finished")),
            # a trailing command after finished, still running for 2.5 s
            record("10:07:40", FILE_READ, state("finished"), state("finished"), duration_ms=2500),
        ],
    )
    # the shared log is not a lane
    write_lane(
        tmp_path,
        "095959",
        [record("10:00:30", ["status", "my-epic"], None, None)],
        session="no-active-workflow",
    )
    return SessionLog(tmp_path).lane("plan", NAME)


def test_straight_run_lists_every_step_in_order(tmp_path):
    lane = straight_run(tmp_path)
    windows = lane.step_windows()
    assert shape(windows) == [
        ("overview", 60.0),
        ("discovery", 120.0),
        ("architecture", 180.0),
        ("assemble", 90.0),
        ("finished", 12.5),
    ]
    assert [(w.start, w.end) for w in windows] == [
        (at(10, 0, 0), at(10, 1, 0)),
        (at(10, 1, 0), at(10, 3, 0)),
        (at(10, 3, 0), at(10, 6, 0)),
        (at(10, 6, 0), at(10, 7, 30)),
        (at(10, 7, 30), at(10, 7, 42, 500000)),
    ]
    # each window opens where the previous one closed
    assert all(a.end == b.start for a, b in zip(windows, windows[1:]))


def test_straight_run_durations_sum_to_the_lane_span(tmp_path):
    lane = straight_run(tmp_path)
    assert span(lane) == 462.5
    assert sum(w.duration_s for w in lane.step_windows()) == 462.5


def test_straight_run_lane_readers_unchanged(tmp_path):
    lane = straight_run(tmp_path)
    log = SessionLog(tmp_path)
    assert list(log.lanes) == [("plan", NAME)]
    assert lane.files == 1
    assert lane.stops == []
    assert [e.at for e in lane.starts] == [at(10, 0, 0)]
    assert lane.start.at == at(10, 0, 0)
    assert lane.finish.at == at(10, 7, 30)
    assert lane.children == 1
    assert lane.resumes == []


# --- 2a. re-entering an earlier step -------------------------------------------


def test_reentered_step_is_two_windows(tmp_path):
    write_lane(
        tmp_path,
        "100000",
        [
            record("10:00:00", NEW, None, state("overview")),
            record("10:01:00", goto("discovery"), state("overview"), state("discovery")),
            record("10:03:00", goto("architecture"), state("discovery"), state("architecture")),
            record("10:04:00", goto("discovery"), state("architecture"), state("discovery")),
            record("10:04:30", goto("architecture"), state("discovery"), state("architecture")),
            record("10:06:00", goto("finished"), state("architecture"), state("finished")),
        ],
    )
    lane = SessionLog(tmp_path).lane("plan", NAME)
    assert shape(lane.step_windows()) == [
        ("overview", 60.0),
        ("discovery", 120.0),
        ("architecture", 60.0),
        ("discovery", 30.0),
        ("architecture", 90.0),
        ("finished", 0.0),
    ]
    assert sum(w.duration_s for w in lane.step_windows()) == span(lane) == 360.0


# --- 2b. a lane resumed in a second file ---------------------------------------


def two_files(tmp_path) -> SessionLog:
    write_lane(
        tmp_path,
        "100000",
        [
            record("10:00:00", NEW, None, state("overview")),
            record("10:01:00", goto("discovery"), state("overview"), state("discovery")),
            record("10:02:00", FILE_READ, state("discovery"), state("discovery"), duration_ms=3000),
        ],
    )
    write_lane(
        tmp_path,
        "110000",
        [
            record("11:00:00", NEW, None, state("overview")),
            record("11:02:00", goto("discovery"), state("overview"), state("discovery")),
            record("11:05:00", goto("finished"), state("discovery"), state("finished")),
            record("11:05:30", FILE_READ, state("finished"), state("finished")),
        ],
    )
    return SessionLog(tmp_path)


def test_two_files_give_windows_per_file(tmp_path):
    lane = two_files(tmp_path).lane("plan", NAME)
    windows = lane.step_windows()
    assert shape(windows) == [
        ("overview", 60.0),
        ("discovery", 63.0),
        ("overview", 120.0),
        ("discovery", 180.0),
        ("finished", 30.0),
    ]
    # the first file's last window ends with its last command, not at the next file
    assert windows[1].end == at(10, 2, 3)
    assert windows[2].start == at(11, 0, 0)
    # the hour between the files is left out
    gap = (at(11, 0, 0) - at(10, 2, 3)).total_seconds()
    assert span(lane) == 3930.0
    assert sum(w.duration_s for w in windows) == span(lane) - gap == 453.0


def test_two_files_lane_readers_unchanged(tmp_path):
    lane = two_files(tmp_path).lane("plan", NAME)
    assert lane.files == 2
    assert len(lane.segments) == 2
    assert lane.stops == ["restart"]
    assert [e.at for e in lane.starts] == [at(10, 0, 0), at(11, 0, 0)]
    assert lane.resumes == []
    assert lane.children == 2
    assert lane.finish.at == at(11, 5, 0)


# --- 2c. a lane resumed within one file ----------------------------------------


def resumed_in_file(tmp_path) -> Lane:
    write_lane(
        tmp_path,
        "100000",
        [
            record("10:00:00", NEW, None, state("overview")),
            record("10:01:00", goto("discovery"), state("overview"), state("discovery")),
            record("10:03:00", goto("tasks"), state("discovery"), state("tasks")),
            record("10:04:00", FILE_READ, state("tasks"), state("tasks"), duration_ms=2000),
            # the next child resumes: `plan new` is refused with a resume report
            record(
                "10:30:00",
                NEW,
                state("tasks"),
                None,
                exit_code=1,
                response=RESUME_RESPONSE,
            ),
            record("10:31:00", FILE_WRITE, state("tasks"), state("tasks")),
            record("10:35:00", goto("finished"), state("tasks"), state("finished")),
            record("10:35:20", FILE_READ, state("finished"), state("finished")),
        ],
    )
    return SessionLog(tmp_path).lane("plan", NAME)


def test_resume_in_file_opens_a_fresh_window(tmp_path):
    lane = resumed_in_file(tmp_path)
    windows = lane.step_windows()
    assert shape(windows) == [
        ("overview", 60.0),
        ("discovery", 120.0),
        ("tasks", 62.0),
        ("tasks", 300.0),
        ("finished", 20.0),
    ]
    # the window before the resume closes at the previous command's end
    assert windows[2].end == at(10, 4, 2)
    # the fresh window opens at the refused `plan new`, from its state_before
    assert windows[3].start == at(10, 30, 0)
    gap = (at(10, 30, 0) - at(10, 4, 2)).total_seconds()
    assert span(lane) == 2120.0
    assert sum(w.duration_s for w in windows) == span(lane) - gap == 562.0


def test_resume_in_file_takes_step_from_state_after_when_present():
    events = [
        event(10, 0, 0, NEW, None, state("overview")),
        event(10, 1, 0, goto("tasks"), state("overview"), state("tasks")),
        event(10, 5, 0, FILE_READ, state("tasks"), state("tasks"), duration_ms=1000),
        # no state_before here, so the step can only come from state_after
        event(10, 20, 0, NEW, None, state("tasks"), exit_code=1, response=RESUME_RESPONSE),
        event(10, 22, 0, goto("finished"), state("tasks"), state("finished")),
    ]
    lane = Lane(kind="plan", name=NAME, files=1, events=list(events), segments=[list(events)])
    assert lane.step_windows() == [
        StepWindow("overview", at(10, 0, 0), at(10, 1, 0)),
        StepWindow("tasks", at(10, 1, 0), at(10, 5, 1)),
        StepWindow("tasks", at(10, 20, 0), at(10, 22, 0)),
        StepWindow("finished", at(10, 22, 0), at(10, 22, 0)),
    ]


def test_resume_in_file_lane_readers_unchanged(tmp_path):
    lane = resumed_in_file(tmp_path)
    assert lane.files == 1
    assert [e.at for e in lane.resumes] == [at(10, 30, 0)]
    assert lane.stops == ["tasks"]
    assert [e.at for e in lane.starts] == [at(10, 0, 0)]
    assert lane.children == 2
    assert lane.finish.at == at(10, 35, 0)


# --- 3. failed commands never move the step ------------------------------------


def test_failed_goto_does_not_move_the_step():
    events = [
        event(10, 0, 0, NEW, None, state("overview")),
        event(10, 1, 0, goto("discovery"), state("overview"), state("discovery")),
        # refused, state unchanged
        event(10, 2, 0, goto("architecture"), state("discovery"), state("discovery"), exit_code=1),
        # refused, no state_after: the --data step must not be taken as the step
        event(10, 3, 0, goto("architecture"), state("discovery"), None, exit_code=1),
        # a failed goto to finished is not the lane's finish either
        event(10, 3, 30, goto("finished"), state("discovery"), None, exit_code=1),
        event(10, 5, 0, goto("architecture"), state("discovery"), state("architecture")),
        event(10, 6, 0, goto("finished"), state("architecture"), state("finished")),
    ]
    # out of time order within the segment, as a lane built by hand may be
    shuffled = [events[i] for i in (3, 0, 6, 1, 5, 2, 4)]
    lane = Lane(kind="plan", name=NAME, files=1, events=list(events), segments=[shuffled])
    assert shape(lane.step_windows()) == [
        ("overview", 60.0),
        ("discovery", 240.0),
        ("architecture", 60.0),
        ("finished", 0.0),
    ]
    assert lane.finish is events[6]
    assert lane.stops == []
    assert lane.children == 1


# --- state_after null falls back to the goto's --data step ---------------------


def test_null_state_after_falls_back_to_goto_data(tmp_path):
    write_lane(
        tmp_path,
        "100000",
        [
            record("10:00:00", NEW, None, state("overview")),
            record("10:01:00", goto("discovery"), state("overview"), None),
            # a non-goto with no state_after names no step and is ignored
            record("10:02:00", FILE_READ, None, None),
            record("10:04:00", goto("finished"), None, None),
        ],
    )
    lane = SessionLog(tmp_path).lane("plan", NAME)
    assert lane.events[1].state_after is None
    assert lane.events[1].step_after == "discovery"
    assert lane.events[2].step_after is None
    assert shape(lane.step_windows()) == [
        ("overview", 60.0),
        ("discovery", 180.0),
        ("finished", 0.0),
    ]
    assert lane.finish.at == at(10, 4, 0)


def test_event_end_adds_duration(tmp_path):
    write_lane(
        tmp_path,
        "100000",
        [{**record("10:00:00", NEW, None, state("overview"), duration_ms=1250),
          "timestamp": "2026-10-23T10:00:00.500000123Z"}],
    )
    e = SessionLog(tmp_path).lane("plan", NAME).events[0]
    assert e.at == at(10, 0, 0, 500000)
    assert e.end == at(10, 0, 1, 750000)
    windows = SessionLog(tmp_path).lane("plan", NAME).step_windows()
    assert windows == [StepWindow("overview", at(10, 0, 0, 500000), at(10, 0, 1, 750000))]
    assert windows[0].duration_s == 1.25
