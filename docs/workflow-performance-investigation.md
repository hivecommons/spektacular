# Why spec, plan and implement are slow — investigation report

*2026-10-24. Based on every Claude Code session recorded for this project
(28 Sep – 24 Oct 2026), Spektacular's own debug session logs, and a fresh batch of
Harbor runs (`p24-*`) made for this report.*

## TL;DR

- **Most of the time goes to model turns, not to running tools.** Across 32 sessions
  ($569 of usage), Claude Code recorded **17 h of API time against 1 h of tool
  time**. Builds and tests barely register. What makes the workflows slow is how
  many turns they take, how many tokens each turn writes, and how often they stop
  to wait.
- **Each turn costs about 3.2 s, plus 7.4 s for every 1,000 output tokens.** Context
  size matters much less: about 0.3 s per 100k tokens. That gives four levers,
  roughly in order: **fewer serial sub-agent waits, fewer blocking questions to the
  user, fewer turns, and fewer tokens re-typed.**
- **Implement is dominated by waiting on sub-agents.** That's 40% of its time (27.5
  min per run), against 19% for the main thread's own model time. Every task runs
  a test sub-agent, a verify sub-agent and now a goal reviewer, one after another.
  In the fresh Harbor implement-epic run, `check_goal` alone is **46% of all step
  time**.
- **Spec is 75% waiting for the user; plan is 41%; implement is 28%.** Much of that
  waiting is built into the templates. Examples: a "continue with the next task?"
  prompt after every task, a plan walkthrough paced as four separate turns that the
  user answers with "ok", knowledge-capture offers in the middle of a run, and
  auto-commit confirmations. When the user steps away, each one stops all
  progress.
- **The per-task state machine is largely ceremony, and agents already work around
  it.** Across 106 real tasks, the median time spent in `update_plan` and in
  `update_changelog` was **0 s**: agents chain several `goto`s into one shell call.
  In Harbor, tasks 2–7 of one spec went through analyze → implement → test →
  verify in 0.0 s each. Meanwhile every step tells the agent to re-read `plan.md`
  (and usually `context.md`) in full. Ticking a checkbox is specified as "read
  the whole plan, Write the whole plan back".
- **Mechanical work is done by the model.** Plan `assemble`, the three `write_*`
  steps, and spec verification's assembly all have the LLM rebuild documents
  from work files it already wrote. The same goes for re-typing the goal
  reviewer's JSON reply word for word. Every token re-typed costs ~7 ms.

