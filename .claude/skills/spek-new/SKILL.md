---
name: spek-new
description: Create a new Specification for a feature, from a conversation or from existing material such as an issue, a tracker epic, a design document, a file, a web page or pasted text ("spec from #45", "start a spec from LIN-123", a pasted link), or split an already-written spec into an epic.
---

> **Version check first.** Before running any other command, run `spektacular version check`.
> - On `status: "match"`, continue with the skill and produce no version-related output.
> - On `"mismatch"`, `"missing"` or `"upgrade_needed"`, the project's settings or installed Spektacular files are out of date: relay the response's `action` message to the user, ask them to run `spektacular migrate` (they can preview it with `spektacular migrate --dry-run`), and wait for their decision before continuing.
> - On `"unsupported_format"`, relay the `action` message: the project was written by a newer Spektacular, which the user must install before continuing.
> - Never run `migrate` or `init`, and never modify installed files yourself. Upgrading is always an explicit, user-initiated action.

> **STOP. Read this before running any command below.**
> A single successful CLI call — including the very first `spec new` — is **NOT** task completion. It is not a milestone to report back to the user. It is one step out of many in a workflow that you must keep driving, turn after turn, without stopping, until the CLI itself tells you the workflow is *finished*. If you find yourself about to say "successfully completed" or summarize results after calling `spec new` or `spec goto` even once, you are wrong — go back and read the `instruction` field you just received, do what it says, and call `goto` again.

# What this skill does

This skill drives a **multi-step interactive workflow** that produces a complete specification file at the `spec_path` returned by the CLI. The workflow is owned by the `spektacular` CLI, not by you — the CLI is the state machine and you are the executor.

On each turn, the CLI returns JSON containing an `instruction` field. That instruction describes exactly one step (e.g. overview, requirements, acceptance criteria, …). You must:

1. Read the `instruction` carefully.
2. Perform the step — usually this means interviewing the user and capturing their answers. Some steps tell you to commit the gathered content to the spec file.
3. When the step is complete, run the `goto` command named at the bottom of the instruction to advance the state machine.
4. Read the next `instruction` from the new JSON response and repeat.

**This is a loop. Do not stop after the first step.** Keep looping — step → goto → next instruction → step — until a returned instruction tells you the workflow is *finished*. Only then should you report completion to the user.

**Concretely: do not stop after `spec new`.** That command only starts the workflow — it returns the *first* instruction (the `interview` step), not a finished spec. Seeing a clean JSON response with no `error` is not a signal to stop; it is the signal to keep going. Reporting success, summarizing "spec initialized," or handing control back to the user at this point is the single most common way this skill is executed incorrectly — do not do it.

# The interview step

Before any section is drafted, the workflow opens with an `interview` step: a single open-ended conversation, not a fixed script. You ask adaptive questions about what's being built, who it's for, and what constraints apply, following up on what the user has already said rather than working through a predetermined list. You have the project's full registered-repo roster available during this step — if the project spans more than one repo and the feature reads as focused on one of them, ask whether it also needs changes in another registered repo, shaped by what that other repo actually is (for example, a documentation repo invites asking whether docs need updating). Stop the interview once further questions wouldn't materially change the draft, not once every conceivable detail has been asked about — this should take a small number of exchanges, not an exhaustive back-and-forth. Save your synthesized understanding (not a transcript) to `.spektacular/work/<spec_name>/interview.md` with your own `Write` tool before advancing; every later section step drafts from this file and presents its draft back for confirmation, rather than asking its own scripted question from a blank prompt. A session interrupted mid-interview resumes on the `interview` step itself — read back `.spektacular/work/<spec_name>/interview.md` (if partially written) and `.spektacular/working-context.md` before continuing the conversation.

# Reading and writing the spec file

The CLI owns the spec file. All spec file access goes through `spektacular spec file`:

- `spektacular spec file read <name>` — read a spec file from the spec store.
- `spektacular spec file write <name> --from <source-path>` — write a spec file into the spec store from a source file on disk. Stage the body under `.spektacular/tmp/` first, then `rm` the scratch file after a successful write.
- `spektacular spec file list` — list spec files in the spec store.

Path arguments are spec file names; `spec file` resolves them against the configured spec directory itself.

# Design documents a spec may reference

