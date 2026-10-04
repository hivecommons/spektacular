# Interview: 000061_version-check-upgrades-install

All of this was settled in conversation before the workflow started; see .spektacular/working-context.md for the user's
exact words. Synthesis:

- **What:** the version check becomes the single place an out-of-date project is detected and, with one yes from the
  user, brought up to date. Out of date means the project's settings, agent files (managed AGENTS.md sections) or
  installed skills were not written by the current binary. A dev build always counts as current.
- **Flow:** skill starts → version check → if out of date, agent asks one question ("update your settings and
  skills?") → yes: agent runs `init <recorded agent>` and carries on with the skill; no: the skill stops. No skill tells
  the user to run any command; every migrate/init instruction is removed from skills.
- **Refusal:** while out of date, the CLI refuses every command except the version check and `init`, with a next
  action naming them.
- **init replaces migrate:** `migrate` is removed outright (hard removal, no alias; release notes name `init`). `init`
  does everything migrate did (upgrade settings for the project and registered repos, reinstall skills and managed
  sections) and still requires an agent argument; the agent passes the one recorded in config.yaml.
- **When:** only at a version check (skill start, including resuming a paused workflow), never inside a step. Upgrading
  on resume is required, because a new binary with old skills breaks the resume. A paused workflow's state is checked
  against the new version's steps; a step that no longer exists is refused with a clear next action.
- **Newer project:** a project written by a newer Spektacular is still refused (install the newer version).
- **Docs:** the docs site's pages that mention `migrate` (getting-started, configuration, knowledge-base, plan-tasks)
  are updated in the same release; no page presents `migrate` as current.
- **Success:** a user who installs a new binary and starts any skill is back to work after one confirmation, typing
  no command; no installed skill mentions migrate or tells the user to run an upgrade command.
- **Out of scope / rejected:** dev builds are never upgraded (developer's job: make install-local); no `version check
  --upgrade` flag; no deprecation period for migrate; no upgrade inside a step.
