> **Version check first.** Before running any other command, run `{{command}} version check`.
> - On `status: "match"`, continue with the skill and produce no version-related output.
> - On `"mismatch"`, `"missing"` or `"upgrade_needed"`, the project's settings or installed Spektacular files are out of date: relay the response's `action` message to the user, ask them to run `{{command}} migrate` (they can preview it with `{{command}} migrate --dry-run`), and wait for their decision before continuing.
> - On `"unsupported_format"`, relay the `action` message: the project was written by a newer Spektacular, which the user must install before continuing.
> - Never run `migrate` or `init`, and never modify installed files yourself. Upgrading is always an explicit, user-initiated action.
