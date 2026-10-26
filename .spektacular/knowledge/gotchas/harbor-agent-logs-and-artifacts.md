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

**Spektacular's own logs come back for every agent.** No suite sets a log
level in the configuration of the project it seeds, and
`SPEKTACULAR_SESSION_LOG` no longer exists: every run records at the
default level (`standard`) to `.spektacular/logs/`, one
`<started>_<agent>_<kind>_<name>.jsonl` per workflow run (each orchestrated
lane included) and `standalone_<day>.jsonl` for commands and spans outside
any run, such as an epic orchestrator's. The pre-built projects' seed
scripts switch logging off while they run and back to the default as their
last step, so the logs hold only the agent's run. The existing
`/app/.spektacular` artifact carries them out, and Harbor reads them only
through `spektacular stats` (`tests/harbor/shared/runlog.py`). The verifiers
that call `spektacular` themselves switch the project's logging off first
(`tests/harbor/shared/loglevel.py`), so their calls stay out of the logs
they assert against. `status` is never logged, at any level, and nothing is
logged where no project config exists, so a call from a repo's code
checkout leaves no log there.

**Regrading needs more than artifacts.** `harbor trial regrade` also needs
the verifier to run in its own container (`environment_mode = "separate"`),
which Harbor builds from a Dockerfile in the task's `tests/` directory, and
no suite has one yet. And a `claude-code` trial leaves root-owned files
under its job directory (`agent/sessions/.claude.json` among them) that the
regrade cannot read, and that stop `go build ./...` and `go test ./...` from
walking `tests/harbor/jobs/` until they are removed or re-owned.
