---
created_date: "2026-10-24"
document_status: draft
specs:
    - name: 000072_knowledge-access-index-and-prune
      depends_on: []
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---

# Workflow tracing and run stats

## Overview

The performance investigation (`docs/workflow-performance-investigation.md`) had to rebuild where spec, plan and implement time goes from agent transcripts, Harbor parsers and guesswork. This epic makes the `spektacular` CLI record that itself. A log mode replaces debug mode and is on by default, writing OpenTelemetry-style trace spans for every CLI call, tagged with the agent, run and step, to a logs directory kept out of git. `goto` opens and closes each step's span, so agents only mark work the CLI cannot see, such as tasks and sub-agent launches, and waits on the user are their own span type so they can be left out of timings. A `spektacular stats` command reports per-run and per-step timing from those logs, Harbor moves onto it instead of its own transcript parsers, and a later HTML viewer draws a run as a waterfall. The epic also carries the self-written knowledge base with its usage index and prune command, which came out of the same investigation's push to stop blocking agents on questions.

## Specs

- **000072_knowledge-access-index-and-prune**: agents write knowledge without asking; a JSON access index records discovery and retrieval; `knowledge prune` removes unused and incorrect entries.
