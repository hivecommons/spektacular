## Step {{step}}: {{title}}

Draft the **Testing Approach** section of `plan.md`.

### This section is high-level only

Describe the overall testing strategy and test types. This section is **high-level only**. Per-task testing detail — which specific tests live in which specific files — stays in the plan's `context.md`.

If you find yourself writing "a test in file X asserts Y on line Z", stop and move that content to the plan's `context.md`.

### What to include

- The kinds of tests being added (unit, integration, contract, regression, end-to-end)
- Which components get the most coverage and why
- The load-bearing assertions — what, in plain language, the tests guarantee
- Where tests slot into existing test conventions in the project
- Any deliberate gaps (e.g. "not adding integration tests because the contract is exercised by unit tests")

### Account for the spec's success metrics

Walk every metric in the spec's **Success Metrics** section and make each one verifiable — do not let any metric drop. For each, state in this section how it will be checked:

- **Behavioural test** — when the metric can be asserted automatically (e.g. "responds within 100ms" → a latency assertion), say so and describe what the test guarantees. The implementer writes the actual test.
- **Manual — captured in the implementation test plan** — when the metric cannot be expressed as an automated behavioural test (load under real infrastructure, manual observation, production telemetry), flag it with exactly that phrase. The implement workflow produces a concrete test-plan artifact for these once the code exists, so do **not** write the procedure here — just classify the metric.

### Account for manual reviews

List every review or check a person must make before the work counts as accepted: a design or UX review, a sign-off, checking behaviour by hand. Flag each with the same phrase, **Manual — captured in the implementation test plan**, and say in one line what is checked. These are never plan tasks: a review left as a task is never ticked. If the work needs none, say so.

If the spec has no success metrics, note that there are none to verify. This metric→verification mapping is the handoff the implementer relies on: the implement workflow consumes the plan, not the spec, so a metric not carried here is invisible downstream.

### What NOT to include

- Specific test file paths
- Per-task test lists
- Shell commands to run the tests

### What to produce

A draft Testing Approach section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Testing Approach** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/testing_approach.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

{{> partials/proceed-unless-blocked}}

Once the drafted testing strategy is saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
