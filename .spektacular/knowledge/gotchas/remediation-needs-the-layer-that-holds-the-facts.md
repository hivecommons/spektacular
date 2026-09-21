---
tags: [error, remediation, validation, cli]
---

# Validation that must suggest remediation belongs in the layer that holds the facts

`conventions/error-messages-must-suggest-remediation.md` requires every error to
carry a runnable `next_action`. That requirement quietly constrains *where* a
check may live: a layer that does not hold the facts the remediation is built
from cannot satisfy it, and will produce a generic message instead. Validating
early, at the command surface, feels like the tidy thing to do and is the trap.

It surfaced while adding tier-and-name addressing to the knowledge commands.
`cmd/knowledge.go` parsed the `--data` address and validated it before building
the `knowledge.Set`. Everything looked right: the refusal carried the correct
error code, a non-empty `next_action`, and the whole test suite was green. But
the next action could only say "reissue with a name" — it could not list the
names available, because only the `Set` knows which stores are configured. The
check ran one layer too high to say anything useful, and no test caught it
because each assertion held individually.

What to do instead: put the check where the information for its remediation
lives, even when that means letting a malformed request travel one layer
further. Here the command surface was reduced to parsing and checking the entry
path, and `Set.resolve` became the sole validator of an address, enriching every
refusal with the store names in the tier concerned. Nothing is written on the
refusal path either way, so validating later costs nothing.

Two things generalise:

- When a check's `next_action` would have to name configured values, registered
  entities, or valid states, it belongs beside whatever owns those, not at the
  CLI boundary.
- Assert the *content* of a `next_action`, not just that it is non-empty. A test
  for non-emptiness passes on exactly the message this convention exists to
  prevent.
