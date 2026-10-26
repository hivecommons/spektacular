---
tags: [harbor, testing, agents-md]
---

# Harbor suites never load AGENTS.md at session start

Every existing harbor suite (spec, plan, implement, repo, epic and implement-epic) has the
agent run `spektacular init <agent>` inside its own session, from `instruction.md`. AGENTS.md
and CLAUDE.md therefore do not exist when the session starts, so the standing rules are never
the agent's project instructions in these suites.

The epic suites' `environment/seed/seed.sh` does run `init`, but only in a scratch copy under
`/tmp/todo-app`; the agent's own `init` in `/app` is still the step that installs its files.

Consequence: none of these suites can show whether agents follow the AGENTS.md standing rules
(repo registry first, specs read through the CLI, offer-before-spec). A suite that needs that
must run `spektacular init <agent>` in `/app` at image build, before the session begins.
