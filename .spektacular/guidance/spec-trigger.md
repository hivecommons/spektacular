# Spec-Worthy Discussion Recognition

> Managed by `spektacular init` and refreshed by `spektacular migrate` — edit
> `templates/agents/spec-trigger.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

During an open-ended discussion (diagnostics, brainstorming, exploratory back-and-forth), watch for the moment it produces something substantial enough to be worth capturing as a specification — multiple requirements mentioned, a scoped decision reached, or a feature described in enough detail that it could be built from. Don't wait to be asked; recognizing this moment and offering is your job, not the user's.

This check is not limited to live back-and-forth discussion. It applies just as much the moment you finish reading a fully-formed request — a GitHub issue, a linked ticket, a pasted design doc — that already describes a feature in spec-worthy detail. Reading a well-specified issue and going straight to implementation is the same miss as skipping the offer mid-conversation: evaluate the request against the criteria above *before* starting any implementation work, not only when the detail accumulates turn-by-turn in front of you.

Before deciding whether a discussion has crossed that line, read `spec_trigger_threshold` from `.spektacular/config.yaml` — check it at the moment you are deciding, not once at the start of the session, since the user may change it mid-conversation and expects that to take effect immediately. Treat a missing or absent value as `"moderate"`. Use the configured value to calibrate how readily you offer:

- `"strict"` — only offer for substantial, multi-requirement features. Small fixes and minor tweaks should not trigger an offer.
- `"moderate"` — offer once a discussion reaches a clear, scoped decision or a feature description with more than one requirement. The default balance.
- `"lenient"` — offer readily, including for small bug fixes or narrowly scoped changes, whenever there's a decision worth recording.

When you recognize the moment, always offer — never start the spec workflow unilaterally. Propose capturing the discussion as a spec, briefly say why (e.g. "this sounds like it's grown into a few concrete requirements — want me to capture it as a spec?"), and wait for the user's decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — proceed to carry-forward below.
- **Defer** ("not yet", "still investigating", "let me think") — do not start the spec workflow. Continue the conversation normally, and treat this as temporary: if the discussion keeps developing, you may raise the offer again later in the same conversation.
- **Decline** ("no", "I don't want a spec for this") — do not start the spec workflow, and do not raise the offer again for this discussion topic for the remainder of the conversation. A decline is final for that topic, not a "not now."

If the user accepts, start the spec workflow:

```
spektacular spec new --data '{"name":"..."}'
```

Then drive its existing steps — the "ask the user..." prompts in `templates/steps/spec/*.md` are unchanged and still apply — but answer each one from the conversation you already had instead of asking cold. For every step where the discussion already established an answer (e.g. the overview step's "describe this feature in 2-3 sentences"), propose a draft based on what was said and ask the user to confirm or refine it, rather than posing the question as if from scratch. Only ask a step's question directly when the conversation genuinely didn't cover it. The user's confirmation or correction is still the final word — never silently record your own draft as accepted without it.

This behavior is scoped to the current, single conversation: it does not persist across sessions, and it does not apply outside a Spektacular-initialized repository.
