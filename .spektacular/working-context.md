# Working context: 000061_version-check-upgrades-install

## Problem and motivation

- Today `version check` (run first by every skill) only reports a status (`match`, `upgrade_needed`, `mismatch`,
  `missing`, `unsupported_format`) plus an `action` telling the agent to "ask the user to run `migrate`"
  (`cmd/version.go:96`). Skills say "never run migrate or init". Meanwhile the gate (`cmd/gate.go`) refuses every
  command except `migrate`, `init` and `version check` with `upgrade_required`.
- User: "if it is the responsibility of the user to call this then it will never be used. We already check the version
  I don't understand why we don't just execute the migrate step as part of this process."
- The upgrade itself is already safe unattended: `migrate.Apply` is idempotent, keeps a `.vN.old` backup, refuses a
  newer format; `migrate` also reinstalls the recorded agent's skills and managed AGENTS.md sections (skipped for `dev`).
- `init <agent>` already runs `migrate.Apply` first, then scaffolds, sets agent, always reinstalls skills, and rewrites
  the whole config.yaml via ToYAMLFile (reformats, drops comments).

## Decisions (user's words in quotes)

1. Version check asks one question: do this project's config, agent files and skills match the current binary version?
   "upgrade_needed should be gone from version_check, we should just check does the current config agent file and
   skills match the current binary version." `upgrade_needed`/`mismatch`/`missing` collapse to one out-of-date result.
2. A `dev` build always passes the version check: "That is a developer problem not the user and I think it is ok."
3. When out of date, the agent asks the user one question: "Theoretically this is a simple question to the user, would
   you like me to update your config..." On yes the agent runs `init <recorded agent>` and carries on; on no the skill
   stops. Remove every migrate/init instruction from the skills: "Really we should not have any migrate commands in
   the skill, this should be part of version check."
4. The refusal stays: "we should not allow any action other than a version check when the config and skills are not
   matching the binary." (`init` must also still run, being the upgrade path.) Its next_action names them, not migrate.
5. `migrate` is removed: "we get rid of migrate, init should do everything migrate does". `init` performs the upgrade
   "because someone may install the new binary and just init". `init` keeps requiring an agent argument: "if the user is
   calling init then they will get prompted to specify the agent"; the agent passes the one recorded in config.yaml.
6. Upgrades happen only at a version check, never inside a step: "You should never upgrade mid workflow" / "Mid workflow
   you are not calling version check so migration would not be triggered". BUT upgrading on resume is required: "We
   implement a spec, get to requirements and pause. The user updates the spektacular binary and then resumes the
   workflow. If spektacular binaries commands have changed but the skills have not then the resume process will not
   work ... So it is probably safer to do the check and upgrade with every version check".
7. Paused workflow state (state.json current step/data) should be validated against the new version's workflow steps;
   a step that no longer exists is refused with a clear next action. (Proposed by agent, not objected to.)
8. A project written by a newer Spektacular is still refused (install the newer version). Assumed; user did not object.

## Alternatives rejected

- User runs `migrate` manually (status quo): "it will never be used".
- Holding back the upgrade while a workflow is paused: breaks resume after a binary update (decision 6).
- `version check --upgrade` flag as the upgrade command: rejected in favour of `init`.
- Treating `dev` as never matching / content-hash comparison: rejected, dev always matches.

## Plan-level notes (not spec content)

- `init` should adopt migrate's targeted settings edits + backup rather than rewriting the whole config.yaml.
- Convention now in knowledge: conventions/plans-never-change-the-active-install.md — this spec's plan must not
  migrate/re-init this repo; verify with go test and throwaway projects. This repo now uses `command: spektacular`.

## Repos

- spektacular (CLI, skills, templates) and docs (spektacular-website: documents migrate/version check — likely needs
  updating; ask in interview).
