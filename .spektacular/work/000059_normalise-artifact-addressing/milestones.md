### Milestone 1: One bare name addresses every spec, plan document and changelog record

**What changes**: A feature's bare name, exactly as the workflow records it, reads, writes, deletes and changes the status of its spec and changelog record. Its plan documents are addressed as the feature plus a document name (`plan`, `context`, `research`, `test-plan`). The list commands print exactly those bare names, so anything a list prints can be passed straight back. Old spellings are refused from this point with a clear message and the same command correctly spelled: a name carrying `.md`, or a plan addressed as `feature/plan.md`. Reading a plan with no document name gives an actionable refusal instead of an internal error that leaked a host path. Every shipped skill, workflow step instruction and harbor suite moves to the new spelling in the same milestone, so a full spec → plan → implement run never hits a refusal. A test guard keeps old spellings from creeping back.

**Validation point**:

- The full Go test suite passes.
- For an existing feature, `spec file read <feature>`, `plan file read <feature> plan` and `changelog file read <feature>` all succeed, and each list output round-trips into read.
- The `.md` and joined forms are refused with `unexpected_extension` and a next action that shows the correct command.
- `plan file read <feature>` is refused with `document_required`.
- The spec, plan and implement harbor suites complete without an addressing refusal.

### Milestone 2: Reported locations are relative to the configuration that declares the store

**What changes**: Every storage location the spec, plan and changelog list commands print is relative to the folder holding the configuration file that declares the store, for example `specs/<feature>.md` instead of `.spektacular/specs/<feature>.md`. It looks the same whether or not a changelog `--repo` is named. `plan status` and `implement status` report the plan by address, with `plan_name` and a new `plan_document`, and give its location as a config-relative `plan_path` instead of an absolute host path, so no host path leaks from any of these commands. External callers can rely on `name` as the address and `path` as a portable location.

**Validation point**:

- The full Go test suite passes.
- On this repo, `spec file list`, `plan file list`, `plan file list <feature>` and `changelog file list` (with and without `--repo spektacular`) print paths starting with the configured directory, with no `.spektacular/` prefix and no absolute path.
- `plan status` and `implement status` during a run show `plan_document: "plan"` and a relative `plan_path`.

### Milestone 3: The new addressing and location convention are documented, with a migration note

**What changes**: Users and external callers can find the rules. The CLI README and the documentation site's configuration reference explain that reported locations are relative to the configuration file declaring the store. A new document-command reference page on the site covers:

- how specs, plan documents and changelog records are addressed
- what the list commands print
- both new error codes

A migration note, in the CLI CHANGELOG and on the site, maps each old spelling to its new form and states that the old spellings are now refused. This milestone changes no behaviour.

**Validation point**:

- The docs site builds and type-checks cleanly, and the new page is reachable from the Resources navigation.
- Every command, field and error code on it matches the CLI's actual output on a scratch project.
- The README and CHANGELOG contain the location rule and the migration mapping.
