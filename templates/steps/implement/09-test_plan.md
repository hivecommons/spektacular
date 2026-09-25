## Step {{step}}: {{title}}

Produce the **test plan** artifact for the success metrics that could not be covered by an automated behavioural test. This step runs last, after every task is implemented and verified, so the procedures you write reference the **real** implementation — actual endpoints, commands, file paths, and thresholds — not guesses from planning time.

### Step 1: Find the metrics that need manual verification

Read the plan's Testing Approach through the plan store — never with the `Read` tool:

```
{{config.command}} plan file read {{plan_name}} plan
```

In the `## Testing Approach` section the planner classified each of the spec's success metrics as either **covered by a behavioural test** or **manual — to be captured in the implementation test plan**. Collect the manual ones. Also include any metric the plan expected to be automatable but that proved otherwise while implementing (you noted these during the `test` step).

If there are none — every success metric is covered by an automated behavioural test — you still write the artifact, with an explicit "none required" state (Step 3).

### Step 2: Write a concrete, grounded procedure for each

For every manual metric, write a procedure someone else could follow without reading the code. Ground each one in what you actually built:

- **What to measure** — the metric and its threshold (e.g. "p99 latency < 100ms", "≥ 1000 req/s").
- **How** — the exact command, endpoint, or tool against the real implementation (e.g. `hey -n 10000 -c 50 http://localhost:8080/monsters/{name}`), including any setup.
- **Expected result** — the pass condition, stated as a number to compare against.
- **Who / when** — who runs it and at what point (e.g. pre-release, against staging).

Do not invent metrics the plan does not list, and do not restate metrics already covered by automated tests — those live in the test suite, not here.

### Step 3: Write the artifact to the plan store

The test plan is a plan-store artifact. **Never write it with the `Write`/`Edit` tools directly into the plan directory** — stage it, then commit it through the CLI:

1. Use the `Write` tool to stage the content at the scratch path `.spektacular/tmp/test-plan.md`. When manual metrics exist, list one procedure per metric. When none exist, write a single explicit line: `All success metrics are covered by automated behavioural tests; no manual test plan is required.`
2. Commit it and remove the scratch file:

   ```
   {{config.command}} plan file write {{plan_name}} test-plan --from .spektacular/tmp/test-plan.md
   rm .spektacular/tmp/test-plan.md
   ```

After the write succeeds, confirm it with `{{config.command}} plan file read {{plan_name}} test-plan`.

### STOP-on-mismatch

If the plan's `## Testing Approach` section does not mention success metrics at all (an older plan produced before metrics were carried through), STOP and report it to the user: ask whether to (a) write the test plan from the spec's success metrics directly, (b) skip the test plan for this run, or (c) abandon. Do not silently invent a test plan from nothing.

### Advance

Once the test plan artifact is written (or the explicit "none required" state is recorded):

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
