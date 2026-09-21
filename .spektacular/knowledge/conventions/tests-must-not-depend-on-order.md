---
tags: [testing, isolation, shuffle, cobra, flake]
---

# Tests must not depend on execution order

Every test must pass in any order and on its own. `make test` runs
`go test -shuffle=on ./...`. A test that passes only after, or only before,
another test is a bug, even when the suite is green in declaration order.

- **Reset shared state through the one command-tree helper.** In `cmd`, every
  command is a package-global cobra command, so a parsed flag stays set for
  whichever test runs next. Any test that executes `rootCmd` must go through
  `resetRootCmd`, which `setupImplementCmd` and `runRootCmd` already call. It
  resets every flag on the whole tree, before the test and again on cleanup.
  Do not add per-command flag reset helpers: a hand-maintained flag list is
  exactly how a new flag gets missed.
- **Don't depend on the wall clock.** A test that compares two runs must not
  rely on both landing in the same second. For example, pin
  `spec.id_method: counter` rather than relying on the default timestamp IDs.
- **To reproduce an order failure**, rerun with the seed the failing run
  printed: `go test -shuffle=<seed> ./cmd`. If the same seed passes on a
  rerun, the cause is timing, not order.