A spec does not have to absorb a worked design. Where the conversation settles an API's shape, a
user-facing flow, a data format, or a worked example of one, that detail can live in a **design
document** in one of the project's declared design sources, with the spec carrying a reference to
it instead of its content. The technical-approach step makes that offer at the point the detail
would otherwise be compressed to a one-line steer; it is always an offer, and nothing is written
without the user's explicit agreement.

A design does not have to already exist to be captured. Where it has been settled in
conversation but never written down, the `spek-design` skill runs a guided interview and writes
the document from it; where the user already has the document, it is stored exactly as supplied.

Design documents are reached through the CLI, never by reading files directly:

- `spektacular design sources` — the design sources this project declares, with their locations.
- `spektacular design list` — the design documents in them.
- `spektacular design write --data '{"source":"<name>","path":"<path>"}' --from <file>` — store a
  design document Spektacular did not author, byte for byte, adding no frontmatter to it and
  reformatting nothing.
- `spektacular design author --data '{"source":"<name>","path":"<path>"}' --from <file>` — store a
  design Spektacular wrote with the user, stamping the same lifecycle record every spec and plan
  carries. Optionally `--spec <spec>` to record which spec's conversation produced it, and
  `--document-status <draft|final|superseded|archived>`. Rewriting an authored design this way
  keeps its original capture date and the specs already referencing it.
- `spektacular design ref add --data '{"spec":"<spec>","source":"<name>","path":"<path>"}'` —
  record the reference on the spec. A reference naming a source the project has not declared is
  refused and nothing is recorded.

# Working files vs. the store document

While you gather each section, write that section's agreed content directly to its own git-tracked working file under `.spektacular/work/<spec_name>/<section>.md` using your own `Write` tool. These working files are **not** store documents — writing them directly with `Write` is correct and expected, and is the one deliberate exception to the "never use `Write`/`Edit`" rule above. That rule protects only the **final assembled** spec, which is written solely through `spektacular spec file write`. The per-section working files are scratch-but-durable: the verification step reads them back to assemble the final spec, and then the working directory is removed once the store write succeeds.

`.spektacular/working-context.md` has a narrower role: it holds only your cross-cutting learnings and the answers the user gave to your questions — never a copy of section content (that lives in the per-section working files). On resume, read back **both** the section working files in `.spektacular/work/<spec_name>/` and `.spektacular/working-context.md`, so you continue from the interrupted step without re-asking for sections already completed.

# How to start

**First, check whether a workflow is already in progress — before asking the user for a spec name.** Run the new command with no `--data`:

```
spektacular spec new
```

This reads the project's single workflow state and changes nothing on disk. One of two things comes back:

- **A resume report** — a JSON object with `"resumable": true` plus the in-progress workflow's `kind`, `name`, and `current_step`, and an `instruction` field. A workflow was interrupted and is still in progress. Do **not** prompt for a spec name — the in-progress workflow already has one. Handle it under "Resuming an in-progress workflow" below. (It may be a *different* kind — a plan or implement run left open.)
- **An error that a name is required** — no workflow is in progress, so there is nothing to resume. Proceed to "Starting a new spec" below.

## Starting a new spec

Only once you know there is no workflow to resume:

**Does the request point at existing material?** If the user refers to an issue, a tracker epic, a design document, a file, a web page or pasted text to start from, however they word it, follow "Starting from existing material" below before choosing a name.

**Does the spec belong to an epic?** Run `spektacular epic list`. If it lists any epics, ask the user whether this spec belongs to one of them, unless the request already says. If it does, pass that epic as `"epic"` on `spec new` below; the spec joins it from the start, and the interview reads the epic and its specs first. If the list is empty, the project has no epics: do not ask.

Ask the user for a spec name now. If the user needs to see what names already exist to avoid collisions, run `spektacular spec file list` — **do not** use `ls`, `find`, or the `Read` tool against `.spektacular/specs/`; the CLI's list is the source of truth for what counts as a spec. Then run:

```
spektacular spec new --data '{"name": "<spec_name>"}'
```

**Only when `spec.id_method` is `external`** (check `.spektacular/config.yaml`), an external system's identifier must be supplied with:

```
spektacular spec new --data '{"name": "<spec_name>", "id": "<external_id>"}'
```

Under `timestamp` or `counter` (the default is `timestamp`), **never pass `id`** — not even when the spec comes from a GitHub issue or ticket with its own number. The CLI mints the ID itself and rejects an explicit one, because a name without the configured ID prefix would be refused by every later plan and changelog write.

