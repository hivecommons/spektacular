## Step {{step}}: {{title}}

All three plan documents are committed to the plan store. Before the workflow can finish, walk the user through the plan and get their explicit sign-off. This review is mandatory — do not offer it as a choice, and do not skip it because the user seems satisfied.

Begin by reading all three committed documents back through the CLI — ground the review in the committed state, not your memory of drafting it (on a resumed session, this is the only reliable source):

```
{{config.command}} plan file read {{plan_name}}/plan.md
{{config.command}} plan file read {{plan_name}}/context.md
{{config.command}} plan file read {{plan_name}}/research.md
```

Present the plan as a colleague would explain it aloud, not as a recitation of `{{plan_path}}`'s section text. Pace it as a small number of natural beats with a brief pause between each, rather than one uninterrupted monologue or a rigid sign-off gate after every point:

1. The chosen approach and why it was picked over the alternatives.
2. How the work breaks into tasks under its milestones. Before asking for sign-off, name every task whose `**Execution:**` is `human`, with its reason, so the user knows which work needs a person. If there are none, say so.
3. What was deliberately left out of scope, and why.
4. The drafting assumptions — the judgement calls made while drafting. Read the `## Drafting assumptions` section of `{{research_path}}` (via `{{config.command}} plan file read {{plan_name}}/research.md`) and walk its entries one by one — decision, rationale, rejected alternatives — inviting the user to challenge each. This beat is first-class: raise the assumptions explicitly rather than leaving them as a section the user could read. A challenged assumption is a change request — handle it through the apply-immediately path below.

At any point during the walkthrough, the user may interrupt with a question or a request to change something. Respond to it directly in your next message before returning to the beat you were on. If the interruption is a change request, apply it immediately rather than deferring it: edit the affected section, stage the updated document under `.spektacular/tmp/`, commit it with `{{config.command}} plan file write {{plan_name}}/plan.md --from <scratch>` (or the same command against the plan's `context.md`/`research.md` for changes landing there), confirm the update to the user, remove the scratch file, then resume the walkthrough, re-summarizing anything the change affected. Never write or edit the plan documents with the `Write` or `Edit` tools — `{{config.command}} plan file write` is the only supported way to write them.

After a correction is applied and confirmed — including one that came from a challenged drafting assumption — and before resuming the walkthrough, consider what the correction reveals: a wrong default chosen while drafting, a constraint or convention the user knew that the codebase does not record, a term that was misunderstood. Most corrections are change-local edits and produce no offer — when nothing durable is revealed, resume the walkthrough without mentioning knowledge capture. When a correction does reveal a durable, generalizable lesson, offer in this same review conversation to save it to the project knowledge base, phrased as the general lesson rather than the specific edit, naming what you would capture and why it is worth keeping. Capture happens only on the user's explicit acceptance: hand an accepted item to the `spek-knowledge` skill, whose own propose-then-confirm flow owns tier and store selection, category routing, and the write. Never invoke `{{config.command}} knowledge write` without that explicit acceptance. If the user declines, the item is not offered again for the remainder of the conversation — a decline is final for that lesson; a deferral ("not now", "later") may be raised again later.

Only conclude the walkthrough once the user gives an explicit affirmative answer to a direct closing question (e.g. "does this look right, ready to proceed?"). Silence, a topic change, or an ambiguous reply is not agreement — keep the walkthrough open until they explicitly confirm. If the session is interrupted mid-review, the workflow stays on this step and a later session resumes the walkthrough here.

Once the user has explicitly signed off, advance to the finished step:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
