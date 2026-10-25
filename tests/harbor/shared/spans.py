"""Reads the spans Spektacular records in its run logs (spec 000074).

Besides one `span` record per command, a run's log under `.spektacular/logs/`
holds spans that open in one command and close in another, as a
`span_start` and a `span_end` record sharing a `span_id`:

- `step` and `task` spans, which the workflow records by itself;
- `sub-agent`, `user-wait` and `work` spans, which agents mark with
  `spektacular trace start` and `spektacular trace stop`.

A `span_end` repeats every field of its start, so it is a complete span on
its own; a `span_start` with no end is a span still open. Spans are read
from every log file: workflow run files (lanes among them) and the day files
of standalone spans, such as an epic orchestrator's child launches.

The verifiers read spans to check that helper launches and questions to the
user were marked. Copied into each suite's tests/ by the Makefile with the
other shared helpers.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from sessionlog import parse_time

SUB_AGENT = "sub-agent"
USER_WAIT = "user-wait"
STEP = "step"
TASK = "task"


@dataclass
class Span:
    trace_id: str
    span_id: str
    parent: str | None
    name: str
    type: str | None
    start: datetime
    end: datetime | None  # None: still open
    status: str | None  # "ok" | "error"; None while open
    end_reason: str | None
    step: str | None  # the step it started in
    lane: str  # the log file's stem

    @property
    def open(self) -> bool:
        return self.end is None


def _span(raw: dict, lane: str) -> Span | None:
    if not raw.get("start_time") or not raw.get("span_id"):
        return None
    attrs = raw.get("attributes") or {}
    end = raw.get("end_time")
    return Span(
        trace_id=raw.get("trace_id") or "",
        span_id=raw["span_id"],
        parent=raw.get("parent_span_id") or None,
        name=raw.get("name") or "",
        type=attrs.get("spektacular.span.type"),
        start=parse_time(raw["start_time"]),
        end=parse_time(end) if end else None,
        status=(raw.get("status") or {}).get("code") if end else None,
        end_reason=attrs.get("spektacular.span.end_reason"),
        step=attrs.get("spektacular.step"),
        lane=lane,
    )


def read_spans(log_dir: Path) -> list[Span]:
    """Every span in the log directory, paired by trace and span id, in the
    order their first record appears (files in name order). An end without
    its start is still a complete span."""
    spans: dict[tuple[str, str], Span] = {}
    log_dir = Path(log_dir)
    if not log_dir.is_dir():
        return []
    for path in sorted(log_dir.glob("*.jsonl")):
        for line in path.read_text().splitlines():
            if '"span_start"' not in line and '"span_end"' not in line:
                continue
            try:
                raw = json.loads(line)
            except json.JSONDecodeError:
                continue
            kind = raw.get("record")
            if kind not in ("span_start", "span_end"):
                continue
            span = _span(raw, path.stem)
            if span is None:
                continue
            key = (span.trace_id, span.span_id)
            if kind == "span_end" or key not in spans:
                spans[key] = span
    return list(spans.values())


def of_type(spans: list[Span], span_type: str) -> list[Span]:
    return [s for s in spans if s.type == span_type]


def user_waits(spans: list[Span]) -> list[Span]:
    return of_type(spans, USER_WAIT)


def sub_agents(spans: list[Span]) -> list[Span]:
    return of_type(spans, SUB_AGENT)
