---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
amendments:
    - at: "2026-10-25T10:05:20Z"
      sections: [Requirements, Acceptance Criteria]
      hash: sha256:611a577a90b4cb3b6343e8e438d223c69741091c16fdfe82b9dbfc30d2ee927b
    - at: "2026-10-25T10:17:30Z"
      sections: [Success Metrics]
      hash: sha256:f9d33619dcf1deeaaa8b2763eebdfa9de0a956b9f9fb5974a6ce71eb8ec7821e
---

# Feature: 000073_log-mode-and-logger-providers

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular will keep a lightweight, always-on record of every command an agent runs: when it started, how long it took, which workflow step and agent it belonged to, and any error. Today that record exists only when debugging is switched on, stores every full response, and still leaves maintainers piecing run timings together from agent transcripts. A small log that is on by default, kept out of version control and trimmed to recent runs gives maintainers reliable timings for every run, and is the foundation for the run statistics, test reporting and run viewer that follow in this epic.


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

- [ ] **Every command is logged by default**
  The system must record every Spektacular command run inside a project, without the user turning anything on, unless the project's configuration switches logging off. Commands that only report on the project or its logs (status and run statistics) are never recorded, so polling and reporting never add to or trim the logs.
- [ ] **Each command is recorded once, linkable to its run**
  The system must record each command once, with its name, start time, end time, success or error status, and identifiers that link it to the other commands of the same run.
- [ ] **Records identify the agent, run and step**
  Each record must carry the agent name, the workflow kind and name, the workflow step the command ran in and, when the command moved the workflow to another step, the step it moved to. When the configuration names no agent, the record carries the agent as `unknown`.
- [ ] **A run is one trace**
  All commands of one workflow run must share one run identifier: the same across an interruption and resume, new when the run is restarted from scratch, and different for each of several workflows running at the same time. A command run outside any workflow gets an identifier of its own.
- [ ] **Runs started before the upgrade keep working**
  A workflow already in progress when the project upgrades to this version must keep resuming without error; it is given a run identifier on its first command after the upgrade, and keeps it from then on.
- [ ] **The default level records timing and errors only**
  At the default level, each record must hold the command, its identification, its timing, its exit status and any error message, and must not hold the command's full response or workflow state snapshots.
- [ ] **A verbose level keeps the full record**
  Users can choose a verbose level that additionally records each command's full response and the workflow state before and after it, as debug mode records today.
- [ ] **Logging can be switched off**
  Users can switch logging off in the project's configuration, after which no command is recorded.
- [ ] **The logger is chosen in configuration**
  Users can choose which logger is used in the project's configuration; a file logger is the default. An unknown logger name is reported as a configuration error that names the accepted loggers.
- [ ] **The log location is configurable**
  Users can set the directory logs are written to; by default logs go to a logs directory inside the project's Spektacular folder.
- [ ] **Logs stay out of version control**
  Projects initialised or migrated by Spektacular must ignore the default logs directory in version control, so logs are never committed by accident.
- [ ] **Only the most recent runs are kept**
  The system must keep logs for the most recent workflow runs only and delete older ones automatically; the number kept is set in the project's configuration and defaults to 10. Commands run outside any workflow are grouped into one log per day, and the same number of most recent days is kept, so standalone commands never push out workflow runs.
- [ ] **Logging never breaks a command**
  A failure to write a log, or to delete old logs, must never change a command's output or exit status.
- [ ] **Debug configuration is migrated**
  Running the project migration must convert an existing debug setting into the new logging configuration, keeping the user's intent: debug on becomes the verbose level, debug off becomes the default level.
- [ ] **Old debug logs are left alone**
  The system must neither read, move nor delete existing debug log files; removing them is left to the user.
- [ ] **The test harness keeps its session reporting**
  The project's end-to-end test harness must keep reporting the workflow steps and parallel runs it reports today, reading them from the new logs, until a later spec in this epic replaces that reporting.
- [ ] **The documentation describes logging**
  User documentation must describe logging (what each level records, where logs go, retention, switching it off, and which commands are never recorded), and the configuration documentation must cover the logging settings and no longer cover debug.


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

