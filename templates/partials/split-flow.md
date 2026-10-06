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
{{command}} epic split --from .spektacular/tmp/epic_split.json
rm .spektacular/tmp/epic_split.json
```

Run `{{command}} epic split --schema` for the full shape. In outline:

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

**5. Afterwards.** Tell the user the epic's name and each resulting spec, and that each can now be planned on its own (`{{command}} plan new`). Run `{{command}} status <epic>` to show them where the epic stands. If the split spec already had a plan, warn that the plan is now stale, because the spec it was made from has changed.

**On a decline,** nothing is written: no epic and no new specs. Continue as if no split had been offered.
