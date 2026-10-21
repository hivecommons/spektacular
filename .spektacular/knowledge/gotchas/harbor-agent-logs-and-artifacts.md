---
tags: [harbor, testing, agent, logs]
---

## Harbor already keeps the agent's transcript; declare other artifacts per agent

**The agent's own transcript is already collected.** Harbor's installed
agents write their native session logs into the job's `agent/` directory
without any `task.toml` entry. `claude-code` sets
`CLAUDE_CONFIG_DIR=/logs/agent/sessions`, so the whole Claude config
directory, every session transcript included, comes back under
`agent/sessions/`. `codex` copies its `$CODEX_HOME/sessions` rollouts there
too. Do not add artifact entries for these: they would duplicate what is
already collected.

**Declare an agent-specific artifact only for that agent.** A
`[[artifacts]]` source that does not exist does not fail a run: Harbor
records it as `failed` in the artifacts manifest and carries on. But
`harbor trial regrade` refuses any trial in which a declared artifact failed
to collect, so listing every agent's log directory in every task makes every
trial unregradable. Agent-specific artifacts live in
`tests/harbor/agents/<agent>-artifacts.toml` (today only Bob's, for
`/root/.bob/logs`), and the Makefile's render targets append the running
agent's file to the rendered `task.toml`. A suite's own `task.toml` declares
only what every agent produces.

**Name log directories, never an agent's home.** An agent's home or config
directory holds its credentials (`/tmp/codex-home` carries Codex's
`auth.json`, `~/.bob/settings` holds the Bob login `bob_agent.py` uploads).
Collect only the log directory beneath it.

**Spektacular's own session log comes back for every agent.** Each suite's
Dockerfile sets `ENV SPEKTACULAR_SESSION_LOG=on`, which records every
`spektacular` call to `.spektacular/debug/` whatever `debug.enabled` says,
and the existing `/app/.spektacular` artifact carries it out. `=on` records
only where a project config exists, so a call made from a repo's code
checkout leaves no `.spektacular/` there. Each orchestrated lane gets its own
file holding its `new`, its `goto`s and any command whose arguments name its
spec while it runs; the shared `no-active-workflow` file holds the
orchestrator's commands and the few a child runs without naming its spec.

**Regrading needs more than artifacts.** `harbor trial regrade` also needs
the verifier to run in its own container (`environment_mode = "separate"`),
which Harbor builds from a Dockerfile in the task's `tests/` directory, and
no suite has one yet. And a `claude-code` trial leaves root-owned files
under its job directory (`agent/sessions/.claude.json` among them) that the
regrade cannot read, and that stop `go build ./...` and `go test ./...` from
walking `tests/harbor/jobs/` until they are removed or re-owned.
