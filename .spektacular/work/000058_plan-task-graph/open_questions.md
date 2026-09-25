- **Does `ToYAMLFile` now emit `plan.task_id` into rewritten config files and migration goldens?** Depends on how the default-seeded `PlanConfig` marshals with `omitempty` on a struct field, which surfaces only when the config and migrate test suites run. If goldens change, update them only when the change is the new key alone; if anything else shifts, STOP and ask the user.
- **Does an in-flight plan workflow paused at the `phases` step exist in any user project at release?** Only observable at upgrade time. If `plan goto` reports an `invalid_transition` for `phases` during implementation testing, STOP and ask the user whether to add a resume alias from `phases` to `tasks`.

No other implementation-time uncertainties remain; every other decision is recorded in the assumption log.
