---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---

# Feature: 000077_html-log-viewer

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Maintainers will be able to open any recorded run as a web page that draws it as a waterfall: every step, task, helper agent, wait on the user, other marked work and command laid out on a timeline, nested under what contains it, with long pauses easy to spot. Numbers in a report say how long a run took; a picture shows at a glance where it stalled, what ran at the same time and what waited on what. The page is a single file that opens in any browser without a server, so it can be kept with a test run or passed to a colleague.


<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **Users can render a run as a page**
  Users can produce a web page for one recorded run, selecting it by workflow name, by run identifier, by workflow kind or by agent (every way the run statistics report selects a run), with the most recent matching run as the default.
- [ ] **The page draws a waterfall**
  The page must draw the run's spans as bars on a shared timeline, each bar placed at its start and sized by its duration, with every span nested under its parent.
- [ ] **Span types are distinguishable**
  Steps, tasks, helper agents, waits on the user, other work an agent marked (general work), and individual commands must each be visually distinct, with waits on the user clearly set apart.
- [ ] **Problem spans stand out**
  Spans that failed, were interrupted or were abandoned must be marked so they stand out from finished work.
- [ ] **Details are available on demand**
  Selecting a span must show its name, type, start, duration, status and recorded details such as the agent, workflow and step.
- [ ] **The page shows the run's totals**
  The page must show the run's wall time, working time, user-wait time and helper-wait time, matching the run statistics report.
- [ ] **Long gaps are visible**
  Gaps between recorded events longer than a threshold must be visible on the timeline; the threshold defaults to 5 minutes, as in the run statistics report, and users can change it.
- [ ] **Users can zoom and collapse**
  Users can zoom into part of the timeline and collapse or expand any span's children.
- [ ] **The page works on its own**
  The page must be a single file that opens in a current browser with no server and no network access.
- [ ] **Large runs stay usable**
  A run with several thousand spans must render and respond to zooming and selection without noticeable delay.
- [ ] **Collected logs can be rendered**
  Users can render a run from a logs directory other than the project's own, such as one collected from a test run.
- [ ] **The documentation describes the viewer**
  User documentation must describe how to render a run and how to read the page.


<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- The viewer must render a run to HTML (user: "Eventually we could build a log viewer that renders to html").
- The viewer must read runs through the trace span records and the run statistics report (000074_trace-spans-for-agent-work and 000075_run-stats-command), on which this spec depends.


<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **A page is produced for the latest run**
  Asking for a page with no selection produces an HTML file for the most recent run; selecting by a workflow's name, by run identifier, by workflow kind or by agent produces one for the most recent run matching that selection.
- [ ] **Bars match the spans**
  For a run, the page shows one bar per recorded span, each starting and ending at the span's recorded times, nested under its parent span.
- [ ] **Types and problems are distinct**
  On a run containing every span type, a failed helper launch, an interrupted step and an abandoned span, each type has its own appearance, user waits are set apart, and the failed, interrupted and abandoned items are each marked.
- [ ] **Selecting shows details**
  Selecting a helper-agent bar shows its name, type, start, duration, status, agent, workflow and step.
- [ ] **Totals match the report**
  The page's wall, working, user-wait and helper-wait times equal the run statistics report's figures for the same run.
- [ ] **A long gap is visible**
  For a run with a 20-minute pause and a 2-minute pause, the 20-minute pause is visibly marked on the timeline at the default threshold and the 2-minute one is not; with the threshold set to 1 minute, both are marked.
- [ ] **Zoom and collapse work**
  Zooming into one step shows only that period across the width of the timeline, and collapsing a step hides its children until expanded.
- [ ] **The page works offline**
  The HTML file opens and renders fully with networking disabled and no server running.
- [ ] **A large run stays responsive**
  A run with 5,000 spans renders in under 3 seconds and responds to a zoom or selection in under 200 ms in a current desktop browser.
- [ ] **Collected logs render**
  Pointing the viewer at a copy of a project's logs directory produces a page showing the same spans, totals and layout as rendering in the project.
- [ ] **The docs describe the viewer**
  The documentation site describes rendering a run and reading the page; the site builds without errors.


<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- Add a `spektacular stats` form (or a sibling command) that writes the page.
- Embed the run's span data and a small inline script and stylesheet in one static file.
- Prefer drawing with plain browser features (SVG or canvas) over a charting library, to keep the file small and self-contained.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Timed on a sample implement run after delivery, the maintainer finds the slowest step and the longest stall from the page in under a minute, without reading the log.
- The page for a run of about 2,000 spans is under 1 MB.


<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- A live view of a run as it happens, or a server that hosts pages.
- Drawing several runs on one page or comparing runs visually.
- Tokens, models or cost, which the logs do not hold.
- Editing or annotating runs from the page.

