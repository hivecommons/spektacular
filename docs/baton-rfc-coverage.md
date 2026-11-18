# Spektacular's Workflows Under the baton Model

Issue #95 proposes **baton**, an engine that runs workflows defined in config.
A workflow is built from `task`, `decision` and `gate` nodes, and work may only
loop back when a gate fails. The RFC's first question is whether that model
covers the workflows Spektacular already runs. This document answers it from
the shipped code. It maps each built-in workflow onto baton's nodes and lists
what the model cannot yet express. It then takes a position on the RFC's open
questions, based on the same evidence.

It is feedback on a proposal, not a design Spektacular has adopted. Choosing
between landing options A, B and C is for the maintainers. The section on
those options only sets out what each would cost, given the gaps found here.

## How the engine works today

Every Spektacular workflow is a table of steps run by one engine,
`internal/workflow/workflow.go`. That engine wraps `looplab/fsm`:

- Each step names its legal source states (`Src`) and its destination
  (`Dst`). An agent moves between steps with
  `<command> <kind> goto --data '{"step":"..."}'`. A move the table does not
  allow comes back as `invalid_transition`, and the error lists the moves
  that would work.
- A step callback may return the name of the next step. The engine then
  follows it straight away (`pendingGoto`, `workflow.go:134`). This is how the
  binary picks a route by itself, with no agent involved.
- State is saved to `state.json` on every transition (`enter_state`,
  `workflow.go:151`). Resuming re-renders the current step's instructions
  without moving (`renderStep`, `workflow.go:266`).
- Callbacks are Go, so a step can do real work before it shows any
  instructions. It can create a scaffold, reset the working context, close an
  artifact's metadata, or refuse to finish.

So the binary already owns the state, which is baton's second principle. What
it does not do is record **who** made a move. The agent calls `goto` every
time, including after the user has approved a draft in conversation.

## Workflow by workflow

### spec (11 steps, `internal/steps/spec/steps.go`)

The table runs straight through: `new → interview → overview → … →
verification → finished`.

| Spektacular today | baton equivalent | Fit |
|---|---|---|
| Section steps (`overview` … `non_goals`). The agent drafts and the user confirms in chat, under the standing "Presenting Drafts and Confirmations" rule. | `task` (author = agent). The user's confirmation is either left in the conversation or recorded as a `decision` (reviewer = human). | Fits. Recording all 7 section confirmations would double the node count. |
| `verification`, which iterates "steps 2–4 until the user is happy" (`templates/steps/spec/08-verification.md:93`). A fresh-eyes sub-agent reviews, the user confirms fixes, and the loop repeats. | measure (`find_issues`, reviewer) → `gate` (`len(issues) == 0`) → work (`fix_issues`, author) → measure. | **Strong fit.** This loop exists today only in prompt text. baton would make it a loop in the graph, with an exit you can observe. |
| `finished` checks that the spec is no longer the empty scaffold (`specStillScaffold`, `steps.go:172`), then closes its metadata as final (`metadata.Close`, `steps.go:179`). | A `gate` with a shell rule for the check. **No baton equivalent for the close.** | Partial. See gap 1. |
| `new` writes the scaffold and resets the working context (`steps.go:95`). | **None.** | Gap 1. |

### plan (20 steps, `internal/steps/plan/steps.go`)

This table also runs straight through, from `new` to `finished`. It adds two
features baton has to account for:

- `verification` loops inside the step: "Re-run this verification until every
  section … is present and filled" (`templates/steps/plan/14-verification.md:65`).
  This is the same measure, gate and work loop as in the spec workflow, and it
  fits for the same reason.
- `walkthrough` waits for "an explicit affirmative answer to a direct closing
  question" (`18-walkthrough.md:24`). A change request is edited in place,
  and the workflow stays on `walkthrough`. The engine forbids going back to an
  earlier step and says so in its error message (`walkthroughRevisionHint`,
  `workflow.go:236`). In baton this is a `decision` (reviewer = human) with
  two outcomes, `accept → finished` and `revise → walkthrough`. `revise` is a
  **self-edge from a decision**, which the draft loader would reject. See open
  question 1.