Add `"sources"` and `"epic"` to the same `--data` when they apply (see the sections below), for example:

```
spektacular spec new --data '{"name": "<spec_name>", "sources": [{"uri": "<stable link>"}], "epic": "<epic name>"}'
```

**If the epic is complete** (`code: epic_complete`), every spec in it is already implemented. Tell the user, and ask whether to add the spec anyway; adding it reopens the epic until the new spec is implemented. Only if they agree, re-run the same command with `"confirm_completed_epic": true` added to `--data`. Never add it without asking.

The CLI may normalize and prefix the requested name. Always use the returned `spec_name` and `spec_path` as the source of truth for follow-up workflows.

The command creates the spec file and state file automatically and returns the first `instruction`. From that point on, follow the loop above: do what the instruction says, then call `spektacular spec goto --data '{"step":"<next_step>"}'` to get the next one. Do not invent step names — every instruction tells you the exact `goto` command to run next.

### If the project has uncommitted changes

When the project sets `auto_commit` to `workflow` or `full`, the named `spec new` above may instead return an **uncommitted-changes report** (`code: uncommitted_changes`) and change nothing on disk. Its `message` names every registered repository holding uncommitted work, and `resource` lists their names.

This is a question for the user, not a decision for you. Tell them which repositories have uncommitted changes and ask whether to git commit that work **before** the spec workflow starts. Then re-run the same command with their answer:

To commit the existing changes first:

```
spektacular spec new --data '{"name": "<spec_name>", "commit_existing": true}'
```

To start without committing them:

```
spektacular spec new --data '{"name": "<spec_name>", "commit_existing": false}'
```

- `true` commits the existing changes first, in their own commit whose message says they are the user's work from before the workflow. The workflow then starts on a clean tree.
- `false` starts the workflow without committing, so the workflow's own automatic commits will include that work alongside the agent's.

Never choose for the user, and never guess from context which they would want — the whole point of the report is that their uncommitted work is about to be swept into a commit they did not make. If the commit fails (`code: auto_commit_failed`), tell them which repository failed and the reason git gave; the workflow has not started.

## Starting from existing material

A spec can start from material the team has already written. Recognise a source however the request is phrased: "spec from issue 45", "#45", "start from LIN-123", a pasted link to any tracker, a design document's name, a file path, or pasted text the user says to start from. Being understood must not depend on careful wording.

1. **Identify the source.** An issue or epic from any tracker, a design document, a file, a web page, or pasted text. If it is ambiguous, such as a bare issue number with no tracker or repository, ask the user rather than guess.
2. **Fetch it with your own tools.** Spektacular prescribes no tool and no tracker: a CLI, an MCP server, a web fetch or a file read are all fine. Whatever the tool, collect the title and body, the discussion (comments, since decisions are often made there), its child items (sub-issues, linked children, task-list entries), and a stable link to record as provenance. For pasted text, the user's message is the content; record a link only if one exists.
3. **If you cannot reach it,** because no tool you have can read it or fetching fails, say so plainly and ask the user to paste the content. Never fall back to a blank interview without saying so.
4. **Check for child items.** A source that already has child items is a set of pieces of work, not one spec. Offer to set up an epic for it instead, with one spec per child (see "A source with child items" below). On decline, continue with a single spec.
5. **Propose a name** from the source's title, and let the user confirm or change it.
6. **Start the workflow with provenance:** `spec new` with `"sources": [{"uri": "<stable link>"}]` (several entries when the spec draws on several sources). The CLI stamps each source's retrieval date. The interview step then seeds every section it can from the material, lists the gaps, and asks only about those.

A design document the spec must be built to is also recorded as a normal design reference; `sources` records only where content came from.

### A source with child items

When the user accepts an epic for a source with child items:

1. **Create the epic first,** with the parent as its source and no specs yet. Stage its body (a `## Overview` drawn from the parent, and an empty `## Specs` section) under `.spektacular/tmp/`, then:

   ```
   spektacular epic write <short-name> --from .spektacular/tmp/epic_body.md --data '{"specs": [], "sources": [{"uri": "<parent link>"}]}'
   rm .spektacular/tmp/epic_body.md
   ```

   A bare name is given an ID; use the returned `name` from here on.
