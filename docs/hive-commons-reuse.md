# Hive Commons Reuse Review

Spektacular imports no Hive Commons Go module. This page records where a Hive
Commons package was checked as a replacement for Spektacular's own code, what
was found, and what was recommended
([#91](https://github.com/hivecommons/spektacular/issues/91)). It is a dated
reading, not a standing rule: re-check it when a Hive Commons module starts
publishing versioned Go tags.

Revisions read (2026-10-24): Spektacular `39a0059` (main), hive `2cd6eb5b2`
(`v5`, its default branch), dibs `d12932a` (main), crux (README only; it holds
no code yet).

## Can Spektacular import these modules at all?

Not yet. A swap means a working `go get`, and none of the Hive Commons Go
modules offers a usable versioned one:

- **hive.** The module `github.com/hivecommons/hive` lives in `src/`, not at
  the repository root. Its release tags are `v5.x.y`, but the module path has
  no `/v5` suffix. `go get github.com/hivecommons/hive@v5.186.4` resolves to
  `v5.186.4+incompatible`, a go.mod-less view of the repository root, which
  does not contain `pkg/...`. The `.../hive/src/...` path fails the same way.
- **crux.** The nested module `github.com/hivecommons/hive/crux` lives at
  `src/crux`, so `go get` cannot find it by its path either ("module declares
  its path as: github.com/hivecommons/hive/crux but was required as:
  github.com/hivecommons/hive/src/crux"). hive's design record
  (`src/docs/design/crux.md`, "After the cut") already names this. Its fix is
  to move `src/` to the root, or to move Crux into `hivecommons/crux` in Stage
  3, "when the first consumer outside this repository appears". The
  `crux/vX.Y.Z` tags it plans are not cut yet. The `hivecommons/crux`
  repository holds only a README.
- **dibs.** `go get` works, but only at a pseudo-version: there are no tags
  and no stability notes.
- **Go version.** hive and crux declare `go 1.26.10` and dibs `go 1.27`.
  Spektacular declares `go 1.25.0`, so any of them would raise its minimum Go.
- **Module weight.** The hive root module requires 29 direct modules (OTel,
  gRPC, bubbletea, CEL, go-github and others). crux requires five (cel-go,
  otel, otel/trace, go-qrcode, yaml.v3). Spektacular requires 12.

The dependency runs the other way today. hive's `pkg/spektacular` runs the
`spektacular` CLI as a subprocess and parses its JSON output. Neither hive
module imports Spektacular's Go code.

## Candidates

| Spektacular | Checked against | Match | Recommendation |
|---|---|---|---|
| `internal/agent` | hive `crux/agent`, `crux/agentparse`, `crux/agentmode`; crux | Name only | Keep |
| AGENTS.md managed sections (`internal/agent/managed_section.go`, `templates/agents`) | hive `crux/agentsmd` | Opposite direction | Keep |
| Skill library (`cmd/skill.go`, `templates/skills`) | hive `crux/skillreg` | Partial, read side only | Keep |
| `internal/knowledge` | hive `pkg/knowledge` | Name only | Keep |
| `internal/store` | dibs `pkg/store`, `pkg/fsutil` | Name only (fsutil: tiny overlap) | Keep |
| `internal/config` | hive `pkg/config`, `crux/config` | Name only | Keep |
| `internal/output` | hive `crux/outputschema`, `crux/jsonextract` | Opposite direction | Keep |
| `internal/sessionlog` | hive `pkg/timeline`, `crux/trajectory`, `pkg/tracing` | Name only | Keep |
| `internal/gitexec` | hive `crux/gitidentity` | None | Keep |
| `internal/worktree` | hive `crux/sandbox` (`worktree.go`) | Name only | Keep |
| `internal/autocommit` commit lock (`lock.go`) | hive `pkg/proclock` | Same job, better mechanism | Adopt the mechanism, not the package |

### `internal/agent` — keep

Spektacular's `Agent` is an installer. `Install` writes skills, command
wrappers, AGENTS.md sections and agent-specific files into a project for
Claude, Bob, Codex, Copilot or omp. hive's `crux/agent` is a different thing:
a runtime of about 24,000 lines that launches and supervises agent processes,
with sandbox, watchdog and OTel imports. It has no installer or registry.
`agentparse` parses tables and lists out of agent output, and `agentmode` is
hive's GitHub permission ladder. Neither does anything Spektacular does.

### AGENTS.md managed sections — keep

Spektacular writes AGENTS.md. `managed_section.go` finds a section by its
heading, then appends it, replaces it in place, or moves it, and writes the
file atomically. `crux/agentsmd` only reads AGENTS.md: front-matter
`skills:`, `## Skill:` sections and `skills/*.md`, folded into a size-capped
prompt block with hive's own precedence wording. It has no write path, so a
swap would lose everything Spektacular uses it for. If the section writer is
ever wanted elsewhere, Spektacular's code is the one to extract.

### Skill library — keep

`spektacular skill <name>` returns one skill from the binary's embedded
`templates/skills`. `crux/skillreg` loads a directory of front-mattered `.md`
files into a versioned, searchable registry, and it imports `crux/agentsmd`.
The overlap is a lookup by name, about 40 lines in Spektacular. Revisit only
if Spektacular needs versioned or third-party skills.

### `internal/knowledge` — keep

Spektacular's `Set` fans list, search and the always-applied reader across
ordered stores tagged with project/repo/all tiers, and resolves each read or
write to exactly one store. hive's `pkg/knowledge` (about 15,000 lines) is an
Obsidian-vault fact store with embedding ranking, confidence and a background
re-index. It imports `pkg/beads`, `pkg/github`, go-github, bbolt and a PDF
parser. The two share a name and nothing else.

### `internal/store` — keep

Spektacular's `Store` reads, writes, lists and searches markdown under a
directory, with tag front matter and `.spektacular_ignore`. dibs `pkg/store`
persists JSON `Idea` records with a lifecycle. dibs `pkg/fsutil` is 46 lines of
atomic write (temp file plus rename). Spektacular already does that itself, in
`migrate/engine.go` (`writeAtomic`) and `agent/managed_section.go`, which is
too little to justify a dependency on an untagged module.

### `internal/config` — keep

Spektacular's config holds the project's `config.yaml` and `repo.yaml`: stores,
knowledge sources, registered repos, and schema versions for `migrate`.
hive's `pkg/config` loads `hive.yaml` and imports about 15 hive packages.
`crux/config` is stdlib-only, but it holds only agent-runtime types (gateway,
LiteLLM, sandbox, watchdog) with no loading code.

### `internal/output` — keep

Spektacular's `output` writes a command's JSON result, optionally filtered to
some fields, and the `ErrorResponse` envelope every refusal uses.
`crux/outputschema` validates the report an agent writes at the end of a hive
run, and `crux/jsonextract` cuts a JSON object out of model prose. Both read
agent output. Neither writes a CLI's result.

### `internal/sessionlog` — keep

`sessionlog` appends one JSON Lines record per CLI invocation for after-the-fact
debugging, using only the standard library. `pkg/timeline` keeps hive's
in-memory issue→PR journeys for the dashboard. `crux/trajectory` asks a second
model whether a running agent has drifted. `pkg/tracing` is OpenTelemetry.
None of them records a CLI session.

### `internal/gitexec` — keep

`gitexec` is the one place Spektacular runs git, with prompts disabled and SSH
in batch mode. `crux/gitidentity` returns hive's bot commit name and email and
never runs git. A bot identity would also go against Spektacular committing as
the user. hive has no shared git runner of its own to adopt.

### `internal/worktree` — keep

`worktree` gives each spec of an epic a branch and worktree in every repo its
plan touches. It merges them back all-or-nothing after a `merge-tree` dry run
in each repo. `crux/sandbox`'s `WorktreeManager` adds and force-removes
detached worktrees for a sandboxed run, with no branches, merging or multi-repo
support.

### `internal/autocommit` commit lock — adopt the mechanism, not the package

This is the one pair that does the same job. `autocommit.AcquireLock` creates
`.spektacular/workflows/.commit.lock` with `O_EXCL` and retries for up to 30
seconds. It removes the file on release, and it breaks a lock file older than
10 minutes. hive's `pkg/proclock` takes an `flock(2)` lock instead, which the
kernel releases however the holder dies. A lock file left by a dead process is
just an unlocked file.

The difference matters when a process holding the lock is killed: until the
10-minute stale timer runs out, every commit fails after its 30-second wait.
The timer has a second risk: a holder that is still alive after 10 minutes has
its lock broken under it.

`proclock` cannot be imported as it is, for three reasons:

- hive is not fetchable (above);
- it fails at once when the lock is held, where Spektacular's lock waits;
- it calls `syscall.Flock` with no build tags, so it does not compile for
  Windows (`GOOS=windows go vet ./pkg/proclock`: `undefined: syscall.Flock`),
  and Spektacular ships a Windows binary (`dagger/main.go`).

The recommendation is to give Spektacular's lock the same self-releasing
property in-repo: `flock` on Unix, `LockFileEx` on Windows, keeping the bounded
wait. Extracting `proclock` into a shared module first is not worth it for one
small consumer that also needs a Windows variant and a wait.

## Outcome

There is no swap worth making today. Nine pairs share a name or a word, but the
Hive Commons package does a different job (or the same job in the opposite
direction). The tenth, the commit lock, does the same job, but its package
cannot be imported, does not build for Windows, and does not wait. The one
action is a follow-up issue to make the commit lock release itself when its
holder dies ([#99](https://github.com/hivecommons/spektacular/issues/99)).

Re-check this page when hive moves its module to the repository root, cuts
`crux/vX.Y.Z` tags, or moves Crux into `hivecommons/crux`, and when dibs
starts tagging releases.
