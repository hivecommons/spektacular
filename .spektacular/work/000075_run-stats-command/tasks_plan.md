<!-- Milestone 1 -->

#### - [ ] Task: Load logs into runs
**Id:** dad1813d-55e1-4a5e-9ee0-667f48f26a41
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the run statistics package's foundation, which turns a logs directory into runs. It reads every log file read-only, skips and counts lines it cannot read, groups records into runs by run identifier, and pairs each span's start with its end. From those it works out each run's kind, name, agent, start and end, wall time and status. It also picks runs by name, run id, kind or agent, with each report mode's defaults. This task starts only once 000073's trace log package and 000074's span records are merged.

*Technical detail:* [context.md#task-load-logs-into-runs](./context.md#task-load-logs-into-runs)

**Acceptance criteria**:
- [ ] Records from run files and day files are grouped into one run per workflow run, and commands run outside any workflow are not reported as runs.
- [ ] Each run has the kind, name, agent and log file its records name, and its wall time runs from its first recorded event to its last, ignoring the moment a later restart closed its leftover spans.
- [ ] A run that reached its last step is finished. One that was restarted or overtaken by a later run of the same workflow is interrupted. Any other is in progress.
- [ ] A malformed line, a line missing its identifiers and a partly written last line are each skipped and counted, and every run's figures are unchanged by them.
- [ ] Runs can be picked by workflow name, by kind, by agent, by these combined, and by a full run id or a unique prefix of one. An unknown, ambiguous or malformed id is refused with a reason.
- [ ] With no selection, a report picks the latest run, a comparison the two latest runs of that run's kind, and a summary every run of that kind.
- [ ] Reading never creates, changes, moves or deletes a file in the logs directory.

#### - [ ] Task: Report on one run
**Id:** d2c23d18-d5b3-4e67-9c65-c8c3354474b9
**Repo:** spektacular
**Depends on:**
- dad1813d-55e1-4a5e-9ee0-667f48f26a41 — Load logs into runs
**Execution:** agent

Builds the report on one run:
- its totals: wall time, working time, user wait and helper wait;
- a row per step: entries, duration, commands, command time and waits;
- each step entry and each plan task;
- every helper launch and wait on the user;
- the longest gaps between recorded events, each with its step.

Waits are counted once even when helpers overlap. Interrupted and unfinished work is flagged and never inflates a total. The report is plain data, ready to be printed as a table or as JSON.

*Technical detail:* [context.md#task-report-on-one-run](./context.md#task-report-on-one-run)

**Acceptance criteria**:
- [ ] For a run with known waits, working time equals wall time less the user waits, and the user-wait and helper-wait totals equal the sums of those periods. Two helpers running at the same time are counted once.
- [ ] A step entered twice shows two entries, its total duration, its command count, its command time, and its helper and user wait.
- [ ] An implement run of a two-task plan shows both tasks, each with its duration and waits.
- [ ] A run with three helper launches lists all three, each with its name, step and duration.
- [ ] A 20-minute pause between two commands is listed first among the longest gaps, with its step and the events either side. At most the requested number of gaps is listed, longest first. The pause counts toward the total over the default 5-minute threshold, but not over a 30-minute one.
- [ ] An interrupted step is marked and counted up to where it was cut off. A helper launch or user wait that never finished is listed and marked, and left out of every wait total and per-step wait.
- [ ] The structured form of the report uses exactly the field names and units given in the plan's Data Structures section.

#### - [ ] Task: Keep the stats commands out of the logs
**Id:** a49a4d72-79d8-4ce2-b320-6a6b63eca363
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teaches the place every command is logged to leave out commands marked as exempt from logging. Without this, running a report inside a project would add a line to the day's log, and possibly trigger the clean-up of old logs, so the report would change the logs it reads. A command is exempt when it, or a command group above it, carries the marker. This task starts only once 000073's rewritten command recorder is merged.

*Technical detail:* [context.md#task-keep-the-stats-commands-out-of-the-logs](./context.md#task-keep-the-stats-commands-out-of-the-logs)

**Acceptance criteria**:
- [ ] A command carrying the logging-exempt marker, directly or through its group, leaves no record, even with logging at the verbose level.
- [ ] Every other command is still logged exactly as before, and its output and exit code are unchanged.

#### - [ ] Task: List runs and report on one from the command line
**Id:** ad0176a5-97fb-47c0-98b1-7267765659a2
**Repo:** spektacular
**Depends on:**
- dad1813d-55e1-4a5e-9ee0-667f48f26a41 — Load logs into runs
- d2c23d18-d5b3-4e67-9c65-c8c3354474b9 — Report on one run
- a49a4d72-79d8-4ce2-b320-6a6b63eca363 — Keep the stats commands out of the logs
**Execution:** agent

Adds the `spektacular stats` command group with `list` and `run`. Both read the project's logs, or the directory given with `--dir`, and both take the run selection. They print readable tables by default, or the standard JSON envelope with `--format json`. `run` also takes the number of gaps to list and the gap threshold. Every mistake is refused with a message saying what to do. The group is marked as exempt from logging.

*Technical detail:* [context.md#task-list-runs-and-report-on-one-from-the-command-line](./context.md#task-list-runs-and-report-on-one-from-the-command-line)

**Acceptance criteria**:
- [ ] In a project whose logs hold three finished runs and one in progress, `stats list` shows all four, newest first, each with kind, name, agent, start time, wall time and status.
- [ ] `stats run` with no selection reports the run that started most recently, and each selection reports only the runs it names.
- [ ] The JSON output parses in the same envelope as other commands and honours `--fields`, and the table output shows the same figures.
- [ ] Pointing `--dir` at a copy of the logs directory, from outside any project, gives the same figures as reporting in the project.
- [ ] A malformed log line leaves every figure unchanged, and the report shows one skipped line.
- [ ] Every log file is byte-identical before and after `stats list` and `stats run`, run inside a project with logging on.
- [ ] An unsupported format, no project without `--dir`, a missing directory, no matching run, an unknown or ambiguous run id, a bad kind, and a bad gap count or threshold are each refused with a message naming the fix. A bare `stats` names its subcommands.

<!-- Milestone 2 -->

#### - [ ] Task: Compare and summarise runs
**Id:** 8c6b6ead-cdbf-48a4-b7f3-ce6a3a5ac516
**Repo:** spektacular
**Depends on:**
- d2c23d18-d5b3-4e67-9c65-c8c3354474b9 — Report on one run
**Execution:** agent

Adds side-by-side comparison and multi-run summaries to the run statistics package. A comparison lines runs up step by step and in total. It shows each run's figures and its difference from the first run, in time and as a percentage, and shows a step a run never entered as absent. A summary gives, for each step and for the totals, the number of runs, the median and the 90th percentile. Both come with readable table forms.

*Technical detail:* [context.md#task-compare-and-summarise-runs](./context.md#task-compare-and-summarise-runs)

**Acceptance criteria**:
- [ ] Comparing two spec runs shows each step's duration for both runs, and the second run's difference from the first in time and percentage. The totals are shown the same way.
- [ ] A step only one run entered is shown as absent for the other, never as zero, and a difference against a zero or absent base has no percentage.
- [ ] Summarising five implement runs shows, for each step and for the totals, a count of 5, the median and the 90th percentile.
- [ ] A step entered by only some of the summarised runs shows how many runs entered it.

#### - [ ] Task: Compare and summarise from the command line
**Id:** d2baacfc-4bdf-4150-8660-4e6a52befcbc
**Repo:** spektacular
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- 8c6b6ead-cdbf-48a4-b7f3-ce6a3a5ac516 — Compare and summarise runs
**Execution:** agent

Adds `spektacular stats compare` and `spektacular stats summary`, with the same run selection, logs directory and output formats as `stats run`. With no selection, `compare` takes the two most recent runs of the latest run's kind, and `summary` takes every run of that kind. Naming runs with `--run` compares or summarises exactly those, in the order given.

*Technical detail:* [context.md#task-compare-and-summarise-from-the-command-line](./context.md#task-compare-and-summarise-from-the-command-line)

**Acceptance criteria**:
- [ ] `stats compare` with no selection compares the two most recent runs of the latest run's kind, oldest first. With two or more `--run` values, it compares exactly those, in that order.
- [ ] `stats summary` with no selection summarises every run of the latest run's kind, and accepts the same selection as the other commands.
- [ ] Both commands give the same figures in table and JSON form, and the JSON uses the field names in the plan's Data Structures section.
- [ ] Asking to compare fewer than two runs is refused with a message saying how to name two.
- [ ] Every log file is byte-identical before and after `stats compare` and `stats summary`.

<!-- Milestone 3 -->

#### - [ ] Task: Documentation site describes run statistics
**Id:** a86aebb6-e10d-48b7-9fbb-a7e273e0a257
**Repo:** docs
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- d2baacfc-4bdf-4150-8660-4e6a52befcbc — Compare and summarise from the command line
**Execution:** agent

Adds a "Run statistics" page to the documentation site. It explains how to list, report on, compare and summarise runs, how to choose runs, and how to report on a copied logs directory, and it defines every figure the report shows. The Resources menu links to the page, and the logging page points to it.

**Content outline** (new page, in order; the wording is illustrative, the commands, flags, defaults and field names are fixed):
1. Hero: heading "Run statistics", sub "Ask Spektacular where a run's time went: in each step and task, waiting on helper agents, or waiting on you."
2. `## Listing runs`: `spektacular stats list` reads `.spektacular/logs/` and shows each workflow run, newest first. Example block:
   ```text
   RUN       KIND       NAME                       AGENT   STARTED              WALL     STATUS
   4bf92f35  implement  000075_run-stats-command   claude  2026-10-25 09:10:02  41m12s   in_progress
   9d0e0e47  plan       000075_run-stats-command   claude  2026-10-25 08:20:50  41m21s   finished
   ```
3. `## Reporting on one run`: `spektacular stats run` reports the latest run. Shows the totals line, the per-step table (STEP, ENTRIES, DURATION, COMMANDS, CMD TIME, HELPER WAIT, USER WAIT), tasks for implement runs, helper launches, waits on the user and the longest gaps. `--gaps 3` and `--gap-threshold 30m` change the gap list and threshold.
4. `## Choosing runs`: `--name <workflow name>`, `--kind spec|plan|implement|repo`, `--agent <agent>`, `--run <id or 8+ character prefix>` (repeatable), combined with AND. Defaults: `run` takes the latest run; `compare` takes the two latest runs of that run's kind; `summary` takes every run of that kind.
5. `## Comparing runs`: `spektacular stats compare --run 9d0e0e47 --run 4bf92f35`. Each step's duration per run, the difference from the first run in time and percent, and the totals. A step a run never entered shows as `-`, not `0s`.
6. `## Summarising runs`: `spektacular stats summary --kind implement`. Per step and in total: the number of runs, the median and the 90th percentile.
7. `## What each figure means`: a definition list.
   - **Wall time**: from the run's first recorded event to its last.
   - **Working time**: wall time less user wait.
   - **User wait**: time a `user-wait` span was open and finished, counted once when several overlap.
   - **Helper wait**: time at least one `sub-agent` span was running, counted once when helpers overlap.
   - **Command time**: the time Spektacular's own commands took.
   - **Entries**: how many times a step was entered.
   - **Gap**: a stretch with no recorded event, labelled with the step it fell in and the events either side.
   - **Gap total**: the time in gaps longer than the threshold (5 minutes by default).
8. `## Unfinished work and damaged lines`: steps, tasks, helpers and waits that were `interrupted`, `abandoned` or never closed (`open`) are marked. An interrupted step counts up to where it was cut off. Unfinished helpers and waits are listed but left out of every wait total. A line that cannot be read is skipped and counted as `skipped_lines`.
9. `## Reports for programs`: `--format json` gives the standard envelope. Example excerpt: `{"run":{"run_id":"...","kind":"plan","wall_ms":2481277,"status":"finished"},"totals":{"wall_ms":2481277,"working_ms":1803000,"user_wait_ms":678277,"helper_wait_ms":512000,"gap_ms":1520000},"steps":[{"step":"discovery","entries":2,"duration_ms":812000}],"error":false}`. Durations are milliseconds, times are UTC, and these field names are stable.
10. `## Logs from elsewhere`: `spektacular stats run --dir path/to/collected/logs` works on a logs directory copied from a test run, from any folder. Reporting never changes a log, and `stats` commands are never logged themselves.
11. CtaBanner: "Where do these records come from?" with a button to the Logging and debugging page.

*Technical detail:* [context.md#task-documentation-site-describes-run-statistics](./context.md#task-documentation-site-describes-run-statistics)

**Acceptance criteria**:
- [ ] The Run statistics page explains listing, reporting on, comparing and summarising runs, choosing runs, and reporting on a copied logs directory.
- [ ] The page defines every figure the report shows, and how unfinished work and skipped lines appear.
- [ ] The Resources menu links to the page, and the logging page points to it.
- [ ] The site builds and type-checks without errors, and the page body has no layout HTML and no em dashes.

#### - [ ] Task: README lists the stats commands
**Id:** b152f2a6-8bbc-4b92-b981-558c68cb032e
**Repo:** spektacular
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- d2baacfc-4bdf-4150-8660-4e6a52befcbc — Compare and summarise from the command line
**Execution:** agent

Adds the four `stats` forms to the README's command overview, next to `status`, with one line each on what they show. It also gives a pointer to the documentation site for the figure definitions.

**Content example** (README "How It Works", after the `status` block):
```text
spektacular stats list                     # every recorded run, newest first
spektacular stats run [--name <spec>]      # where one run's time went: steps, tasks, helpers, waits, gaps
spektacular stats compare                  # the two latest runs of a kind, side by side
spektacular stats summary --kind implement # median and 90th percentile per step
```

*Technical detail:* [context.md#task-readme-lists-the-stats-commands](./context.md#task-readme-lists-the-stats-commands)

**Acceptance criteria**:
- [ ] The README shows `stats list`, `stats run`, `stats compare` and `stats summary` with a one-line description each, and mentions `--dir` and `--format json`.
