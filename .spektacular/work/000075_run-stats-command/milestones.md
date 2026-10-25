### Milestone 1: Maintainers can list runs and see where one run's time went

**What changes**: `spektacular stats list` shows every workflow run in the logs, newest first. Each run shows its kind, name, agent, start time, wall time, and whether it finished, was interrupted or is still in progress. `spektacular stats run` reports on one run, the latest by default, or one chosen by name, run id, kind or agent. The report gives:
- the run's wall time, working time, time waiting on the user and time waiting on helper agents;
- each step's total time, entry count, commands, command time and waits;
- each plan task's time and waits, for an implement run;
- every helper agent launched, with its step and duration;
- every wait on the user;
- the longest pauses, with the step each fell in, and the total time in pauses over a threshold.

Interrupted and unfinished steps, tasks, helpers and waits are flagged, and they never inflate a total. Both commands print a readable table, or the standard JSON envelope with `--format json`. They work on a logs directory copied from anywhere with `--dir`. A damaged log line is skipped and counted. Reporting never changes a log, because the `stats` commands are never logged themselves.

**Validation point**: The package's unit tests and the `cmd` integration tests pass in the full shuffled Go suite. They cover listing, every selection, totals, steps, tasks, helpers, gaps, unfinished work, damaged lines, a copied directory and byte-identical logs after reporting. Running the built binary against a fixture logs directory shows a readable list and run report.

### Milestone 2: Maintainers can compare runs and summarise many

**What changes**: `spektacular stats compare` puts two or more runs side by side, step by step and in total. It shows each run's figures and its difference from the first run, in time and as a percentage. A step one run never entered shows as absent for that run, not as zero. By default it compares the two most recent runs of the latest run's kind. `spektacular stats summary` summarises a set of runs, by default every run of the latest run's kind. For each step and for the totals, it gives the number of runs, the median and the 90th percentile. Both take the same run selection and output formats as the run report. This is what makes a slow run or a regression visible across runs, and what the Harbor harness's cross-run report will read in spec 000076.

**Validation point**: Unit tests over five fixture implement runs give the expected counts, medians and percentiles. A two-run spec comparison shows each step's values, its differences, and an absent step as `null`. `cmd` tests show both commands in both formats, with their refusals. The full Go suite passes.

### Milestone 3: The run report is documented

**What changes**: The documentation site gains a "Run statistics" page, linked from the Resources menu and from the logging page. It explains how to list, report on, compare and summarise runs, how to choose runs, and how to report on a copied logs directory. It also defines every figure: wall time, working time, user wait, helper wait, command time, step entries, gaps and the gap threshold, unfinished work, and skipped lines. The project README lists the four `stats` commands beside `status`.

**Validation point**: The site builds and `astro check` passes, with no layout HTML in the page body and no em dashes in the new text. The README shows the `stats` commands.
