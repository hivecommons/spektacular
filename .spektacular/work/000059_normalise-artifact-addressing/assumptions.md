### Extension detection is any `.` in a name (discovery)
- **Decision**: a feature or document address containing `.` is refused as `unexpected_extension`; one containing `/` (plan joined path, or any path in a spec/changelog name) is refused with the same code.
- **Rationale**: feature names are validated `^[a-z0-9_-]+$` (`cmd/spec.go:22`) and plan document names are `plan|context|research|test-plan`, so a `.` can only be an extension and a `/` only a joined path.
- **Rejected**: only refusing a trailing `.md` (misses `.markdown`/`.txt` and leaves the "names never carry an extension" rule half-enforced).

### Config-relative location means relative to the folder holding the declaring config file (discovery)
- **Decision**: central spec/plan/changelog `path` = path relative to `.spektacular/` (the config.yaml folder), e.g. `specs/x.md`; repo-routed changelog `path` = relative to the repo.yaml folder (unchanged, e.g. `changelog/<project>/x.md`).
- **Rationale**: matches the acceptance criterion ("start with the store's configured directory", no `.spektacular/` prefix, no absolute path) and the existing `--repo` output.
- **Rejected**: knowledge-style store-root-relative paths (`x.md`), which would not start with the configured directory.

### Step PathVars and new/goto results stay absolute (discovery)
- **Decision**: only list commands and `plan status` / `implement status` change location convention; `{{plan_path}}`/`{{spec_path}}` template vars and `plan new`/`goto` result `plan_path` are unchanged.
- **Rationale**: spec scopes the location requirement to those commands; step paths are agent-facing and the workflow commands are a non-goal.
- **Rejected**: converting every reported path (larger blast radius, not required).

### Chosen direction: address layer in a new `internal/artifact` package driving the one command builder (architecture)
- **Decision**: add `internal/artifact` (Address, Parse, StorePath, NameFromEntry, two ErrCode consts); make `newStoreFileCmd` address-driven; existing step layout helpers delegate to it; refusals rendered in the command builder with a next_action that rebuilds the same command correctly spelled (including the flags the caller set).
- **Rationale**: one builder serves every verb/kind (cmd/storefile.go:181); a single layout mapping keeps `.md` in one place (user prefers DRY); the builder holds the facts the remediation needs.
- **Rejected**: tolerant normalisation (violates the hard-break constraint); addressing inside `store.Store` (widens a backend contract for a naming concern); ad-hoc per-verb parsing in cmd (duplicates validation 15 times).

### Both refusal shapes use `unexpected_extension` (architecture)
- **Decision**: a `.` or `/` in any address segment, including a joined plan path, is `unexpected_extension`; `document_required` covers every plan document verb (read, write, delete, set-document-status) given only a feature.
- **Rationale**: spec defines exactly two codes and says joined paths are refused "with a distinct, documented error code"; applying `document_required` to all plan doc verbs keeps the verbs uniform.
- **Rejected**: a third `joined_path` code (not in the spec); `document_required` on read only (inconsistent verbs).

### delete / set-document-status keep central-only changelog routing (architecture)
- **Decision**: do not add `--repo` to `changelog file delete` / `set-document-status`; they take the bare name on the central store as today.
- **Rationale**: the spec is about addressing, not new routing; those verbs have never taken `--repo`.
- **Rejected**: adding `--repo` everywhere (new capability, out of scope).

### Status changes apply to the workflow-status forms only (architecture)
- **Decision**: `plan status` (no arg) and `implement status` gain `plan_document` and report `plan_path` config-relative; the named `plan status <name>` artifact form (which reports `name` and no path) is unchanged.
- **Rationale**: the named form already reports the bare name and no location; the acceptance criterion targets the forms that report `plan_path`.
- **Rejected**: adding plan_path to the shared artifact-status result (would change spec status too, not required).

### set-document-status output splits name and path (architecture)
- **Decision**: output `{"name", "document"?(plan), "path"(config-relative), "document_status", "closed_date"?}` instead of echoing the raw argument as `path`.
- **Rationale**: "name is the address, path is the location" applies to every command output; echoing an address as `path` would conflate them.
- **Rejected**: leaving `path: args[0]` (now a bare name mislabelled as a path).

### List drops non-address entries (architecture)
- **Decision**: spec/changelog lists and `plan file list <feature>` report only `.md` files (as bare names); top-level `plan file list` reports only feature directories.
- **Rationale**: "every listed name is accepted by read"; a directory or non-document file is not readable by address.
- **Rejected**: keeping every raw entry (would print names read refuses).

### Docs: a new document-command reference page on the site (architecture)
- **Decision**: add `src/pages/documents.mdx` (Resources nav) covering spec/plan/changelog file commands, addressing rules, list output, both error codes and an "Upgrading from earlier spellings" migration section; update configuration.mdx store keys, plan-tasks.mdx status sample, README and CHANGELOG (Breaking change paragraph).
- **Rationale**: no command reference, error-code list or migration page exists anywhere; the spec requires all three.
- **Rejected**: a site-wide command reference for every command (scope creep); README only (spec requires the site).

### Plan verbs relax cobra arity so one-arg calls get `document_required` (implementation_detail)
- **Decision**: plan document verbs use `RangeArgs(1,2)` and let the address parser refuse; spec/changelog keep `ExactArgs(1)`.
- **Rationale**: cobra's "accepts 2 arg(s)" error surfaces as an internal error (live repro in spec research) — exactly what the spec forbids.
- **Rejected**: `ExactArgs(2)` (produces the generic cobra error).

### Replace positional booleans on the builder with a per-kind descriptor (implementation_detail)
- **Decision**: `newStoreFileCmd` takes a kind descriptor struct instead of `(short, dir, requireID, repoRouted, validate)`.
- **Rationale**: the builder now needs the kind for parsing and next actions; a struct avoids a sixth positional arg.
- **Rejected**: adding another bool/string parameter.

### Milestone split: addressing + all callers together, then locations/status, then docs (milestones)
- **Decision**: M1 bundles the CLI addressing change with every shipped caller and the harbor suites; M2 locations/status; M3 docs.
- **Rationale**: the hard-break constraint requires shipped instructions to move in the same change as the refusal, so M1 is only independently deliverable if callers are in it; locations/status are a separate user-visible change with no caller impact on templates; docs change no behaviour.
- **Rejected**: CLI-first then callers (M1 alone would ship refusing instructions); one giant milestone (no intermediate validation point).

### Harbor runs are a human task (tasks)
- **Decision**: running the three harbor suites is `human`, split from the agent task that updates the suites.
- **Rationale**: needs Docker, the harbor CLI and Claude credentials, ~25 min each, outside CI.
- **Rejected**: folding the run into the agent task (agent likely lacks the credentials).

### Spec/changelog `list` stops accepting a positional sub-path (tasks)
- **Decision**: `spec file list` and `changelog file list` take no positional argument; `plan file list [<feature>]` takes an optional feature.
- **Rationale**: a sub-path is a storage location, not an address; with bare names there is nothing to list below a spec or changelog record.
- **Rejected**: keeping `[path]` (would keep a path-addressed input alive).
