**A read-side package beside the write-side one.** 000073 and 000074 make `internal/tracelog` the package that writes the log. This plan adds its counterpart for reading: `internal/runstats`.
- It imports `tracelog` for the record type and the names. `tracelog` never imports it, so the write path every command takes stays as small as 000073 planned.
- Inside the package, I/O sits at one edge: the loader, and the text renderer writing to an `io.Writer`. Everything between them is a pure function from records to results.
- A developer reading it finds one file per stage (load, assemble, select, report, gaps, intervals, compare, summary, render). Each stage has a table-driven test fed with hand-built records.
- This follows the layering of `internal/status`, which builds a `Report` as data and renders it separately as pretty text. `cmd` only wires flags to the package.

**One timeline per run, and everything measured against it.** Assembly turns a run's records into three things:
- paired spans, each with an effective end and a `complete` flag;
- its command records;
- a sorted list of timeline events.

Every figure then comes from those three:
- wall time is the timeline's extent;
- waits are interval unions of complete spans;
- per-step and per-task figures clip those unions to each entry's interval;
- gaps are the uncovered stretches between consecutive timeline events;
- status comes from the end reasons and from the other runs of the same workflow.

A small interval helper (merge, clip, total) is the only arithmetic primitive, so double counting is impossible by construction. The rules for unfinished work live in one place, the effective-end computation, instead of being repeated in each figure: abandoned ends are left out of the timeline, and open or abandoned spans are capped at the run's last other event.

**Comparison and summary as figure tables.** Both start from per-run reports. They join the reports by step name, keeping the step order of the first run that has each step, and emit one generic row per figure. That generic shape (`figure` with aligned `values`/`delta`/`delta_pct`, or `count`/`median`/`p90`) is new to the codebase. It is chosen so a later spec can add a figure without changing the output's shape. Harbor reads it in 000076, and the viewer in 000077.

**The command follows the existing group pattern.** `stats` is built like the other command groups:
- a cobra group whose bare form returns the `unknown_subcommand` envelope;
- subcommands with shared flags;
- `--schema` through `commandSchema`;
- `--format pretty|json` validated as `status` validates it;
- JSON through the shared output writer, so `--fields` works;
- every refusal an `output.NewError` with a next action.

Sentinel and typed errors from the package (an unknown, ambiguous or malformed run id, no runs, too few to compare) are mapped to output errors in one place in `cmd`.

**Exempt from logging, by annotation.** The gate already uses a cobra annotation to let a few commands through. The root recorder gets a matching one: a command whose path carries the logging-exempt annotation is left out of the log. It is checked on the command that actually ran, so it works in tests and in production. The `stats` group sets it once, on the group, and its subcommands inherit it through the parent walk. This is the only change to code from 000073. It is a new, reusable rule: a read-only log command is never logged.

**Reading large logs cheaply.** Verbose-level lines carry full responses and can be many megabytes long. The loader:
- reads lines with no length cap;
- decodes into the record type, whose attributes are a plain map;
- streams each file rather than holding it whole.

A benchmark over a generated ten-run directory backs the under-one-second success metric. No index, cache or sidecar file is introduced.