2. **Specify the first child** as its own spec in that epic, seeded from the child:

   ```
   spektacular spec new --data '{"name": "<child name>", "sources": [{"uri": "<child link>"}], "epic": "<epic name>"}'
   ```

3. When that spec finishes, the workflow offers the next child that has no spec yet. The user can stop at any point and continue later.

**A child item that appears later.** When the user brings up a new child item for a source whose epic already exists, or you see one while revisiting the source, offer to start a spec for it in that epic with the same `spec new` command.

## Splitting a spec that is already written

The user can ask for a split at any time, on any spec, including one already written with no workflow running. Read it first with `spektacular spec file read <name>`, and check whether it already belongs to an epic (its `epic` frontmatter field). Then apply the same check and flow the spec workflow uses at its `split` step:

### The split check

The check asks one question about a complete spec: **is this more than one independently useful piece of work?** Size alone is not the test. It runs once, when a spec is complete, and whenever the user explicitly asks for a split. It never runs during open-ended discussion: recognising spec-worthy discussion is a separate behaviour and never offers an epic.

**Read the sensitivity first.** Read `epic_split_threshold` from `.spektacular/config.yaml` now, at the moment you are deciding, not from memory: the user may have changed it. Treat a missing or absent value as `"moderate"`. This setting is separate from `spec_trigger_threshold`; never use one in place of the other. It moves only the gate and the number of signals needed, as below. It never overrides the supporting-work rule or the counter-signals.

**The gate.** Offer a split only if you can name at least two specs, each with its own overview and at least one acceptance criterion that can be verified without the others. If you cannot name them, do not offer, however many signals have fired. This keeps every offer concrete.

**Strong signals.** Any one is enough, provided the gate passes:

- the source the spec was started from lists child items (sub-issues, a task list);
- the requirements fall into groups that could each ship and be useful alone;
- the acceptance criteria cannot all be verified by one change;
- the user's own wording phases the work ("phase 1", "first … then later", "v1 is just …").

**Weak signals.** At least two together are needed:

- more than about seven requirements;
- more than one design document needed;
- an interview that did not converge, where each answer opened new areas;
- a section draft that kept growing content belonging to a different concern.

**How `epic_split_threshold` moves the check:**

- `"strict"` — offer only on a strong signal, and only when each proposed spec could ship on its own today. Weak signals alone never lead to an offer.
- `"moderate"` — the default: one strong signal, or two weak signals together.
- `"lenient"` — one strong signal, or a single weak signal, is enough.

The gate applies at every level.

**Supporting work never counts.** Docs, tests, migrations, config, changelog entries, and skill or template updates that describe or support the same change belong in the same spec. Touching several repos or several surfaces is not a signal in itself; it matters only when a part would be useful on its own, such as a new tutorial unrelated to the code change. Code plus its docs is one spec, at every sensitivity.

**Counter-signals.** These suppress the offer even when signals have fired, at every sensitivity:

- the requirements are tightly coupled, so neither part is useful or testable alone;
- several surfaces or repos serve one capability, or one atomic change.

**Offer, never act.** When the check passes, offer — never split on your own:

> "This sounds like more than one spec. Split it into an epic with these N specs?"

followed by each proposed spec with its one-line scope. Wait for the user's decision. If the check does not pass and the user asked for a split, say plainly why it cannot be split (for example, "the two parts can't be verified separately"), rather than staying silent.

On a decline, continue exactly as if no split had been offered. Do not repeat the offer unless the scope visibly grows after the decline: a new strong signal, or a new independent requirement group. Re-wording what was already there does not count.

### The split flow

A split always acts on a complete spec. Every requirement, criterion and constraint is already written and agreed, so a split redistributes that content into several complete specs in one operation. No resulting spec needs another interview.

Never take any step below without the user's explicit agreement to the split.

**1. Agree the split with the user.** Agree, item by item:

- the list of specs: the spec being split keeps its name and becomes the epic's first spec, narrowed to its own part; each new spec gets a short title;
- the dependencies between them: spec B depends on spec A when B cannot be implemented until A has been (writing and planning are never held back);
- where every piece of content goes:
  - every requirement and every acceptance criterion goes to **exactly one** spec;
  - a constraint or non-goal that applies to several specs is **copied into each** of them, so every spec stays complete on its own;
  - an acceptance criterion that can only be verified once several specs are in place belongs to the spec that completes it: the one that depends on the others;
  - the overall overview moves to the epic, and each spec gets its own overview.