- Records must follow the OpenTelemetry trace data model, written by Spektacular's own output logger (user: "use the otel trace approach and then write a custom output logger").
- The logger must be chosen through a provider setting, following the existing spec and plan provider pattern (user: "a logger like we have plan providers").
- Logs must be JSON Lines files, written by default to a logs directory inside the project's Spektacular folder (user: "This file should be jsonp and should be put into a logs directory").
- Debug mode must be replaced by this log mode, not kept beside it (user: "refactor this debug mode to be a log mode").
- The agent name must come from the project's configuration, not be detected from the running environment (user, interview answer).
- The run identifier must come from the current workflow state, not be held anywhere else (user: "Use the id from the current status.json").
- The documentation site's debugging page must be the page updated to describe logging (user: "We need the debugging page updated to show logging").


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

- [ ] **A fresh project logs without setup**
  In a project newly initialised with no logging settings changed, running any Spektacular command leaves a JSON Lines log file in the project's logs directory containing a record of that command.
- [ ] **Records follow the trace data model**
  Every record of a command carries a trace identifier, a span identifier, a name, a start time, an end time and a status.
- [ ] **Records name the agent, workflow and step**
  In a project whose configuration names agent `claude`, a `spec goto` command's record carries the agent `claude`, the workflow kind `spec`, the spec's name, the step it ran in and the step the workflow moved to. With no agent in the configuration, the record carries the agent `unknown`.
- [ ] **A resumed run keeps its trace**
  Starting a spec workflow, running two `goto` commands, then running `spec new` without a name (the resume report) and another `goto` produces records that all carry the same trace identifier.
- [ ] **A restarted run gets a new trace**
  Running `spec new --force` for the same spec name produces records with a trace identifier different from the earlier run's.
- [ ] **Concurrent runs keep separate traces**
  Two plan workflows for different specs, run at the same time in the same project (as planning an epic does), produce records whose trace identifiers differ between the two specs and are each constant within one spec.
- [ ] **A run from before the upgrade resumes**
  A spec workflow started with the previous Spektacular version and resumed with this one continues without error, and every record from its commands after the upgrade carries the same trace identifier.
- [ ] **Commands outside a workflow get their own trace**
  With no workflow in progress, two consecutive `knowledge search` commands produce two records with different trace identifiers, both in the same log for that day.
- [ ] **The default level leaves out responses**
  At the default level, a record of a command whose output is several kilobytes contains its command, timing, exit status and error message (when it failed), and contains neither the command's output nor any workflow state snapshot.
- [ ] **A failed command records its error**
  At the default level, running a command that fails (for example `spec goto` with an unknown step) produces a record with an error status, a non-zero exit code and the error message the command returned.
- [ ] **The verbose level records the full response**
  With the verbose level configured, a command's record contains its full output exactly as returned and the workflow state before and after it.
- [ ] **Logging off records nothing**
  With logging switched off in the configuration, running several commands creates no log file and adds no record to existing ones.
- [ ] **Status and reporting are not recorded**
  With logging at the default level, running `status` and any run statistics command adds no record to any log.
- [ ] **The provider setting selects the logger**
  The configuration accepts the file logger and logs are written by it; an unknown logger name is reported as a configuration error naming the accepted loggers.
- [ ] **A custom directory is honoured**
  With the log directory set to another path, records are written there and nothing is written to the default logs directory.
- [ ] **Logs are ignored by version control**
  After `init` in a new git repository, and after `migrate` in a project initialised by an earlier version, writing logs leaves `git status` showing no files from the logs directory.
- [ ] **Only the configured number of runs is kept**
  With the default retention, after 12 separate workflow runs the logs directory holds the logs of the 10 most recent runs only; with retention set to 3, it holds the 3 most recent. Running many standalone commands in between removes no workflow run's log.
- [ ] **A logging failure does not fail the command**
  With the logs directory made unwritable, and separately with an old log that cannot be deleted when retention trims old runs, a command's output and exit code are identical to a run with logging switched off.