The biggest wins are, in order: (1) cut or merge implement's serial per-task
sub-agents, (2) stop blocking questions by default, (3) move bookkeeping into CLI
verbs and have `goto` return the current task inline, and (4) let the CLI assemble
documents. Details and estimates are under [Recommendations](#recommendations).

---

## Data and method

| Source | What it gave |
|---|---|
| `~/.claude/projects/-home-nicj-code-github-com-hivecommons-spektacular/*.jsonl`: 32 main sessions and 172 sub-agent transcripts | Timestamps of every request, message, tool call and tool result; token usage per API call; Claude Code's own `cost-state` totals |
| `.spektacular/debug/*.jsonl`: 36 session logs | Every `spektacular` call, with arguments, response size and step transitions |
| Harbor: fresh `p24-claude-*` runs (spec, plan, implement, goal-check, epic, implement-epic) on Sonnet 5.5, compared with yesterday's `s55-*` (Sonnet 5.5) and `m70-*` (Sonnet 4.6) | Controlled, repeatable step timings, using `tests/harbor/shared/report.py` |

**How the time was split.** Each session was cut into workflow steps at every
`spektacular {spec|plan|implement} {new|goto}` call. Each gap between consecutive
transcript events was then labelled by what happened just before it:

- **model**: waiting for the API;
- **tool**: a Bash or other tool running;
- **sub-agent wait**: an `Agent`/`Task` call, or waiting for a background child's
  "Another Claude session sent a message";
- **human**: an end-of-turn followed by the next prompt, or an `AskUserQuestion`.
  Gaps over 30 minutes are counted separately as "away", not as waiting.

Token usage was taken once per API message.

**Caveats.**

- Older transcripts have no `api-request` records, so some model latency shows
  up as "harness". Treat model plus harness as the main thread's model time.
- Agents chain several `goto`s into one shell call, which blurs step
  boundaries. For example, coding often lands in `analyze` or `update_plan`.
- Real sessions often mix a workflow with side conversations, especially in
  implement `new`/`finished` and plan `new`.
- Harbor runs are tiny toy projects with scripted answers. They are good for
  overhead and regressions, but they understate real per-task cost.

The analysis scripts are in the session scratchpad
(`/tmp/claude-1000/-home-nicj-code-github-com-hivecommons-spektacular/7749e883-49c1-4cde-8a8c-c657b9a71dbe/scratchpad/`:
`analyze.py`, `summarize.py`, `latency.py`, `outmix.py`, `results.py`). The
scratchpad is temporary, so ask if you want them kept.

---

## Findings

### 1. Where the wall time goes, per workflow (real sessions, main thread)

Average minutes per run (8–12 runs of each kind):

| | model | harness¹ | tool | sub-agent wait | **human** | API calls / run |
|---|---:|---:|---:|---:|---:|---:|
| **spec** (12) | 2.6 (12%) | 1.5 (7%) | 0.2 (1%) | 1.3 (6%) | **16.5 (75%)** | 28 |
| **plan** (8) | 12.2 (29%) | 4.9 (12%) | 1.1 (3%) | 6.6 (16%) | **17.4 (41%)** | 81 |
| **implement** (9) | 13.3 (19%) | 6.6 (10%) | 2.2 (3%) | **27.5 (40%)** | 19.5 (28%) | 142 |

¹ Mostly model latency in transcripts that lack request timestamps. "Away"
gaps (over 30 min) are excluded.

An implement run averaged **11 tasks** (5–17).

### 2. What a turn costs (2,955 Opus 5.5 calls)

| context size | calls | median duration | median output tokens | time to first block |
|---|---:|---:|---:|---:|
| 0–50k | 345 | 4.0 s | 292 | 2.6 s |
| 100–200k | 808 | 5.4 s | 330 | 3.3 s |
| 300–500k | 477 | 7.2 s | 329 | 4.1 s |
| 500k+ | 221 | 8.8 s | 694 | 5.1 s |

A least-squares fit gives **duration ≈ 3.2 s + 0.32 s per 100k context + 7.4 s
per 1k output tokens**. That splits model time into 35% fixed per-turn cost, 7%
context and **58% output generation**. Context size is a cost problem (cache reads
are ~300× output by volume) much more than a latency problem.

Contexts are often large anyway. Spec steps ran at **550–730k tokens**, because
specs are usually started at the end of a long discussion session. There were 4
compactions over the period.

### 3. Implement: serial sub-agents are the biggest single cost

Sub-agent waits are **27.5 min per run**, 40% of wall time. By step, summed over
real runs: `test` 85 min, `verify` 22 min, `update_plan` 10 min (a chained
verify), `implement` 7.5 min, plus epic-lane waits in `new`.

| Sub-agent role (real sessions) | n | median duration | median API calls | median input tokens |
|---|---:|---:|---:|---:|
| test (`follow-test-patterns`) | 60 | 1.6 min | 10 | 624k |
| verify (`verify-implementation`) | 35 | 0.9 min | 8 | 358k |
| research (analyze/discovery) | 25 | 2.7 min | 18 | 1.3M |

From the debug logs, the `test` step has a **median of 84 s per task** (mean 115 s)
across 105 tasks. It's the longest per-task step. Each sub-agent starts cold: it
reloads the system prompt, finds the conventions again and re-reads the code the
main thread already has in context. Then the main thread waits.

`04-test.md` and `05-verify.md` make the sub-agent mandatory ("Writing the tests
in your own context does not complete this step"). That's true even for a
one-line task, and even when verification is just `go test ./...` with output
piped through `tail`.

**The goal check now adds a third serial child per task.** In Harbor
implement-epic (Sonnet 5.5), `check_goal` is **46% of step time** in both today's
run (157 s of 339 s) and yesterday's (251 s of 551 s). Per task, it costs about
4–5 main-thread turns:

1. stage the diff and status;
2. launch the reviewer (background) and end the turn;
3. resume on the notification;
4. re-type the reviewer's JSON into `reply.json` ("write the reply
   **unchanged** … with your own file tool");
5. run `goal record`.

On top of that is the reviewer's own run (5–80 s).

The goal-check Harbor run also shows a waiting bug. After launching the reviewer,
the agent called `ScheduleWakeup(60s, "wait for reviewer")`. The reviewer
finished in 7 s and the workflow was done at 11:17:38, but the headless session
sat until the wakeup fired a no-op turn at 11:19:02. **65% of that run was
idle.**

### 4. Implement: per-task ceremony

Each task walks `analyze → implement → test → verify → check_goal → update_plan →
update_changelog`, and nearly every step re-loads context:

- `02-analyze`, `03-implement`, `04-test`, `05-verify`, `06-update_plan` and
  `07-update_changelog` (twice) each tell the agent to re-read `plan.md` through the
  CLI ("Do not rely on an earlier step's output still being in your context").
  Implement, test and verify re-read `context.md` too. That's **6–8 full-document
  reads per task**. Implement run `000060` made **53 `plan file read … plan`
  calls returning 4.7 M characters** (~1.2 M tokens). Plan run `000071` read its
  own plan 26 times (2.0 M characters).
- `06-update_plan` (tick checkboxes) and `07-update_changelog` (append an entry)
  are specified as: read the whole plan, change it, **stage the whole document
  with the Write tool**, then `plan file write --from`. Agents mostly avoid the
  full re-type by writing ad-hoc Python patch scripts. That still cost **~73k
  output tokens of shell scripts in `update_plan` alone** across 8 runs, plus 2–4
  turns per task.
- `07-update_changelog` also asks the user "continue with the next task, or stop
  for review?" after every task (see §6).
- Each step's instructions arrive fresh on every task. `update_changelog` is 8.2 KB
  and the others are 3.7–4.3 KB, so about 30 KB of instructions per task.

Agents treat most of this as a formality. Medians across 106 real tasks: `analyze`
23 s, `implement` **9 s**, `verify` 17 s, `update_plan` **0 s**, `update_changelog`
**0 s**. The code gets written before advancing past `analyze`, and the
bookkeeping steps are chained in one shell call. The step machine adds turns and
re-reads, but it doesn't enforce the order it describes.

### 5. Plan and spec: the model does mechanical assembly

- **Plan `assemble`** (`13-assemble.md`) asks the agent to read 12 working files
  and place each under the right heading of three scaffolds. It also asks for two
  extra round trips to fetch skills (`gather-project-metadata`,
  `determine-feature-slug`). The result is staged with the Write tool. Then come
  `verification`, `write_plan`, `write_context` and `write_research` as four more
  steps. In real runs, agents built the documents with Python string assembly
  (73k characters of shell in `plan/assemble`), re-typing the Overview and other
  prose as string literals.
- **Plan `tasks`** writes the task content twice: `tasks_plan.md` (~24k tokens
  over 7 runs) and `tasks_context.md` (~40k tokens). It was 17.7 min of model
  time over 7 runs. Most of that is genuine content, but every word is re-read
  later many times (§4).
- **Spec `verification`** (`08-verification.md`, 16.7 KB of instructions, the
  largest of any step) also asks the model to assemble the spec from its working
  files. It's 40% of the Harbor spec run, mostly the fresh-eyes reviewer
  sub-agent (17 s).
- **Small section steps add up.** Plan has 19 steps and spec has 12. Each costs at
  least a `goto`, the instruction read, a working-file write and often a
  `working-context.md` refresh. In real runs the eleven small plan section steps
  plus `write_*` were ~2.4 min of model time per plan. That's real but modest:
  plan's main costs are research (`discovery` plus research sub-agents) and the
  walkthrough.

### 6. Blocking questions turn into idle time

Real examples from the transcripts:

- **Plan walkthrough** (`18-walkthrough.md`) is paced as "a small number of natural
  beats with a brief pause between each". In practice the user answers "ok", "ok",
  "ok", "ok", "looks good approved": **four waits per plan**. Walkthrough human
  time was 52 min across 8 runs (plus 297 min "away" in one run). In addition, the
  walkthrough re-reads all three documents, and the Harbor plan suite fails a run
  that skips this.
- **Implement**: "Task `X` is complete. The next task is `Y`. Continue, or stop
  here for review?" after every task. The answer is "ok great, keep going". Plus
  "Advancing will auto-commit everything above — how should I handle it?",
  "Milestone 1 finished… advancing makes a git commit of 79 files", and
  knowledge-entry confirmations mid-run.
- **Spec**: 75% of its time is human, which is natural for an interview. But every
  section step (overview, requirements, acceptance criteria, constraints,
  technical approach, success metrics, non-goals) ends in a confirmation, and so
  does every reviewer finding. After the interview the sections are mostly
  derivable, yet they are confirmed one at a time.

None of these costs model time, but each one stops progress until you come back.
For a run you walk away from, they are the slowness.

### 7. Smaller contributors

- **Instruction volume**: ~430k tokens of `goto` instructions entered context over
  the period, about 8% of all tool-result volume. Even so, the heaviest ones are
  worth trimming: spec `verification` 16.7 KB, plan `discovery` 15 KB, plan
  `assemble` 14.4 KB, spec `technical_approach`/`constraints`/`split` ~11 KB each,
  implement `new` 9.5 KB.
- **`working-context.md` refresh footer** on every step: 255 refreshes (agents skip
  it about two-thirds of the time), ~99k output tokens, about 26 min in total.
- **Tool time** (go test, git, grep) is ~1 h of 18 h. Not worth optimising.

### 8. Fresh Harbor runs (p24, today)

| suite | p24 Sonnet 5.5 (today) | s55 Sonnet 5.5 (yesterday) | m70 Sonnet 4.6 |
|---|---|---|---|
| spec | 1.2 m, 21 turns, $0.37, **48/49 fail** | 1.1 m, $0.26, 48/48 pass (another s55 run: 0.3 m, 9/48) | 9.6 m, $1.67, pass |
| plan | 2.4 m, 31 turns, $0.61, **93/94 fail** | 3.7 m, $0.68, 94/95 fail | 14.1 m, $2.95, pass |
| implement | 0.5 m, 10 turns, $0.14, 14/14 pass | 1.0 m, $0.20, 12/14 fail | 5.5 m, $1.56, pass |
| goal-check (unmet) | 2.1 m (**1.3 m idle**), $0.28, **8/9 fail** | 1.2 m, $0.27, 7/8 fail | 3.7 m, $1.42, pass |
| epic (plan-epic) | **3.7 m**, 13 turns, $1.27, 31/31 pass | 20.8 m, $1.41, 27/27 pass | 16.6 m, $4.86, pass |
| implement-epic | **5.8 m**, 14 turns, $1.75, **54/59** (5 errors) | 11.6 m, $1.71, 54/54 pass | 21.8 m, $8.38, pass |

Observations:

- The epic suites more than halved since yesterday. Plan-epic went from 20.8 to
  3.7 min, with three plan lanes running in parallel at 82–103 s each.
  Implement-epic went from 11.6 to 5.8 min. The 000071 child-agent work clearly
  helped.
- **Failures:**
  - spec: `test_review_was_a_briefed_child`. The reviewer prompt omitted "review
    only what is written".
  - plan: `test_docs_draft_during_walkthrough`. The walkthrough didn't read the
    committed documents back. Faster, but against the template.
  - goal-check: `test_scenario_verdicts`.
  - implement-epic: 5 **verifier errors**, not agent failures. In
    `tests/harbor/shared/transcripts.py:478` (`claude_children`; the same pattern is at line 624), `o.get("message")`
    returns a string for some stream-json objects, and `.get("content")` is then
    called on it: `AttributeError: 'str' object has no attribute 'get'`. All
    `TestChildren` cases error at setup, and the recorder reports `children:
    unknown`. A one-line `isinstance(msg, dict)` guard fixes it.
- **Coverage gap:** the `implement-workflow` suite starts at `test_plan`, and
  `goal-check` starts at `check_goal`. **No single-spec suite exercises the
  per-task analyze → test → verify → check_goal loop**, which is where real
  implement time goes. Only implement-epic touches it, with one-line tasks.
- The report's `idle_s` is the single longest gap, not total idle. It catches a
  stall like the ScheduleWakeup one, but not many small waits.

---

## Recommendations

Ranked by expected wall-clock saving on a real feature (Opus 5.5, about 11
implement tasks). The estimates use the measured cost of 3.2 s per turn and 7.4 s
per 1k output tokens.

### 1. Stop running three serial sub-agents per task (implement), ~15–25 min per run

- **Verify inline.** Verification is running the commands and summarising them.
  Let the main thread run them with output trimmed (`go test ./... 2>&1 | tail
  -20`, or a `--quiet` wrapper) instead of a mandatory sub-agent. That saves about
  1 min per task.
- **Make the test sub-agent depend on complexity.** The plan already records Low,
  Medium or High complexity per task. Write tests inline for Low and Medium. Keep
  the fresh-context sub-agent for High, or batch it per milestone (one test agent
  covering all of a milestone's tasks). That saves about 1 min per task at the
  84 s median.
- **Make the goal check cheaper:**
  - (a) Let the reviewer write its reply to `reply.json` itself (one write-only
    path), or let `goal record` read the reviewer's hand-back directly, so the
    main thread never re-types the JSON.
  - (b) Have the CLI stage the diff and status itself as part of `goto
    check_goal`, rather than via two shell writes.
  - (c) Consider running it per milestone, or **pipelined**: start the reviewer
    in the background and begin `analyze` on the next task, recording the
    verdict when it arrives.
- **Never poll for children.** Add to the implement and epic instructions: "wait
  for the completion notification; do not call `ScheduleWakeup`, `sleep` or
  `Monitor` while a child runs". The implement-epic suite already has
  `test_no_wakeup_while_children_run`. Extend that check to goal-check and
  implement.

### 2. Don't block on questions by default, ~10–20 min of waiting per run, more when unattended

- Implement: **continue to the next task by default**. Make pausing an opt-in
  (`implement.pause_between_tasks`, or "stop after this task" in the request).
  Today that's the orchestrated behaviour; make it the default.
- When `auto_commit` is configured, don't ask before committing. The setting *is*
  the answer.
- Collect knowledge-capture offers and present them **once at the end of the run**
  rather than mid-task (spec 000072, self-written knowledge, already heads that
  way).
- Plan walkthrough: give **one** compact summary (approach, tasks with human
  execution, out of scope, assumptions) and then ask a single "approve or change?".
  Drop the four paced beats. Keep the apply-immediately path for change requests.
- Spec: after the interview, **draft all remaining sections in one pass** and
  review them in one or two turns, rather than seven section-by-section
  confirmations. The interview is where your input matters most. The section steps
  mostly restate it.

### 3. Turn bookkeeping into CLI verbs, and let `goto` carry the current task, ~5–8 min per run

- Add `spektacular plan task done <id> [--criteria 1,3]` and `spektacular plan
  changelog append <name> --from <entry.md>`. The CLI already knows the task IDs
  and the plan structure, and it already merges lifecycle metadata on write. These
  replace the read, patch and write-back cycle (2–4 turns and an ad-hoc script per
  task) with one call each. With the goal check enabled, `update_plan` can be done
  entirely by `goal record`, since it already knows which criteria were met.
- **Have `goto analyze` (and later steps) return the current task's plan block and
  its `context.md` section inline** in the instruction. Then delete the "re-read
  plan.md / context.md" step from analyze, implement, test, verify, update_plan
  and update_changelog. That removes 6–8 full-document reads per task, and the
  context growth that goes with them.
- **Collapse the per-task steps** to something like `work` (analyze + implement +
  test), `verify` (inline), `check_goal`, then `record` (tick plus changelog, by
  CLI). Agents already compress the current seven steps into a few calls. The
  machine should match how the work is really done, which also makes the timings
  meaningful.

### 4. Let the CLI assemble documents, ~1–3 min per plan and fewer steps

- `spektacular plan assemble <name>` should concatenate the working files into the
  three scaffolds, filling in metadata and the slug itself, so neither skill
  needs fetching. The agent supplies only `## Overview`, and only the parts of
  `context.md` that aren't one-to-one copies. Fold `write_plan`, `write_context`
  and `write_research` into that command, or into a single `commit` step.
- Do the same for spec `verification`: `spektacular spec assemble <name>` stages
  `.spektacular/tmp/spec_template.md` from the working files, and the step keeps
  only the reviewer and triage.
- Merge the small plan section steps into two or three groups (for example
  *design*: architecture, components, data structures, implementation detail;
  *delivery*: dependencies, testing, milestones, tasks; *boundaries*: open
  questions, out of scope). Each merge removes at least two turns and one
  instruction payload.

### 5. Run workflows in a clean context, a modest latency gain and a large cost saving

Specs routinely start at 550–730k tokens of context. Latency only rises about 2 s
per turn at that size, but every turn re-reads the whole context from cache, and
cache reads dominate token volume. Two options:

- after `spec new` writes `working-context.md`, recommend `/clear` and resume from
  it; or
- have the skills run plan and implement in a sub-agent briefed with the
  working context.

The orchestrated epic lanes already work this way.

### 6. Trim instructions and footers (low priority)

- Cut spec `verification` (16.7 KB). Move the reviewer brief into the
  sub-agent's own skill or agent definition rather than pasting it into the main
  thread's context every time. Trim plan `discovery`, `assemble` and
  `update_changelog`.
- Only emit the `working-context.md` refresh footer on steps where decisions are
  made (interview, discovery, walkthrough, end of task), not on every mechanical
  step.

### 7. Measure the loop that matters

- Add a single-spec Harbor implement suite with 3–5 real (non-trivial) tasks that
  starts at `read_plan`, so per-task overhead is benchmarked and regressions show.
- Fix the `claude_children` verifier crash (§8), so implement-epic reports its
  children again.
- In `report.py`, report total idle and a per-step split into model, tool,
  sub-agent and human time, as this investigation did. `idle_s` as "longest gap"
  hides many small waits.
- Consider recording `model_time_s` per step in `metrics.json`. The transcript
  already carries request and response timestamps.

---

## Appendix: per-step numbers (real sessions, main thread, summed over runs)

**Implement** (10 runs, 106 tasks). Model minutes / sub-agent-wait minutes / API calls / output tokens:

| step | model | sub-agent wait | calls | output |
|---|---:|---:|---:|---:|
| implement | 19.3 | 7.5 | 163 | 155k |
| verify | 19.0 | 21.8 | 178 | 180k |
| new | 17.0 | 91.8 | 171 | 119k |
| update_plan | 16.6 | 9.8 | 190 | 166k |
| test | 16.1 | 84.6 | 246 | 169k |
| analyze | 12.6 | 2.6 | 119 | 103k |
| update_changelog | 4.3 | 0 | 60 | 40k |
| update_feature_changelog | 3.2 | 0 | 23 | 32k |

(Step labels are approximate because of `goto` chaining. Code edits often land
under `update_plan` or `verify`.)

**Plan** (8 runs). Model minutes / human minutes / output tokens:
`new` 35.0 / 50.2 / 182k · `discovery` 19.4 / 14.5 / 156k · `tasks` 17.7 / 0.2 /
138k · `architecture` 4.9 / 0 / 47k · `walkthrough` 4.4 / 52.3 / 62k · `assemble`
4.1 / 0 / 35k · the eleven other steps 0.1–1.8 each.

**Spec** (12 runs). Model minutes / human minutes:
`verification` 7.4 / 17.9 · `interview` 6.8 / 39.5 · `new` 3.8 / 18.7 · `overview`
1.7 / 37.4 · `split` 2.3 / 16.0 · every other section step under 1.5 model minutes.

**Implement step wall time from debug logs** (106 tasks, gaps over 1 h dropped):
`test` median 84 s, mean 115 s · `analyze` 23 s / 89 s · `update_changelog` 0 s /
58 s · `verify` 17 s / 34 s · `implement` 9 s / 27 s · `update_plan` 0 s / 2 s.
