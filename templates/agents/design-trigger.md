## Design-Worthy Detail Recognition

> Managed by `{{command}} init` — edit `templates/agents/design-trigger.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

**Stay alert whenever a conversation is working out how something will
actually behave** — an API's shape, a user-facing flow, a data format, or a
worked example of any of these. Being alert is not the same as offering.
Noticing costs nothing and happens continuously; offering happens only once
the detail clears the bar below. Watching only for detail that has already
settled is how the moment gets missed, because by then the conversation has
usually moved on. Recognizing it is your job, not the user's — don't wait to
be asked.

What you are watching for is a design document: the worked design a feature is
built to, kept wherever the team already keeps its designs, and referenced by
the spec that needs it rather than copied into it.

The bar to offer is deliberately high. A design document is not a place to
park anything technical that came up. Ask whether the detail is **settled**
(the user has decided it, not merely floated it), whether it is **worked** (a
concrete shape, format or flow rather than a direction), and whether it would
**make the spec unreadable if written inline**. All three must hold. A hard
boundary the solution must honour is a constraint and belongs in the spec; a
preference the planner may adapt is technical direction and belongs in the
spec; only a worked design that would swamp the spec belongs in a document of
its own. If a one-line steer captures it, it is not a design document.

Note these are three homes for the *content*, not three degrees of how binding
it is. A design document is settled by definition, so a spec that references
one records that pointer among its **constraints**, never as technical
direction: the plan workflow builds on a referenced design, weighs its options
within the shape it fixes, and raises a disagreement with the user rather than
designing around it.

A design enters a project in more ways than one, and three of them are easy to
walk past:

- **The user already has the document.** They mention a design doc they wrote,
  or paste one in. Nothing needs working out; it needs storing, exactly as
  they supplied it, and referencing.
- **A design that already exists is being changed.** The conversation
  contradicts or extends a design the project already holds. The document is
  now wrong, and saying so is part of your job.
- **There is no spec in sight.** Design talk does not wait for a spec to
  exist. A design can be worked out and written down on its own, and a
  reference recorded later if a spec ever needs one.

When you recognize the moment, offer — never write a design document
unprompted. Say what you would capture, which declared source you would write
it to, and why the spec is better off pointing at it than containing it. Run
`{{command}} design sources` to see the declared sources if you do not
already know them. Wait for the user's decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — invoke the `spek-design` skill, which owns the conversation
  from here: it runs the interview when the design still has to be worked out,
  stores a document the user already has exactly as supplied, and revises one
  that already exists. It ends in `{{command}} design author --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>` for a design
  Spektacular writes with the user, or `{{command}} design write --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>` for one the user
  handed over, which is stored byte for byte with nothing added. Then, **only
  if a spec exists**, record the reference with `{{command}} design ref add
  --data '{"spec":"<spec name>","source":"<name>","path":"<path>"}'`, because a
  document nothing references is invisible to the plan workflow. When there is
  no spec yet, writing the document is the whole of the work.
- **Defer** ("not now", "later", "once we've settled it") — write nothing.
  Continue the conversation normally, and treat this as temporary: if the
  discussion keeps developing that detail, you may raise the offer again later
  in the same conversation.
- **Decline** ("no", "keep it in the spec") — write nothing, and do not raise
  the offer again for this detail for the remainder of the conversation. A
  decline is final for that detail, not a "not now." Declining also means the
  detail does not get smuggled into the spec body instead: it stays out, or it
  stays as the one-line steer it already was.

Silence or deflection is not acceptance. Never create or overwrite a design
document without the user's explicit agreement — though a direct instruction
to write one *is* that agreement, and needs no further confirmation.
