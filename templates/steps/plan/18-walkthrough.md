## Step {{step}}: {{title}}

{{^orchestrated}}
All three plan documents are committed to the plan store. Before the workflow can finish, walk the user through the plan and get their explicit sign-off. This review is mandatory — do not offer it as a choice, and do not skip it because the user seems satisfied.

Ground the review in the committed documents. You wrote them in the last three steps, so when their content is in your context, use it as it is. Read them back through the CLI only on a resumed session or after a context compaction:

```
{{config.command}} plan file read {{plan_name}} plan
{{config.command}} plan file read {{plan_name}} context
{{config.command}} plan file read {{plan_name}} research
```

Present the plan in **one message**, as a colleague would explain it aloud, not as a recitation of the plan's section text. Keep it compact, a few lines per point:

1. The chosen approach and why it was picked over the alternatives.
2. How the work breaks into tasks under its milestones. Name every task whose `**Execution:**` is `human`, with its reason, so the user knows which work needs a person. If there are none, say so.
3. What was deliberately left out of scope, and why.
4. The drafting assumptions: the judgement calls made while drafting, from the `## Drafting assumptions` section of the plan's `research` document. List every entry, one line each with the decision and its rationale, and invite the user to challenge any of them. A challenged assumption is a change request, handled through the apply-immediately path below.

End that message with a single closing question, such as "Approve the plan, or tell me what to change?". Do not split the walkthrough into several turns, and do not pause for an answer between points.

When the user asks a question or requests a change, answer it directly in your next message. Apply a change request immediately rather than deferring it: edit the affected section, stage the updated document under `.spektacular/tmp/`, commit it with `{{config.command}} plan file write {{plan_name}} plan --from <scratch>` (or the same command against the plan's `context.md`/`research.md` for changes landing there), confirm the update to the user, and remove the scratch file. Then re-summarize only what the change affected and ask the closing question again. Never write or edit the plan documents with the `Write` or `Edit` tools — `{{config.command}} plan file write` is the only supported way to write them.

After a correction is applied and confirmed — including one that came from a challenged drafting assumption — consider what the correction reveals: a wrong default chosen while drafting, a constraint or convention the user knew that the codebase does not record, a term that was misunderstood. Most corrections are change-local edits and produce no offer — when nothing durable is revealed, carry on without mentioning knowledge capture. When a correction does reveal a durable, generalizable lesson, offer in this same review conversation to save it to the project knowledge base, phrased as the general lesson rather than the specific edit, naming what you would capture and why it is worth keeping. Capture happens only on the user's explicit acceptance: hand an accepted item to the `spek-knowledge` skill, whose own propose-then-confirm flow owns tier and store selection, category routing, and the write. Never invoke `{{config.command}} knowledge write` without that explicit acceptance. If the user declines, the item is not offered again for the remainder of the conversation — a decline is final for that lesson; a deferral ("not now", "later") may be raised again later.

Only conclude the walkthrough once the user gives an explicit affirmative answer to the closing question. Silence, a topic change, or an ambiguous reply is not agreement — keep the walkthrough open until they explicitly confirm. If the session is interrupted mid-review, the workflow stays on this step and a later session resumes the walkthrough here.

Once the user has explicitly signed off, advance to the finished step:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
```
{{/orchestrated}}
{{#orchestrated}}
All three plan documents are committed to the plan store. This run is orchestrated: the user reviews every plan of the epic together once planning ends, so there is **no sign-off here**. Do not ask the user to review this plan, and do not wait for an answer.

Base your summary on the committed documents. You wrote them in the last three steps, so when their content is in your context, use it as it is. Read them back through the CLI only on a resumed session or after a context compaction:

```
{{config.command}} plan file read {{plan_name}} plan
{{config.command}} plan file read {{plan_name}} context
{{config.command}} plan file read {{plan_name}} research
```

From them, prepare the summary you will hand back to your orchestrator at the end of the run. Keep it short, one or two lines per point:

1. The chosen approach and why it was picked over the alternatives.
2. The milestones and their tasks. Name every task whose `**Execution:**` is `human`, with its reason, or say there are none.
3. What was deliberately left out of scope.
4. The drafting assumptions from the `## Drafting assumptions` section of the plan's `research` document: each decision in a line, so the user can challenge it at the review.
5. The project-wide rules this plan relies on or decides: any convention governing files or practices that other specs may also touch (for example how the changelog is kept, or a shared interface's shape), one line each with what this plan does, or say there are none. Your orchestrator compares these across the epic's plans to find where they disagree.
6. The manual checks: every success metric and review the Testing Approach flags **Manual — captured in the implementation test plan**, one line each with what a person will check, or say there are none. Your orchestrator lists them in the epic's planning summary.

Then advance to the finished step, which marks the documents final:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
```
{{/orchestrated}}
