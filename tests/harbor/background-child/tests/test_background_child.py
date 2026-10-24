"""Verifier for the background-child probe.

The agent was told to start one background child that sleeps three times for
five minutes, end its turn without waiting, and write the child's reply once
the completion notice arrives. The probe answers one question: does a
background child outlive ten minutes in a headless run, or is it stopped,
and does its completion notice start a new turn of the main agent?

The verifier reports the answer, and never asserts which way it went. It
fails only when the run did not set up the probe: no child, more than one,
a child that ran in the foreground, or a child that ended early by itself.

Claude only: other agents' logs cannot show a child's background state or
the turn its notice started.
"""

from __future__ import annotations

from pathlib import Path

import pytest

import delegation
import transcripts as tx

OUTCOME_FILE = Path("/logs/verifier/probe-outcome.txt")
TEN_MINUTES_S = 600

pytestmark = pytest.mark.skipif(not delegation.is_claude(), reason="the probe reads a Claude transcript only")


@pytest.fixture(scope="module")
def objs() -> list[dict]:
    return tx.read_jsonl(tx.CLAUDE_TRANSCRIPT)


@pytest.fixture(scope="module")
def children(objs) -> list[tx.Child]:
    return tx.claude_children(objs)[0]


@pytest.fixture(scope="module")
def turns(objs) -> list[tx.Turn]:
    return tx.claude_turns(objs)


def outcome_line(children: list[tx.Child], turns: list[tx.Turn]) -> str:
    woken = any(t.woken_by == "task-notification" for t in turns)
    if len(children) != 1:
        return f"PROBE OUTCOME: {len(children)} children started; notice-woken turn: {woken}"
    child = children[0]
    return f"PROBE OUTCOME: {child.ending} after {child.duration_s} s; notice-woken turn: {woken}"


def test_report_outcome(objs, children, turns):
    """Always passes: prints what happened, whichever way it went."""
    lines = [outcome_line(children, turns)]
    for c in children:
        lines.append(
            f"child {c.id}: background={c.background} ending={c.ending} duration_s={c.duration_s} "
            f"launched_at={c.launched_at} ended_at={c.ended_at} hand_back={c.hand_back!r}"
        )
    for i, t in enumerate(turns):
        lines.append(f"turn {i}: ended_at={t.ended_at} woken_by={t.woken_by}")
    for w in tx.claude_wakeups(objs):
        lines.append(f"orchestrator wake-up: {w.tool} {w.detail} at {w.at}")
    text = "\n".join(lines)
    print("\n" + text)
    try:
        OUTCOME_FILE.parent.mkdir(parents=True, exist_ok=True)
        OUTCOME_FILE.write_text(text + "\n")
    except OSError:
        pass


def test_exactly_one_child(children):
    assert len(children) == 1, f"expected exactly one child, the run started {len(children)}"


def test_child_ran_in_the_background(children):
    assert children, "the run started no child"
    assert children[0].background is True, f"the child ran with background={children[0].background}"


def test_child_outlived_ten_minutes_or_was_stopped(children):
    # Either is an answer to the probe. A child that ended by itself in under
    # ten minutes did not run the sleeps it was given, so the probe proves
    # nothing.
    assert children, "the run started no child"
    c = children[0]
    outlived = c.duration_s is not None and c.duration_s > TEN_MINUTES_S
    assert outlived or c.ending == "stopped", (
        f"the child neither ran past {TEN_MINUTES_S} s nor was stopped: "
        f"ending={c.ending} duration_s={c.duration_s}"
    )
