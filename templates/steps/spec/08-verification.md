## Step {{step}}: {{title}}

### 1. Assemble the spec

Assemble the final spec from the per-section working files you wrote during the earlier steps. Read each section's working file under `.spektacular/work/{{spec_name}}/` and drop its content under the matching `## ` heading in the scaffold below. The working files hold body content only — this scaffold owns the headings and their order.

Each section's HTML comment in the scaffold below also states that section's required format (e.g. Requirements and Acceptance Criteria are checklist items with a bold title and indented detail; Constraints, Technical Approach, Success Metrics, and Non-Goals are one bullet per item). The earlier steps should have already produced working files in this shape — but check each working file against its comment now, as a last check, and reformat before staging if any section drifted into plain paragraphs. Do not change the content or meaning, only its shape.

- `.spektacular/work/{{spec_name}}/overview.md` → `## Overview`
- `.spektacular/work/{{spec_name}}/requirements.md` → `## Requirements`
- `.spektacular/work/{{spec_name}}/constraints.md` → `## Constraints`
- `.spektacular/work/{{spec_name}}/acceptance_criteria.md` → `## Acceptance Criteria`
- `.spektacular/work/{{spec_name}}/technical_approach.md` → `## Technical Approach`
- `.spektacular/work/{{spec_name}}/success_metrics.md` → `## Success Metrics`
- `.spektacular/work/{{spec_name}}/non_goals.md` → `## Non-Goals`

If a working file is missing, that section was never completed — STOP and gather it (re-run the matching step) before assembling.

Stage the assembled spec to the scratch path `.spektacular/tmp/spec_template.md` now (use your own `Write` tool), filling the scaffold below. This staged file is what the reviewer reads in the next step and, once approved, what gets committed to the store.

```markdown
{{spec_template}}
```

### 2. Fresh-eyes review (independent subagent)

**Do not review the spec yourself.** You wrote every section during the interview, so you know what each one was *meant* to say — which means leaks read as fine to you and you cannot reliably audit your own text. The plan workflow downstream receives only this assembled file, with none of the conversation context. So verify it the same way it will actually be consumed: hand it to a reviewer that has never seen the interview.

Spawn a subagent with a **fresh context** (use your Task/Agent tool). Give it exactly two things and nothing else:

- the path to the staged spec, `.spektacular/tmp/spec_template.md`, to read; and
- the reviewer brief below, pasted verbatim.

Tell the subagent explicitly: **review only what is written in that file.** Do not read the working files, `.spektacular/working-context.md`, the conversation, or any other source; do not ask the caller for intent. Judge the spec purely as a naive reader would. Return a flat list of findings — for each: `section`, `quote` (the offending text), `issue_type` (leak / misplaced-hard-rule / duplication / missing-constraint / format / unclear / incomplete), and `suggested_fix`. If nothing is wrong, return an empty list.

> If you have no way to spawn a subagent, fall back to reviewing the staged file yourself — but read **only** `.spektacular/tmp/spec_template.md`, ignore everything you remember from the interview, and apply the brief as strictly as a stranger would.

<details>
<summary><strong>Reviewer brief — paste this to the subagent verbatim</strong></summary>

You are reviewing an assembled feature specification. You have only the file. Judge it exactly as written — you have no other context and must not seek any. Validate it for:

- **Completeness** — every section is filled (none left blank or a placeholder).
- **Clarity** — requirements are specific and testable.
- **Consistency** — sections reference each other appropriately and do not contradict.
- **Section hygiene** — each section stays within its brief, with no implementation mechanism leaking into sections that must stay implementation-free.
- **Format** — Requirements and Acceptance Criteria items are each a checklist line (`- [ ] **Title**`) with detail indented below, not a plain paragraph. Constraints, Technical Approach, Success Metrics, and Non-Goals items are each their own bullet point (`- ...`), not run together as prose. Flag any section written as flowing paragraphs instead of the required list/checklist shape as `issue_type: format`, quoting the offending paragraph, with a suggested fix of splitting it into the correct shape without changing its meaning.

Run the hygiene review in **both directions**:

1. **Mechanism leaking into mechanism-free sections.** Overview, Requirements, and Acceptance Criteria must stay free of implementation mechanism. (Constraints is *not* mechanism-free — a mandated technology like "must use SQLite" is a legitimate hard-rule constraint.) Flag mechanism that has drifted into Overview/Requirements/Acceptance Criteria; the fix is to move it to whichever section owns it — `## Constraints` if it is a hard rule, Technical Approach if it is non-binding direction.
2. **Hard rules sitting in Technical Approach.** Technical Approach holds non-binding *direction* the planner may adapt — never hard rules. Any "must" / "must not" / "cannot" found in Technical Approach — including a mandated technology such as "must use SQLite" — is a constraint in the wrong place: the fix is to move it up to `## Constraints`. Also check whether `## Constraints` is thinner than the spec implies — an empty or sparse Constraints section is not automatically correct (see "Constraints completeness" below).
3. **Duplication across sections.** Each section owns its content — no section should restate what another already captures. A common case: Technical Approach re-listing the constraints ("use an embedded datastore, replace the existing storage, configurable location") that are already in `## Constraints`. Flag any restatement; the fix is to trim it to a one-line reference (e.g. "No technical direction beyond the captured constraints; left for the plan workflow to propose"), keeping the content in its owning section only. Another common case: **Non-Goals restating the inverse of a constraint** — Constraints says "must use SQLite" and Non-Goals says "other datastores are out of scope", or Constraints says "must replace the file storage" and Non-Goals says "no dual-writing the legacy files". The non-goal is redundant because the constraint already implies it; flag it as `duplication` and drop it from Non-Goals. This is not a "deliberate decision to leave as-is" — a constraint stated in reverse is duplication, not a standard pattern.

