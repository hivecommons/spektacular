### The split check

The check asks one question about a complete spec: **is this more than one independently useful piece of work?** Size alone is not the test. It runs once, when a spec is complete, and whenever the user explicitly asks for a split. It never runs during open-ended discussion: recognising spec-worthy discussion is a separate behaviour and never offers an epic.

**Read the sensitivity first.** Read `epic_split_threshold` from `.spektacular/config.yaml` now, at the moment you are deciding, not from memory: the user may have changed it. Treat a missing or absent value as `"moderate"`. This setting is separate from `spec_trigger_threshold`; never use one in place of the other. It moves only the gate and the number of signals needed, as below. It never overrides the supporting-work rule or the counter-signals.

**The gate.** Offer a split only if you can name at least two specs, each with its own overview and at least one acceptance criterion that can be verified without the others. If you cannot name them, do not offer, however many signals have fired. This keeps every offer concrete.

**Strong signals.** Any one is enough, provided the gate passes:

- the source the spec was started from lists child items (sub-issues, a task list);
- the requirements fall into groups that could each ship and be useful alone;
- the acceptance criteria cannot all be verified by one change;
- the user's own wording phases the work ("phase 1", "first … then later", "v1 is just …").

**Weak signals.** At least two together are needed:

- more than about seven requirements;
- more than one design document needed;
- an interview that did not converge, where each answer opened new areas;
- a section draft that kept growing content belonging to a different concern.

**How `epic_split_threshold` moves the check:**

- `"strict"` — offer only on a strong signal, and only when each proposed spec could ship on its own today. Weak signals alone never lead to an offer.
- `"moderate"` — the default: one strong signal, or two weak signals together.
- `"lenient"` — one strong signal, or a single weak signal, is enough.

The gate applies at every level.

**Supporting work never counts.** Docs, tests, migrations, config, changelog entries, and skill or template updates that describe or support the same change belong in the same spec. Touching several repos or several surfaces is not a signal in itself; it matters only when a part would be useful on its own, such as a new tutorial unrelated to the code change. Code plus its docs is one spec, at every sensitivity.

**Counter-signals.** These suppress the offer even when signals have fired, at every sensitivity:

- the requirements are tightly coupled, so neither part is useful or testable alone;
- several surfaces or repos serve one capability, or one atomic change.

**Offer, never act.** When the check passes, offer — never split on your own:

> "This sounds like more than one spec. Split it into an epic with these N specs?"

followed by each proposed spec with its one-line scope. Wait for the user's decision. If the check does not pass and the user asked for a split, say plainly why it cannot be split (for example, "the two parts can't be verified separately"), rather than staying silent.

On a decline, continue exactly as if no split had been offered. Do not repeat the offer unless the scope visibly grows after the decline: a new strong signal, or a new independent requirement group. Re-wording what was already there does not count.
