## Where the Code Lives — Read This First

**STOP. The directory you are running in is not necessarily the code.** A project holding only
`.spektacular/`, `knowledge/` and `repos/` is normal: its code lives wherever the registry says.

- **Before any code-touching work** (searching, reading, editing, running tests, verifying, or
  answering how something works), run `{{command}} repo list` and work in each repo's `root`.
- **Never assume the starting directory holds a repo's code, and never guess a path.** The
  registry is the only authority on where code lives.
- **Pass the relevant repo's `root` explicitly to every sub-agent you launch.**
- **If a repo's `root` is not on disk, stop and tell the user** the path the registry gave.

This binds everywhere: every workflow, ad-hoc question, skill and sub-agent.
Managed by `{{command}} init`; full guidance: [{{guidance_path}}]({{guidance_path}}).