Per-section tests:

- **Overview** — no file paths, section names, step names, framework/library names, or code identifiers. A non-engineer stakeholder should be able to read it and understand the value.
- **Requirements** — apply the test: *if this were implemented with a completely different mechanism, would the requirement still be true?* If no, it should be rephrased at the behavior level with the mechanism moved to Technical Approach. Warning signs: specific file paths, section or heading names, step or state names, numeric step positions.
- **Constraints** — the hard rules and boundaries the solution must honor, phrased as "must" / "must not" / "cannot". A mandated technology ("must use SQLite"), a required integration, a compatibility guarantee, and a deployment boundary all belong here. A "must use X" does **not** belong in Technical Approach — a binding technology mandate is a constraint.
- **Acceptance Criteria** — each criterion must be observable from outside the implementation. "File X exists after user accepts" is good. "FSM rejects step Y from source state Z" is a unit test of internal plumbing — flag it to be rephrased as an observable outcome.
- **Technical Approach** — non-binding *direction* and preferences to guide the planner, which the planner may adopt, adapt, or replace. Concrete mechanisms, libraries, and file paths are welcome here **as suggestions**, never as hard rules. Anything stated as a hard "must" is a constraint, not technical direction.

**Constraints completeness.** When `## Constraints` is empty or sparse, do not take that at face value — re-read Technical Approach, Requirements, and Overview for boundaries the work must operate within but that were never captured as constraints. Common signals:

- **Existing system to integrate with or replace** — e.g. Technical Approach says *"replace the current file-based storage (monsters.json, users.txt)"*. That implies a real boundary: the work must migrate from / stay compatible with the existing store and not lose existing data. Surface it as a `missing-constraint`.
- **Deployment / runtime boundaries** — e.g. *"no separate server process"*, "single self-contained binary", a fixed runtime or platform. If real, these are constraints.
- **Compatibility guarantees** — anything phrased as "must not break" or "must stay compatible with".

Report each as a `missing-constraint` finding. Do not invent constraints the spec does not support, and allow a genuine "no constraints" outcome — an empty Constraints section can be correct.

Return your findings as the flat list described above, and nothing else.

</details>

### 3. Triage the findings (you, with full context)

Now you — who *did* run the interview — filter the reviewer's findings. The subagent flags everything that reads wrong to a stranger; your job is to decide which flags are real:

- **Real leak, misplacement, duplication, or genuinely-missing constraint the user has not deliberately chosen** → keep it, and propose the fix to the user.
- **Something the user already debated and deliberately locked in** (a design decision they chose to state where it is, or a genuine "no constraints" outcome) → drop it. Do not re-raise a decision the user already made.
- **Clarity/completeness nits the user already accepted as-is** → use judgement; prefer dropping over re-litigating.

Do **not** silently rewrite. For every finding you keep, propose it to the user and wait for confirmation. Use these shapes:

> *"In <section>, item N names <specific implementation detail>. I'd like to rephrase it as <behavior-level version> and move the implementation detail to Technical Approach. OK?"*

> *"Constraints is empty, but Technical Approach says the work replaces the existing monsters.json/users.txt storage. That reads like a constraint (must migrate existing data / stay compatible with the current store). Add it to Constraints? OK?"*

### 4. Apply confirmed fixes

For each fix the user confirms, update the **owning section's working file** under `.spektacular/work/{{spec_name}}/` with your own tools (the working files remain the source of truth and are read back on resume), then re-assemble `.spektacular/tmp/spec_template.md` from the working files so the staged spec reflects the change. If the fixes were substantial, you may re-run the fresh-eyes review (step 2) on the updated file until it comes back clean.

Iterate steps 2–4 until the user is happy and the spec is correct, complete, and free of section-hygiene leaks — implementation leaking up, and real constraints left uncaptured.

### 5. Commit

Once the user is happy, commit the staged spec to the spec store.

**Never edit the spec file with the `Write` or `Edit` tools.** `{{config.command}} spec file write` is the only supported way to write the spec — it routes the write through Spektacular into the configured spec directory.

The approved content is already staged at `.spektacular/tmp/spec_template.md` from the steps above. Point `spec file write` at it with `--from`, then remove the scratch file:

```
{{config.command}} spec file write {{spec_name}} --from .spektacular/tmp/spec_template.md
rm .spektacular/tmp/spec_template.md
```

That writes the final spec into the configured spec directory through Spektacular. The argument is the bare spec name, with no file extension — `spec file write` finds the configured spec directory for you.

After `spec file write` succeeds, verify the stored spec with `{{config.command}} spec file read {{spec_name}}`. Once you have confirmed it is correct, the per-section working files have served their purpose — remove the working directory with your own file tools:

```
rm -rf .spektacular/work/{{spec_name}}
```

Then advance:

```
{{config.command}} spec goto --data '{"step":"{{next_step}}"}'
```
