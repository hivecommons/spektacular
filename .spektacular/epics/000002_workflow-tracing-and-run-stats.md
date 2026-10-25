---
created_date: "2026-10-24"
document_status: draft
specs:
    - name: 000072_knowledge-access-index-and-prune
      depends_on: []
    - name: 000073_log-mode-and-logger-providers
      depends_on:
        - 000072_knowledge-access-index-and-prune
    - name: 000074_trace-spans-for-agent-work
      depends_on:
        - 000073_log-mode-and-logger-providers
    - name: 000075_run-stats-command
      depends_on:
        - 000074_trace-spans-for-agent-work
    - name: 000076_harbor-on-built-in-logs
      depends_on:
        - 000075_run-stats-command
    - name: 000077_html-log-viewer
      depends_on:
        - 000075_run-stats-command
        - 000076_harbor-on-built-in-logs
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---






# Workflow tracing and run stats

## Overview

The performance investigation (`docs/workflow-performance-investigation.md`) had to rebuild where spec, plan and implement time goes from agent transcripts, Harbor parsers and guesswork. This epic makes the `spektacular` CLI record that itself. A log mode replaces debug mode and is on by default, writing OpenTelemetry-style trace spans for every CLI call, tagged with the agent, run and step, to a logs directory kept out of git. `goto` opens and closes each step's span, so agents only mark work the CLI cannot see, such as tasks and sub-agent launches, and waits on the user are their own span type so they can be left out of timings. A `spektacular stats` command reports per-run and per-step timing from those logs, Harbor moves onto it instead of its own transcript parsers, and a later HTML viewer draws a run as a waterfall. The epic also carries the self-written knowledge base with its usage index and prune command, which came out of the same investigation's push to stop blocking agents on questions.

## Specs

- **000072_knowledge-access-index-and-prune**: agents write knowledge without asking; a JSON access index records discovery and retrieval; `knowledge prune` removes unused and incorrect entries. Independent of the others.
- **000073_log-mode-and-logger-providers**: debug mode becomes an always-on log mode; every CLI call is an OpenTelemetry-model span carrying agent, run and step, written by a file logger to a git-ignored logs directory, with retention.
- **000074_trace-spans-for-agent-work**: step changes open and close step spans and implement records task spans; agents mark helper launches and user waits (their own type) with trace start and stop. Depends on 000073.
- **000075_run-stats-command**: `spektacular stats` lists, reports on, compares and summarises runs from the logs. Depends on 000074.
- **000076_harbor-on-built-in-logs**: Harbor takes its step, helper, idle and timeline measures from the logs through `stats`, keeping transcripts only for tokens, models and cost. Depends on 000075.
- **000077_html-log-viewer**: a run rendered as a self-contained HTML waterfall. Depends on 000075.