- [ ] **Migration converts debug settings**
  Running `migrate` on a project with debug turned on produces a logging configuration at the verbose level, on a project with debug turned off produces one at the default level, and in both cases the debug setting is gone from the configuration.
- [ ] **Old debug files are untouched**
  After `migrate` and further commands, every file that was in the old debug log directory is still there with identical contents.
- [ ] **The test harness still reports sessions**
  A Harbor workflow suite run after this change reports the same workflow steps and parallel runs in its results as before, with no reliance on the old debug log directory.
- [ ] **The docs describe logging**
  The documentation site's debugging page describes the levels and what each records, the logs location, retention, switching logging off and which commands are never recorded, and the configuration reference documents the logging settings and no longer documents a debug setting; the site builds without errors.


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

- Evolve the existing per-command session recorder into the file logger rather than starting afresh; it already records once per command, contains every failure, and files each workflow session separately.
- Each command is a short-lived process, so a span cannot live in memory across commands. Prefer self-contained records that later readers pair or nest by identifier; a per-command span starts and ends in one process and can be one record. Leave room in the record format for spec 2 of this epic, whose spans open in one command and close in another, and whose user-wait spans are their own type.
- Mint the run identifier when a workflow starts; the workflow state has no such field today.
- Keep one log file per workflow run, as the debug log does, so retention can count and delete whole runs.
- Add the logs directory to the ignore rules Spektacular already writes into a project, through `init` and `migrate`, rather than asking users to edit their own ignore files as the debugging page does today.
- The existing per-command environment switch (`SPEKTACULAR_SESSION_LOG`, set to `on` by the Harbor end-to-end test harness and to `off` by polling tools) can be kept and remapped onto levels, or replaced; only the maintainer and Harbor depend on it.
- Harbor's session-log reader (`tests/harbor/shared/sessionlog.py`) reads the debug log today; point it at the new logs with the smallest change, since spec 4 of this epic replaces it with `spektacular stats` output.
- Risk: the default level now runs on every command in every project, so writing a record and deleting old runs should stay cheap (see Success Metrics).
- Spec 3 of this epic (`spektacular stats`) reads these logs, so the record format is shared by the whole epic.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- For a real workflow run, the maintainer can read each step's start, end and duration from the log alone (from the times of the commands that changed step), without consulting the agent's transcript.
- Logging at the default level adds under 10 ms to a typical command's wall time, measured over 100 runs of `spec file list` with logging on and off.
- At the default level, one complete implement run of a plan with 10 tasks produces a log under 1 MB.
- No command's output or exit code differs between logging on and off across the full Go test suite and the Harbor workflow suites.


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

- Step spans opened and closed by workflow step changes, explicit start and stop commands for tasks and sub-agents, and user-wait spans are covered by the next spec in this epic (trace spans for agent work).
- Reading, summarising or comparing logs is covered by the run statistics spec in this epic.
- Replacing the test harness's own transcript and session-log parsers is covered by the Harbor spec in this epic; this spec only keeps its existing reporting working.
- A visual or HTML view of a run is covered by the log viewer spec in this epic.
- Exporting to an OpenTelemetry collector, standard output or any other logger beyond the file logger is deferred.
- Recording agent tool calls, model calls or token usage, which the CLI cannot see.
- Retention by age or by total size; only a count is configurable.

## Amendments

- **2026-10-25: Requirements, Acceptance Criteria** (spek-plan-epic review of epic 000002_workflow-tracing-and-run-stats)
  Epic end-of-planning review (user, 2026-10-25): status and run statistics commands are never logged, and the per-command environment override (SPEKTACULAR_SESSION_LOG) is removed; Harbor sets its log level in its seeded test projects config. The Technical Approach bullet about keeping or remapping the environment switch is superseded by this amendment.
- **2026-10-25: Success Metrics** (spek-plan-epic review of epic 000002_workflow-tracing-and-run-stats)
  Epic end-of-planning review (2026-10-25): status is no longer logged, so the overhead metric is timed on spec file list instead.
