## Step {{step}}: {{title}}

Append a phase entry to the inline `{{changelog_section_name}}` section of plan.md, then decide whether to loop back for another phase or advance to the repo-level changelog.

The plan documents are owned by spektacular. **Never read or edit plan.md with the `Read`, `Write`, or `Edit` tools** — go through `{{config.command}} plan file read` and `{{config.command}} plan file write`.

### Step 1: Ensure the `{{changelog_section_name}}` section exists

Read plan.md with `{{config.command}} plan file read {{plan_name}}/plan.md`. If the `{{changelog_section_name}}` heading is absent, this is the first `update_changelog` invocation for this plan — append a new `{{changelog_section_name}}` section **after** the existing `## Out of Scope` section (or at the very end of the file if `## Out of Scope` is missing).

If the `{{changelog_section_name}}` heading is present, append new entries under the existing section, after any entries already there.

### Step 2: Write the phase entry

For the phase you just completed, append an entry with this shape:

```
### <YYYY-MM-DD> — Phase N.M: <phase title>

**What was done**: <1-3 sentences summarizing the code change in plain language>

**Deviations**: <anything that didn't match the plan, or "None" explicitly>

**Files changed**:
- `path/to/file.go`
- `path/to/another/file.go`
- `<repo-name>: path/within/that/repo.go`

**Discoveries**: <anything the next phase or a future maintainer should know — a tricky API, a hidden constraint, a renamed symbol, a missed edge case>
```

In the **Files changed** list, prefix every path with its repo's name (`<repo-name>: path`) whenever more than one repo is registered; an unprefixed path belongs to the only registered repo. The final feature-changelog step derives one entry per affected repo mechanically from these prefixes, so keep them accurate.

For the exact format and more examples, launch a sub-agent with:

```
{{config.command}} skill update-changelog
```

Apply the entry by reading plan.md with `{{config.command}} plan file read {{plan_name}}/plan.md`, adding the entry, staging the updated document with the `Write` tool at the scratch path `.spektacular/tmp/plan_update.md`, then committing it and removing the scratch file:

```
{{config.command}} plan file write {{plan_name}}/plan.md --from .spektacular/tmp/plan_update.md
rm .spektacular/tmp/plan_update.md
```

### Step 2b: Assess discoveries for durable knowledge

Re-read the `**Discoveries**` entry you just wrote and weigh each item for durability — does it hold beyond this one change? Durable means something a future plan or implementation would pay to re-discover: an undocumented coupling, a surprising contract, a convention inferred from behavior rather than written down anywhere, a wrong default that had to be corrected. Only items that are both durable and non-obvious beyond the current change qualify — most phases produce none, and when nothing qualifies, say nothing about knowledge capture and continue straight to Step 3.

When a discovery does clear that bar, offer — in the same message as the Step 3 continue-or-pause conversation — to save it to the project knowledge base. Name what you would capture and why it is worth keeping, phrased as the general lesson rather than the specific edit. Capture happens only on the user's explicit acceptance: hand an accepted item to the `spek-knowledge` skill, whose own propose-then-confirm flow owns tier and store selection, category routing, and the write (the raw `{{config.command}} knowledge` commands sit beneath it). Never invoke `{{config.command}} knowledge write` without that explicit acceptance — silence or deflection is not acceptance.

If the user declines, the item is not offered again for the remainder of the conversation — a decline is final for that discovery. If the user defers ("not now", "later"), you may raise the offer again later in the conversation. Either way, the `**Discoveries**` entry itself stays exactly as written; the offer is additive on top of it.

### Step 3: Check for remaining unchecked phases

Re-read plan.md with `{{config.command}} plan file read {{plan_name}}/plan.md` and count `#### - [ ] Phase` (unchecked) headings under `## Milestones & Phases`.

**If unchecked phases remain**:

- By default, ask the user whether to continue with the next phase or pause here. Example prompt: "Phase N.M is complete. The next phase is `Phase N.(M+1): <title>`. Continue, or stop here for review?"
- If the user has previously said "run without asking" (or equivalent autonomous mode), skip the prompt and loop automatically.
- To loop, advance to `analyze` — this uses the multi-source FSM transition that lets `analyze` be reached from `update_changelog`:

  ```
  {{config.command}} implement goto --data '{"step":"analyze"}'
  ```

**If no unchecked phases remain**:

- This was the last phase. Advance to `test_plan` to write the manual test plan; the feature-changelog step that follows it writes the project record and one user-facing record per affected repo:

  ```
  {{config.command}} implement goto --data '{"step":"test_plan"}'
  ```

### STOP-on-mismatch

If the plan file's state after Step 2 is inconsistent (e.g. you just wrote a changelog entry for Phase N.M but that phase's checkbox is still unchecked after the previous `update_plan` step), STOP and report the inconsistency to the user.
