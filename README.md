# Spektacular

Agent-agnostic CLI tool for spec-driven development, providing skills and integrations for coding agents (Claude, Bob, Codex) to plan and implement work from a written spec.

> **Status:** early development — see the [releases page](https://github.com/hivecommons/spektacular/releases) for the latest version.

## What is Spektacular?

Spektacular is a self-contained Go binary that brings spec-driven development to AI coding agents. You write a markdown specification; Spektacular turns it into a reviewed implementation plan and then drives a coding agent to implement it — keeping your intent reviewable at every stage.

Its core competencies:

- **Self-contained binary plus installed agent skills.** A single binary that, on `init`, installs the skills (and commands) your coding agent needs to run the Spektacular workflows.
- **State-machine-driven workflow.** Spec, plan, and implement each run as a stepwise state machine. Spektacular hands the agent one per-step prompt at a time (`new` / `goto` / `steps`), so every stage is resumable — stop, inspect, edit, and resume without losing work.
- **Agent-agnostic, multi-agent support.** Works with claude, bob, and codex; pick the one your team already uses, or register your own.
- **Project knowledge base.** A searchable, layered store of conventions, architecture, gotchas, and learnings that feeds context into planning.
- **Project design documents.** The worked design a feature is built to (an API shape, a user-facing flow, a data format) kept wherever your team already keeps it and referenced by the spec that needs it, so specs stay readable and planning is bound to the design that was agreed.

## How It Works

Spektacular follows a three-stage workflow — **spec → plan → implement** — each driven step by step by a state machine:

1. **Spec.** You write a markdown spec (requirements, constraints, acceptance criteria); `spec new` scaffolds one from a template.
2. **Plan.** `plan new` explores your codebase, asks clarifying questions, and writes a detailed implementation plan — `plan.md`, `research.md`, and `context.md`.
3. **Implement.** `implement new` drives the coding agent through each phase of the plan and validates the result against your acceptance criteria.

Workflow progress can be inspected without reading Spektacular's state files directly. `spektacular spec status` and `spektacular plan status` keep reporting the single in-progress workflow. Passing an artifact name switches the commands to per-artifact status:

```bash
spektacular spec status <name>
spektacular plan status <name>
```

Both take the bare artifact name with no extension (`000057_git-commit`, not `000057_git-commit.md`), the same name `state.json` records in `data.name`. That bare name is the stable key shared by convention across the spec file, the plan directory and the changelog record; it is the key to join on, and the addressing convention [#46](https://github.com/hivecommons/spektacular/issues/46) proposes for every verb. `plan status <name>` reports the plan's `plan.md` only, not the plan's other documents.

The named form returns JSON with the artifact kind, name, document status, workflow step when that artifact is currently in progress, completed steps, `created_at`, `closed_at`, and the `spec` / `plan` frontmatter cross-references (surfaced when present, but rarely populated today). Two timestamps are kept apart: `updated_at` is workflow activity and appears only while that artifact has the in-progress workflow, so its absence means nothing is live; `modified_at` is the store's modification time for the artifact and moves on any write, including a checkout or a reformat. Frontmatter dates are stored as `YYYY-MM-DD` and are emitted as RFC3339 midnight UTC timestamps. `spec file list` and `plan file list` carry `modified_at` per entry, so polling many artifacts is one list call.

For the full pipeline, see the [how-it-works documentation](https://spektacular.dev/how-it-works/).

## Install & getting started

Spektacular is a single self-contained Go binary.

```bash
# Homebrew
brew install hivecommons/homebrew-repo/spektacular

# Go 1.21+
go install github.com/hivecommons/spektacular@latest
```

Or download a pre-built binary from the [releases page](https://github.com/hivecommons/spektacular/releases). See the [install docs](https://spektacular.dev/install/) for apt and other methods. You also need a supported coding agent CLI (claude, bob, or codex) installed and configured.

Once installed, the minimal path is initialise → spec → plan → implement:

```bash
# 1. Initialise your project for a coding agent (claude, bob, or codex)
spektacular init claude

# 2. Scaffold a spec, then fill in your requirements
spektacular spec new --data '{"name":"auth-feature"}'
$EDITOR .spektacular/specs/<returned-spec-name>.md

# 3. Generate an implementation plan
spektacular plan new --data '{"name":"<returned-spec-name>"}'

# 4. Implement the plan
spektacular implement new --data '{"name":"<plan-name>"}'
```

Spec names are normalised and prefixed by the CLI, so use the returned `spec_name` and `spec_path` for follow-up commands rather than the name you passed.

Specs are plain markdown with a small set of structured sections (overview, requirements, constraints, acceptance criteria, and so on), and `spec new` scaffolds the template for you. For the full walkthrough and spec format, see the [getting-started tutorial](https://spektacular.dev/tutorials/getting-started) and the [how-it-works documentation](https://spektacular.dev/how-it-works/).

## Supported agents

Spektacular ships with three coding-agent integrations. `spektacular init <agent>` runs the chosen agent's install step, writing its workflow skills (and, where the agent has no skill mechanism, command wrappers) into your project:

- **claude** — installs the workflow skills under `.claude/skills/` and ensures the project's `CLAUDE.md` imports `@AGENTS.md`, so the Spektacular agent rules take effect.
- **bob** — installs skills under `.bob/skills/` and command wrappers under `.bob/commands/`.
- **codex** — installs skills under `.agents/skills/`.

Each integration is deliberately small: an agent implements a narrow `Agent` interface — `Name()` (its CLI identifier) and `Install()` (which writes its workflow artefacts) — and registers itself with the agent package from an `init()` function. Adding a new agent means implementing those two methods and registering the type.

Both the coding agent and the storage layer are pluggable behind defined Go interfaces — the `Agent` interface in `internal/agent` and the `Store` interface in `internal/store` (the read/write/search surface backing the spec, plan, and knowledge stores). Only the `file` store ships today. For the full interface signatures and how to add your own backend or agent, see the [extending documentation](https://spektacular.dev/extending/) and the [plugins overview](https://spektacular.dev/plugins/).

## Project Structure

Running `spektacular init <agent>` creates:

```
.spektacular/
├── config.yaml              # agent, command, debug, store settings, and the repo registry
├── repo.yaml                # the colocated repo's own configuration
├── specs/                   # your specification files
├── plans/                   # generated plans (plan.md, research.md, context.md)
├── changelog/               # changelog records written by the implement workflow
└── knowledge/               # default project knowledge source
    ├── conventions/         # always-applied: standing rules, one per file
    ├── glossary/            # always-applied: shared domain/project terms
    ├── architecture/        # looked-up: how the system is built
    ├── gotchas/             # looked-up: sharp edges and traps
    ├── learnings/           # looked-up: empirical findings from past work
    └── decisions/           # looked-up: the reasoning behind choices
```

Each knowledge category directory is scaffolded with a `README.md` describing what belongs in it. By default Spektacular reads `.spektacular/knowledge/` as this repo's own store, addressed by the name the project registered the repo under; the project can declare additional shared stores — for example a `team` directory or a machine-wide `global` one — under `knowledge.sources` (see [Configuration](#configuration)). See [Knowledge](#knowledge) for how it is organised and consumed.

## Knowledge

Knowledge is the accumulated know-how a project draws on when planning — conventions, glossary terms, architecture notes, gotchas, learnings, and decisions. It is strictly a **planning-time input**: the planning agent reads it while producing a plan, and the relevant parts are written into the plan itself. The implement workflow then consumes only the plan documents — the plan is the contract.

### Six categories, two retrieval tiers

Every entry belongs to exactly one of six categories, fixed by the first segment of its path. Each category has a **retrieval tier** that decides when its entries are loaded:

- **Always-applied** — `conventions` (standing rules to follow) and `glossary` (shared domain and project terms). Loaded in full on every planning task, and deliberately excluded from search results so they are never surfaced twice.
- **Looked-up** — `architecture`, `gotchas`, `learnings`, and `decisions`. The larger reference body, fetched only when a search matches, so it can grow without weighing down every task.

The category model — names, retrieval tiers, and per-category boundaries — is declared once in code, so it stays consistent across directory scaffolding, search labelling, and retrieval. A category's *retrieval tier* says **when** its entries are loaded; the addressing *tier* below says **which** knowledge a store holds. The two are different axes.

### Tiers, search, and de-duplication

Knowledge lives in one of two tiers. Every registered repo contributes exactly one store, addressed by the name the project registered it under, holding knowledge about that repo's own code. The project declares any number of shared stores under `knowledge.sources`, each with its own `name`, for knowledge that belongs to no single repo (see [Configuration](#configuration)). Every read, search, and always-applied load states a tier and, optionally, the store names to narrow to; every result reports the tier and store it came from.

Reading and writing name exactly one store, so they take a `tier` and a `name` alongside the path; a request that leaves either out is refused, and the refusal lists the names available in that tier. Searching, listing, conventions, and the always-applied load take `--tier <project|repo|all>` and a repeatable `--filter <name>`; omitting the narrowing covers every store the tier reaches, and no store is ever included or excluded implicitly.

Lookups are **consolidated and de-duplicated** across stores: each entry carries a SHA-256 checksum over its exact bytes, and byte-identical entries appearing in more than one store collapse to a single result. A search result looks like:

```
Hit {
  tier      // addressing tier of the originating store (project or repo)
  name      // name of the originating store (e.g. docs)
  path      // locator relative to the store root (e.g. gotchas/db-timeouts.md)
  title     // the document's first heading, or the locator when it has none
  excerpts  // compact matched excerpts
  score     // sum of query-term occurrences (ranking)
  category  // category derived from the path (e.g. gotchas, architecture)
  checksum  // SHA-256 over the entry's raw bytes; the byte-identity de-dup key
}
```

For the full model — every category definition, the retrieval tiers, the addressing tiers, and the de-duplication rationale — see the [knowledge-base documentation](https://spektacular.dev/knowledge-base/).

### CLI

Agents (and you) reach knowledge through the `spektacular knowledge` commands rather than reading the files directly, so access stays consistent across stores. The main subcommands:

- `knowledge search <query>` — keyword-search the stores the request covers (excluding the always-applied categories), returning ranked, tier- and category-tagged hits, each carrying the entry's tags. A document need not contain every query word: it is returned if it carries evidence for any of them. Narrow with `--tier`, `--filter`, and a repeatable `--tag`
- `knowledge tags` — list the tag vocabulary already in use, with an entry count for each, most-used first; takes `--tier` and `--filter`
- `knowledge conventions` / `knowledge always-applied` — read the always-applied entries in full; both take `--tier` and `--filter`
- `knowledge categories` — list the categories and their retrieval tiers
- `knowledge read` / `knowledge write` — read and write one addressed entry, via `--data '{"tier":"…","name":"…","path":"…"}'`
- `knowledge delete` — remove one addressed entry, via `--data '{"tier":"…","name":"…","path":"…"}'`; an address holding nothing succeeds and changes nothing, and a category's own generated description is refused
- `knowledge list` — list entries across the stores the request covers; takes `--tier` and `--filter`
- `knowledge sources` — list the configured stores by tier and name, with their locations

Every subcommand accepts `--schema` to print its input/output JSON schema and exit.

### Capturing knowledge

When research surfaces a durable learning, gotcha, or convention worth keeping, the agent **proposes** the destination — the tier, the store name, and the path — along with the exact content, and waits for your explicit confirmation before writing; it never persists to a knowledge store unprompted. In a Spektacular-initialised repo, the `spek-knowledge` skill is the entry point for reading, contributing to, and updating the knowledge base in any session, and coding agents route what they would otherwise save to their own per-user memory into the project knowledge base instead, so captured knowledge lands in git and travels with the project.

## Configuration

Configuration is split across two files, and a colocated single-repo project simply holds both in the same `.spektacular/` directory. A repo's Spektacular files can also live apart from its code, in a folder that points at the code (see [Repo configuration](#repo-configuration-repoyaml) below).

- **`.spektacular/config.yaml` (project configuration).** The project's identity, the coding agent Spektacular drives, the registry of member repos with the location of each repo's Spektacular files, the central `spec`, `plan`, and `changelog` stores, and the design sources the project declares. Spektacular always runs against a project: running it in a directory with no `config.yaml` produces an explicit error pointing at `init` (there is no parent-directory search).
- **`.spektacular/repo.yaml` (repo configuration).** A repo's own concerns only: what it is, where its code lives, its knowledge sources, and its changelog provider. It carries no pointer to any project, so one repo can belong to several projects at once.

> **Breaking change**: earlier releases used a single `config.yaml` without a project `name`. Existing setups re-initialize with `spektacular init <agent>`: init backfills the name (from the directory basename, or `--name`), seeds the colocated repo's `repo.yaml`, and registers it in the new `repos` list.

### Project configuration (`config.yaml`)

```yaml
schema: 3                           # settings format version, written by Spektacular; `migrate` raises it
written_by: 0.16.0                  # the Spektacular version that last wrote this file (informational)
skills_version: 0.16.0              # the Spektacular version that last installed the agent skills
name: my-project                    # required, slug-safe; namespaces changelog entries
source: git@example.com:org/my-project.git  # optional; the project's git address, recorded in derived changelog entries only
command: spektacular
agent: claude
auto_commit: "off"                  # off | workflow | full: automatic git commits (local only)
debug:
  enabled: false
spec:
  provider: file
  id_method: timestamp              # how new spec identifiers are generated
  config:
    directory: specs                # relative to the folder holding config.yaml
plan:
  provider: file
  config:
    directory: plans                # relative to the folder holding config.yaml
changelog:
  provider: file
  config:
    directory: changelog            # central changelog; entries land under <directory>/<name>/
repos:
  - name: my-project                # the colocated repo, registered by init: this .spektacular/ folder
    location: .
  - name: docs                      # a repo checked out beside this one, with its own .spektacular/
    location: ../../docs/.spektacular
  - name: lib                       # a repo folder in this project; its code is cloned from a git source
    location: ../repos/lib
knowledge:
  sources:                          # optional, the project's shared stores only (e.g. a team share);
    - name: team                    # each repo declares its own store in its repo.yaml
      provider: file
      config:
        location: ../team-kb        # relative to the folder holding config.yaml, as repos are
design:
  sources:                          # optional, where the project's design documents live;
    - name: api                     # declared by the project only, never by a repo
      provider: file
      config:
        location: ../design/api     # relative to the folder holding config.yaml; may sit outside the project
```

Each repo entry needs a slug-safe unique `name` and a `location`: the folder holding that repo's `repo.yaml` (`local` is still accepted and means the same thing). A relative location is resolved from the folder holding `config.yaml`, and nothing is appended to it, so the project's own footprint is `.` and a repo folder in the project is `../repos/<name>`. A repo is normally added through a guided flow: you are asked which repo to add, and its name, description, role and tags are each proposed for you from what the repo says about itself, one question at a time, with a plain-language confirmation before anything is written. Spektacular's files go inside the repo being added unless it cannot take them or you say otherwise, in which case they live in a folder under the project and the repo is left with only its code. An add can be started and finished while a spec or plan is already in progress. A caller that already knows every detail can still register a repo in a single command with `repo add`. Where the code lives is declared in the repo's own `repo.yaml` as `source`; the old `address` key is no longer read, and a config that still carries it fails to load with an error saying where the value now goes. `description`, `role`, and `tags` are optional metadata, also in `repo.yaml`, that cross-repo planning uses to attribute requirements to the right repo. Add to the registry with `spektacular repo new`, or `spektacular repo add` when every detail is already known, and inspect it with `spektacular repo list`; removal is a manual config edit. Cloned repos are never fetched or pulled automatically; a stale clone produces a warning only.

**Relative locations everywhere in `config.yaml` share one base: the folder holding `config.yaml`.** That covers a repos entry's `location`, a `knowledge.sources` entry's `config.location`, a `design.sources` entry's `config.location`, and the `spec`, `plan` and `changelog` store directories, so `..` is the project's own root and `../team-kb` a folder beside it. The store directories default to `specs`, `plans` and `changelog`, which land in `.spektacular/specs`, `.spektacular/plans` and `.spektacular/changelog`; a store directory outside the project is refused with the corrected value to write. An absolute location is used as written. A knowledge source that does not resolve to a directory fails fast, naming the store, the path it resolved to, and the base it resolved from; when the store is found where the pre-1.0 rule would have put it, the error also names the exact corrected value to write. A design source behaves the same way and fails fast with the same three facts, with one deliberate difference from the store directories: a design source's location is allowed to resolve outside the project, and is written back exactly as declared rather than re-expressed, because a design source points at a folder the team already keeps.

### Repo configuration (`repo.yaml`)

```yaml
schema: 2                         # settings format version, written by Spektacular
written_by: 0.16.0                # the Spektacular version that last wrote this file
description: the documentation repo
role: documentation
tags: [docs]
source:                           # where the code is; omit when it is this folder
  provider: file                  # file or git
  config:
    location: ..                  # a path relative to this file, or a git URL for the git provider
knowledge:                        # the repo's single store; synthesised if the file is absent
  provider: file                    # addressed by the name the project registered this repo under
  config:
    location: knowledge
changelog:
  provider: file
  config:
    directory: changelog            # where this repo's derived entries land
```

A repo's Spektacular files can sit inside its code, in a `.spektacular/` folder holding `repo.yaml` with a file source pointing at `..`, or in a folder of their own, for example one folder per repo under a project, with `source` pointing at a checkout on disk (absolute, relative to the folder holding `repo.yaml`, or using `${VAR}`) or at a git repository that Spektacular clones into `.spektacular/repos/<name>/` on first use. In the separate layout the code repository receives only code changes; knowledge and changelog entries land under the folder holding `repo.yaml`. `spektacular repo list` reports the resolved source as each repo's `root`. See [Multi-Repo Projects](https://spektacular.dev/projects/) for the layouts.

Knowledge aggregates across every registered repo's declared sources (in registry order) followed by the project-owned sources, so a repo's knowledge travels with it into every project that registers it. Changelog entries, central and derived per-repo, are namespaced under a folder named after the project (`<directory>/<project-name>/<id>_<slug>.md`), so multiple projects writing into one repo can never collide.

### Upgrading (`migrate`)

Every settings file records the format version it was written in. When a release changes that format, or when you install a new Spektacular, commands stop with an error naming `migrate` until the project is brought up to date; `migrate`, `init`, `version check` and `help` always run, so the way out is always reachable.

```bash
spektacular migrate --dry-run   # list every change without touching disk
spektacular migrate             # apply them
```

An upgrade brings `config.yaml` and every registered repo's `repo.yaml` present on disk to the current format, keeps a `<file>.v<N>.old` copy of each file it rewrites, names any repo it skipped because it is not checked out, and reinstalls the agent skills when they are older than the running Spektacular. Re-running `spektacular init <agent>` applies the same upgrades. Settings written by a *newer* Spektacular are refused rather than converted, with a message to update Spektacular; such a file is never rewritten.

### Excluding paths (`.spektacular_ignore`)

Any source root (a repo, or the project's own storage locations) may carry a `.spektacular_ignore` file using gitignore pattern syntax. Matching paths are excluded from Spektacular's own listing and search results, keeping build artifacts and dependency directories out of planning research, but a directly named path is never blocked, and agents' native file tools are unaffected.

For the full reference (every key, the id-method semantics, name-normalisation rules, and `${VAR}` expansion) see the [configuration documentation](https://spektacular.dev/configuration/). For the concept of multi-repo projects, why the configuration is split this way, and how work is attributed across repos, see [Multi-Repo Projects](https://spektacular.dev/projects/).

## Testing

Spektacular has two layers of tests: a fast Go unit suite, and an end-to-end [Harbor](https://harborframework.com/) harness that runs the workflows against real AI coding agents inside sandboxed Docker containers.

### Unit tests

```bash
go test ./...   # or: make test
```

### End-to-end (Harbor)

#### Prerequisites

- Docker
- [uv](https://docs.astral.sh/uv/) (Python package manager)

#### Install Harbor

```bash
uv tool install harbor
```

#### Run the oracle (scripted) tests

The oracle agent runs a scripted solution to validate the test harness itself —
no AI tokens required:

```bash
harbor run -p tests/harbor/spec-workflow -a oracle -o tests/harbor/jobs
```

#### Run with a real agent

Harbor needs an auth token to run Claude Code inside the container. If you use
Claude Max (OAuth), export the token from your local credentials:

```bash
export ANTHROPIC_AUTH_TOKEN=$(python3 -c "import json; print(json.load(open('$HOME/.claude/.credentials.json'))['claudeAiOauth']['accessToken'])")
```

If you use an API key instead, export that:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

Then run:

```bash
harbor run -p tests/harbor/spec-workflow -a claude-code -m claude-sonnet-4-6 -o tests/harbor/jobs
```

Makefile wrappers run the suites for you, building the binary and wiring up the agent-specific placeholders:

```bash
make harbor-test-spec            # spec workflow (claude)
make harbor-test-spec-codex      # spec workflow (codex)
make harbor-test-plan            # plan workflow (claude)
make harbor-test-repo            # guided repo add, answering each question (claude)
make harbor-test-repo-delegated  # guided repo add, handing the whole set over (claude)
```

#### Test results

Results are written to `tests/harbor/jobs/` (gitignored). Each run produces:

```
tests/harbor/jobs/<timestamp>/
├── result.json                    # Overall pass/fail and metrics
└── spec-workflow__<id>/
    ├── agent/                     # Agent output log
    ├── verifier/
    │   ├── test-stdout.txt        # pytest output
    │   └── reward.txt             # 1 = pass, 0 = fail
    └── trial.log                  # Full trial log
```

#### Available test tasks

| Task | Description |
|---|---|
| `tests/harbor/spec-workflow` | Full spec creation workflow, end to end |
| `tests/harbor/plan-workflow` | Full plan generation workflow, end to end |
| `tests/harbor/repo-workflow` | Guided repo add, checked against the agent's own transcript |

The repo-workflow suite is the one whose assertions are about the conversation rather than the
files: that only the repo itself is asked for cold, that every later question carries a proposed
value, that the questions arrive one per exchange, and that none of Spektacular's internal
vocabulary reaches the user. Its rules are themselves checked by
`tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py`, which runs locally under plain
pytest with no container and proves each rule fails on a transcript that breaks it:

```bash
python3 -m pytest tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py
```

## Building from Source

```bash
# build binary
make build

# run tests
make test

# cross-compile for all platforms
make cross
```

`make build` produces the binary at `./bin/spektacular`. The `Makefile` targets:

| Target | Description |
|---|---|
| `make build` | Build the `./bin/spektacular` binary |
| `make test` | Run `go test ./...` |
| `make lint` | Run `go vet ./...` |
| `make clean` | Remove build artefacts |
| `make install-local` | Build and copy the binary to `/usr/local/bin` |
| `make cross` | Cross-compile for darwin/linux/windows (amd64 + arm64) |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b my-feature`)
3. Make your changes
4. Run the tests and vet checks (`make test`, `make lint`)
5. Submit a pull request

## License

[Apache 2.0](LICENSE)