Each spec must leave with its own testable acceptance criteria. Where one is thin, fill it in with the user now; this is what fixes the thin-criteria problem a split exists for.

**2. One review over every resulting spec.** Spawn one subagent with a fresh context (use your Task/Agent tool) and hand it every resulting spec, each rendered in full with every section, in one staged file. Brief it exactly as verification briefs its reviewer: review only what is written, and check each spec for completeness, clarity, consistency, section hygiene and format, plus two split-specific checks: no requirement or acceptance criterion appears in more than one spec, and every spec has a non-empty overview and at least one acceptance criterion verifiable on its own. Triage the findings with the user and apply only the fixes they confirm. If you have no way to spawn a subagent, review the staged file yourself as a stranger would.

**3. Provenance.** The sources the spec was started from move to the epic, because they described the whole request. A source that was only about the narrowed first spec stays on it. A new spec carries a source only when something seeded it directly (its own sub-issue, or material brought in for that spec alone).

**4. Stage the description and run `epic split`.** Write one JSON description under `.spektacular/tmp/` with your own `Write` tool, then run the split and remove the scratch file:

```
spektacular epic split --from .spektacular/tmp/epic_split.json
rm .spektacular/tmp/epic_split.json
```

Run `spektacular epic split --schema` for the full shape. In outline:

```json
{
  "spec": "<the spec being split>",
  "overview": "<the epic's overall overview>",
  "sources": [{"uri": "<a source moving to the epic>"}],
  "specs": [
    {"name": "<the spec being split>", "depends_on": [],
     "body": {"overview": "…", "requirements": ["…"], "acceptance_criteria": ["…"],
              "constraints": ["…"], "technical_approach": ["…"], "success_metrics": ["…"],
              "non_goals": ["…"]}},
    {"title": "<new-spec-title>", "depends_on": ["<the spec being split>"], "scope": "<one line>",
     "body": { "…every section, as above…": "" }}
  ]
}
```

Each list item is one requirement, criterion or bullet. For a requirement or criterion, put the bold title on the first line and the detail on the lines after it. A new spec's `depends_on` may name another new spec by its title; the CLI allocates every new spec's name, rewrites those titles, writes each spec complete and `final`, narrows the split spec, writes or extends the epic, and links them all. If anything fails part-way it restores every document, so a refusal can simply be fixed and re-run.

Splitting a spec that already belongs to an epic adds the new specs to that same epic; epics never nest. If that epic is complete (`code: epic_complete`, every spec in it implemented), tell the user and ask whether to add to it anyway, which reopens it; only if they agree, add `"confirm_completed_epic": true` to the staged description and run `epic split` again.

**5. Afterwards.** Tell the user the epic's name and each resulting spec, and that each can now be planned on its own (`spektacular plan new`). Run `spektacular status <epic>` to show them where the epic stands. If the split spec already had a plan, warn that the plan is now stale, because the spec it was made from has changed.

**On a decline,** nothing is written: no epic and no new specs. Continue as if no split had been offered.

## Resuming an in-progress workflow

When the in-progress check above returns a resume report:

**First check the report's `kind`.** If it is **not** `spec`, a *different* workflow (a plan or implement run) is in progress — you cannot resume it from the spec skill, and the CLI will refuse to. Do **not** run a `spec goto`. Instead follow the report's `instruction`: tell the user a `<kind>` workflow is in progress and let them choose — continue it with that workflow's skill (`spektacular <kind> goto`), or discard it and start the spec with `spektacular spec new --force`. Only proceed with the steps below when the report's `kind` is `spec`.

1. Ask the user whether to **resume** the in-progress spec or **start a new one**. (The report's `instruction` field restates both options.)
2. **To resume**, first read back the previous session's work with your own file tools: the per-section working files under `.spektacular/work/<name>/` (sections already completed) **and** `.spektacular/working-context.md` (learnings + the user's answers). Then run the resume command using the report's `current_step`:

   ```
   spektacular spec goto --data '{"step":"<current_step>"}'
   ```
3. **To start fresh instead** (discarding the in-progress workflow — it remains recoverable via git), re-run with `--force` and a name:

   ```
   spektacular spec new --force --data '{"name": "<spec_name>"}'
   ```
