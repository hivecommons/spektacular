- **Run statistics package (new, `spektacular` repo).** This package owns everything the report computes. It is built in layers, each using the one before:
  - **Log loader.** Reads a logs directory read-only, line by line, into records. It skips and counts lines that are not valid records: unparsable JSON, a missing trace id, record kind or start time, or a partly written last line. It never fails on a single line.
  - **Run assembler.** Groups records into runs by trace id, across files. It keeps only traces that carry workflow attributes, takes kind, name, agent and log file from their records, and pairs span starts and ends by span id. It builds each run's event timeline, leaving out the ends of spans closed as abandoned, and derives the run's start, end, wall time and status.
  - **Selector.** Narrows runs by name, run id (or unique prefix), kind and agent, and applies each mode's defaults: the most recent run; the two most recent of its kind; every run of its kind.
  - **Run report.** Builds one run's totals, per-step figures, step entries, per-task figures, helper launches, user waits and gaps. It uses a small interval helper (union, clip, total) for waits and gaps.
  - **Comparison and summary.** Line runs up by step name. They produce each figure's per-run values with differences from the first run, or the count, median and 90th percentile across runs.
  - **Text renderer.** Draws the same results as plain text tables for people.

  The package depends only on the trace log package from 000073 and 000074, for the record type and the record, span-type, end-reason and attribute names. Its analysis is pure, so the command layer and spec 000077's viewer both reuse it.
- **Stats command group (new, `spektacular` repo).** `stats list`, `stats run`, `stats compare` and `stats summary`. It owns:
  - the shared flags: `--dir`, `--format`, `--name`, `--kind`, `--agent`, and a repeatable `--run`;
  - the run report's `--gaps` and `--gap-threshold`, which comparisons and summaries also use for their gap figures;
  - working out the logs directory: `--dir`, otherwise the project's configured log directory;
  - input validation and every refusal, each with a next action;
  - `--schema`.

  It writes results through the shared output writer (the JSON envelope and `--fields`) or through the package's text renderer. A bare `stats` returns the standard unknown-subcommand error. It carries the logging-exempt marker and is registered on the root command.
- **Command recorder in the root command (changed, `spektacular` repo; rewritten by 000073).** It learns one rule: a command marked logging-exempt, or one whose parent is, writes no log record. That keeps every report read-only inside a project. Nothing else in 000073's pipeline changes.
- **Output writer (reused, unchanged).** It provides the JSON envelope, the `"error": false` discriminant, the `--fields` filter and the error shape.
- **Trace log package (reused, unchanged; from 000073 and 000074).** It is the source of the record type and the names the report keys on. Its per-run reader and span index are not used by the report.
- **README (changed, `spektacular` repo).** The "How It Works" command overview gains the four `stats` forms, next to `status`.
- **Documentation site (changed, `docs` repo).** A new "Run statistics" page explains:
  - listing, reporting, comparing and summarising runs;
  - selecting runs;
  - reporting on a copied logs directory;
  - what each figure means, and how unfinished work and damaged lines are shown.

  The Resources navigation links to it, and the logging page gains one sentence pointing to it.
