## Design-Worthy Detail Recognition

> Managed by `{{command}} init` — edit `templates/agents/design-trigger.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

While a conversation is settling how something will actually work — an API's
shape, a user-facing flow, a data format, or a worked example of any of
these — watch for the moment that detail becomes settled enough to build to.
That is a design document: the worked design a feature is built to, kept
wherever the team already keeps its designs, and referenced by the spec that
needs it rather than copied into it. Recognizing this moment is your job, not
the user's — don't wait to be asked.

The bar is deliberately high. A design document is not a place to park
anything technical that came up. Ask whether the detail is **settled** (the
user has decided it, not merely floated it), whether it is **worked** (a
concrete shape, format or flow rather than a direction), and whether it would
**make the spec unreadable if written inline**. All three must hold. A hard
boundary the solution must honour is a constraint and belongs in the spec; a
preference the planner may adapt is technical direction and belongs in the
spec; only a worked design that would swamp the spec belongs in a document of
its own. If a one-line steer captures it, it is not a design document.

When you recognize the moment, offer — never write a design document
unprompted. Say what you would capture, which declared source you would write
it to, and why the spec is better off pointing at it than containing it. Run
`{{command}} design sources` to see the declared sources if you do not
already know them. Wait for the user's decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — write the document with `{{command}} design write --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>`, then record the
  reference on the spec with `{{command}} design ref add --data
  '{"spec":"<spec name>","source":"<name>","path":"<path>"}'`. Both steps, every
  time: a document nothing references is invisible to the plan workflow, and a
  reference to a document that was never written is a broken reference.
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