### implement (12 steps, `internal/steps/implement/steps.go`)

This is the only built-in workflow with a real loop. It also has the most
logic for choosing a route.

| Spektacular today | baton equivalent | Fit |
|---|---|---|
| `update_changelog → analyze` while unchecked tasks remain (`analyze` Src includes `update_changelog`, `steps.go:31`). | measure = parse the plan's task checkboxes (`plantask.OpenTasks`). `gate` = `len(open_tasks) == 0`, with fail routing to `analyze`. | **Fits the "loops only from gates" rule exactly.** Each pass ticks one task, so stall detection has a natural signal. |
| In a single-task run the engine picks the exit itself: `test_plan` if this was the last open task, `finished` otherwise (`updateChangelog`, `steps.go:150`). | A `gate` with a condition on run context: `task != "" && len(open_tasks) > 0`. | Fits. Conditions on context cover it. |
| `verify` STOP-on-mismatch. The user chooses one of three outcomes: **fix and re-run** ("Return to the `implement` step"), **accept the partial failure**, or **abandon the task** (`05-verify.md:35-37`). | A `gate` (shell rules = the verification commands). Its fail path goes to a `decision` (human) with the outcomes `fix → implement`, `accept → update_plan` and `abandon → …`. | **Does not fit as drafted.** `fix` is a backward edge from a decision. `accept` is a human overriding a failed gate. See open question 1 and gap 3. |
| Abandoning a task in a whole-plan run means "Skip to the next unchecked task". The skipped task stays unchecked. | The open count does not fall, so stall detection would escalate. | Gap 4. Items need a status besides open and resolved. |
| `finished` refuses to complete without the feature changelog (`changelog_missing`, `steps.go:239`), and closes the test plan and changelog as final. | A `gate` with a file-exists rule for the refusal. **No equivalent for the closes.** | Partial. Gap 1. |

Before any of these steps run, starting a workflow passes through a **start
gate** (`startGate`, `cmd/autocommit.go:200`). If registered repos have
uncommitted changes, the user is asked whether to commit them first, and the
engine makes that commit. In baton that is a human `decision` ahead of the
first node, followed by a side effect the engine performs. Gap 1 again.

### repo add (10 steps, `internal/steps/repo/steps.go`)

This table runs straight through except for one step with several sources:
`placement` can be reached from `name`, `description`, `role` or `tags`
(`steps.go:42`). That is the delegation shortcut. A user who hands over every
value at once skips the remaining questions.

In baton, each of those four steps becomes a `decision` with the outcomes
`next` and `delegate → placement`. Every edge points forward, so the model
covers it, but four `task`s turn into four `decision`s just to allow a
skip-ahead. A `task` whose `next` can name outcomes, forward-only, would keep
this compact.

`repo add` is also deliberately excluded from the single workflow slot that
spec, plan and implement compete for (`steps.go:6-8`). baton already gives
each run its own state, but it has no way to say that two workflows exclude
each other. See gap 5.

## Answer to the RFC's first question

**Mostly yes.** Every route in the four workflows can be drawn with `task`,
`decision` and `gate`. The model also improves on today's engine in one
important way. The convergence loops in spec and plan `verification` live
only in prompt text, so the binary cannot see them. baton would put them in
the graph and give each one an exit condition that can be observed.

These gaps would have to close before Spektacular could run on baton without
losing behaviour:

1. **Engine side effects.** Steps today create scaffolds, reset the working
   context, close artifact metadata as final, and make git commits (on
   start, at transitions and at milestones). baton has gates, which check
   things, but no node that *does* things. Two ways to add one: an
   `on_enter` / `on_pass` command hook on nodes, or a `system` participant
   that runs `task` nodes. A `system` participant sits outside the four-eyes
   rule, because it produces nothing that needs judging.
2. **Backward routes chosen by a human.** These are `verify → fix →
   implement` and the walkthrough's `revise`. See open question 1.
3. **Human override of a failed gate.** `verify`'s "accept the partial
   failure" needs a recorded override, with the reason, by a participant who
   did not write the failing work. That respects four eyes, but baton has no
   override outcome today.
4. **Item statuses beyond open and resolved.** An abandoned task is neither.
   If stall detection only counts open items, it flags a loop the human has
   just chosen to steer. Something like `deferred` or `wont_fix`, excluded
   from the stall count, is needed.
5. **Workflows that exclude each other.** Spektacular allows one active
   spec, plan or implement run, with repo add running alongside. baton
   needs an exclusion group or an equivalent, or Spektacular gives up this
   rule.
6. **Engine-computed template variables.** Instructions are Mustache
   templates fed values the engine works out at runtime: `next_step`,
   `last_task`, the rendered scaffold (`spec_template`) and the selected
   task. baton's prompts would need the same, or the values have to come
   from gate or measure outputs written into the run context.

Two smaller points concern the participant model:

- **The spec reviewer today is a sub-agent.** The fresh-eyes review in spec
  `verification` is an agent checking an agent, inside a run between a human
  and an agent. The MVP's "human + agent only" would make the human the
  reviewer of every node. Alternatively, sub-agents could count as distinct
  participants earlier than the roadmap plans.
- **TTY confirmation and chat-driven humans.** In Spektacular the human
  talks to the agent, and the agent runs the CLI. If every human approval
  needs an interactive TTY, the human has to switch to a terminal at each
  gate. That is acceptable for the few gates that matter: final spec
  approval, the walkthrough close, and a verify override. It would be heavy
  for the roughly twenty section confirmations across spec and plan that today
  are a chat reply.

## Positions on the open questions

These are recommendations, grounded in the mapping above.

1. **Can a `decision` route backwards?** Yes, under a graph rule instead of
   an edge rule: *every cycle must contain a gate*. Under that rule:
   - `verify → fix → implement → test → verify` is legal, because the loop
     re-runs the verification gate.
   - The walkthrough's `revise` self-edge is legal if the walkthrough is
     modelled as a gate with a human approver.
   - An unmeasured loop between two decisions stays illegal.

   The loader can check this rule (look for strongly connected components
   that contain no gate). It keeps principle 4's promise that every loop has
   an exit you can observe.
2. **Expression language.** Use an existing, non-Turing-complete language
   that compiles when the workflow loads, rather than a minimal built-in.
   `google/cel-go` is the stronger choice: it is typed, so the loader can
   reject a condition that names an undeclared tracked item. Spektacular's
   own conditions are tiny (`len(open_tasks) == 0`, `task != ""`), but a
   typo in one should fail at load time, not halfway through a run.
3. **Notifying humans.** A CLI inbox is enough for the MVP. In Spektacular's
   case the human is in the same session as the agent. When `baton next`
   reports that a human holds the baton, the agent stops and tells them.
   Hooks for chat or email can wait for agent + agent runs, where nobody is
   watching.
4. **Where tracked items live.** The artifact should stay the source of
   truth, and run state should keep the measurement history. Spektacular's
   tasks are checkboxes in `plan.md`, read by `plantask.Parse`. Copying them
   into run state would leave two places that can disagree. The measure step
   should write a snapshot of the items into the run, which gives stall
   detection its history, and read the items from the artifact.
5. **More than one active node?** Strictly one for the MVP. Every
   Spektacular workflow has exactly one current state, because the
   `looplab/fsm` engine has no concept of parallel states. None of the
   mappings above needs a second active node.

## What each landing option costs

- **A. Engine inside Spektacular.** Gaps 1 to 6 have to close before any
  built-in workflow moves to config, because all four rely on Go callbacks.
  The four step tables are about 1,100 lines with roughly 2,700 lines of
  tests behind them, and the reimplementation would happen against those
  tests while users depend on them.
- **B. Separate project in hivecommons, adopted over time.** The gap list
  above becomes the checklist for adoption. baton can ship its MVP without
  gaps 1 and 5, and Spektacular can switch over one workflow at a time.
  repo add is the cheapest to start with: it is nearly linear, and its only
  side effect is the registration its `register` step writes.
- **C. Separate and independent.** Spektacular gets nothing from the model,
  and its prompt-only verification loops stay invisible to the binary.

The evidence here favours **B**. It is the only option that lets baton's
model be proven on new workflows before Spektacular's built-in workflows rely
on it. The decision remains with the maintainers.
