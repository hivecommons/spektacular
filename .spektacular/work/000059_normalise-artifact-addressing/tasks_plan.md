### Milestone 1: One bare name addresses every spec, plan document and changelog record

#### - [ ] Task: Add the artifact address package
**Id:** d20ef613-ccf0-47b6-84d7-5df0b7e34ab1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Introduce a small package that owns what a document address is: a kind, a feature name and, for plans, a document name. It parses what a caller typed, refuses any segment carrying an extension or a path separator, and refuses a plan document address with no document. It maps a valid address onto the file provider's layout and turns a listed entry back into its bare name. It defines the two new error codes, `unexpected_extension` and `document_required`, and is the only place that knows documents are stored as `.md` files.

*Technical detail:* [context.md#task-add-the-artifact-address-package](./context.md#task-add-the-artifact-address-package)

**Acceptance criteria**:
- [ ] Bare spec, changelog and plan-document addresses parse, and each maps to the exact file location the workflows already write.
- [ ] Any address segment containing a `.` or `/` (such as `x.md`, `x/plan.md` or `specs/x`) is refused as an unexpected extension, and the refusal carries the correctly spelled address.
- [ ] A plan document address with only a feature is refused as document required.
- [ ] A listed file or folder entry turns back into exactly the bare name that parses to it, and entries that are not documents are reported as not addressable.

#### - [ ] Task: Address spec, plan and changelog document commands by bare name
**Id:** 5abaeadf-acd7-41a4-8038-50b6fd0d2e15
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Rework the one command builder behind every `spec file`, `plan file` and `changelog file` verb so it parses its arguments through the address package. Spec and changelog verbs take the bare feature name. Plan document verbs take the feature and the document as two arguments, and `plan file list` lists features, or one feature's documents, by bare name. Old spellings are refused, and the next action restates the same command, with the caller's flags, correctly spelled. A missing document gets a `not_found` pointing at the matching list command. `set-document-status` reports the address and the location separately.

*Technical detail:* [context.md#task-address-spec-plan-and-changelog-document-commands-by-bare-name](./context.md#task-address-spec-plan-and-changelog-document-commands-by-bare-name)

**Acceptance criteria**:
- [ ] Every name printed by listing specs, changelog records (with and without a repo), plans and one plan's documents, passed unchanged to read, returns that document's content, and no listed name ends in an extension.
- [ ] Write, delete and set-document-status succeed with the bare names a list printed and act on the same document read returns. Changelog write, read and list with `--repo` still route to the named repo.
- [ ] Every verb given a name ending in `.md`, or a plan addressed as `feature/plan.md`, fails with `unexpected_extension`, changes nothing in the store, and its next action is the same command correctly spelled.
- [ ] Every plan document verb given only a feature fails with `document_required`, never an internal error. Its message contains no absolute path, and its next action names `plan file list <feature>` and an example read.
- [ ] The ID-prefix check, repo provenance stamping and plan-body validation behave exactly as before for correctly addressed writes.

#### - [ ] Task: Route workflow layout helpers and the walkthrough hint through the address
**Id:** 9e200427-c471-4d7f-af3e-df8795c0659d
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Make the spec, plan and implement workflow packages' existing path helpers delegate to the address package, so workflow steps write exactly the documents the CLI reads by name, and `.md` is spelled out in one place. Update the plan walkthrough's revision hint, which the workflow engine builds in Go, so it tells the agent to rewrite a plan document with the two-argument write form.

*Technical detail:* [context.md#task-route-workflow-layout-helpers-and-the-walkthrough-hint-through-the-address](./context.md#task-route-workflow-layout-helpers-and-the-walkthrough-hint-through-the-address)

**Acceptance criteria**:
- [ ] Every workflow still creates, reads and writes the same spec, plan, context, research, test-plan and changelog files it did before.
- [ ] The walkthrough revision hint names `plan file write <feature> <doc> --from <scratch>` and no longer mentions a `.md` document path.

#### - [ ] Task: Move shipped skills and step instructions to the new addressing
**Id:** 5e105e3b-47be-4a35-9719-a41556c34edd
**Repo:** spektacular
**Depends on:**
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
- 9e200427-c471-4d7f-af3e-df8795c0659d — Route workflow layout helpers and the walkthrough hint through the address
**Execution:** agent

Rewrite every workflow skill, step template and shared partial that tells an agent how to read, write or list a spec, plan document or changelog record so it uses the bare-name and feature-plus-document forms. Update the error next actions that name the list commands. Widen the instruction guard test so any old spelling in any shipped template or rendered instruction fails the build. Update the existing template assertions to the new spellings, then regenerate the installed skill copies and managed agent sections from the templates.

*Technical detail:* [context.md#task-move-shipped-skills-and-step-instructions-to-the-new-addressing](./context.md#task-move-shipped-skills-and-step-instructions-to-the-new-addressing)

**Acceptance criteria**:
- [ ] No shipped skill, step instruction, partial or managed agent section addresses a spec, plan document or changelog record with an extension or a joined path.
- [ ] A test fails if an old spelling is reintroduced in any template, including partials and managed agent sections, or in any rendered agent-facing instruction.
- [ ] The installed Claude and Bob skill copies and the managed AGENTS.md sections in this repo match what the templates now render.

#### - [ ] Task: Update harbor end-to-end suites to the new addressing
**Id:** 8ce57330-46a5-43fd-b48e-a594303a3118
**Repo:** spektacular
**Depends on:**
- 5e105e3b-47be-4a35-9719-a41556c34edd — Move shipped skills and step instructions to the new addressing
**Execution:** agent

The harbor suites carry hand-maintained copies of CLI spellings in their task instructions, scripted solutions and verifier oracles. Update every spec, plan and changelog document command in the spec, plan and implement suites to the new forms, as literals rather than derived from the templates. The same pass fixes the spec suite's scripted spec write, which already used a retired stdin form.

*Technical detail:* [context.md#task-update-harbor-end-to-end-suites-to-the-new-addressing](./context.md#task-update-harbor-end-to-end-suites-to-the-new-addressing)

**Acceptance criteria**:
- [ ] No harbor suite instruction, scripted solution or verifier oracle uses an extension or joined-path address.
- [ ] The implement suite's verifier looks for the bare-name changelog writes the updated step instructions now produce.

#### - [ ] Task: Run the harbor suites against the new addressing
**Id:** 7558648e-d65e-409e-a125-b6d84c6e8bf4
**Repo:** spektacular
**Depends on:**
- 8ce57330-46a5-43fd-b48e-a594303a3118 — Update harbor end-to-end suites to the new addressing
**Execution:** human — needs Docker, the harbor CLI and Claude credentials an agent will not have, and roughly 25 minutes per suite outside CI

Run the spec, plan and implement harbor suites once each to prove that a full spec → plan → implement run, driven only by the shipped instructions, completes without any addressing refusal. Report any failure back so the instruction or oracle can be corrected before the milestone closes.

*Technical detail:* [context.md#task-run-the-harbor-suites-against-the-new-addressing](./context.md#task-run-the-harbor-suites-against-the-new-addressing)

**Acceptance criteria**:
- [ ] The spec, plan and implement harbor suites each pass.
- [ ] None of the three run transcripts contains an `unexpected_extension` or `document_required` refusal.

### Milestone 2: Reported locations are relative to the configuration that declares the store

#### - [ ] Task: Report list locations relative to the declaring configuration
**Id:** b741574a-ce76-41f8-a559-82e188d1ec7f
**Repo:** spektacular
**Depends on:**
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
**Execution:** agent

Make every `path` printed by the spec, plan and changelog list commands, and by set-document-status, relative to the folder that holds the configuration file declaring the store. For the central stores that is the folder holding config.yaml, and for a repo's changelog it is the folder holding repo.yaml. The store resolver hands back that base with the store, and one routine renders every reported location, so the convention is identical with and without `--repo`.

*Technical detail:* [context.md#task-report-list-locations-relative-to-the-declaring-configuration](./context.md#task-report-list-locations-relative-to-the-declaring-configuration)

**Acceptance criteria**:
- [ ] Locations printed by listing specs, plans, a plan's documents and changelog records start with the store's configured directory, and contain neither the project's hidden settings folder prefix nor an absolute path.
- [ ] Changelog locations follow the same convention whether or not a repo is named.
- [ ] Set-document-status reports its location in the same convention.

#### - [ ] Task: Report the plan by address and relative location in status
**Id:** 364d550d-1546-4e57-98f8-b6b7ccdd5865
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Make `plan status` and `implement status` report the plan by address, with the existing `plan_name` and a new `plan_document` that is always `plan`. Replace today's absolute `plan_path` with the plan's location relative to the declaring configuration. Update both commands' published output schemas to match. The named `plan status <feature>` form, which already reports the bare name and no location, is unchanged.

*Technical detail:* [context.md#task-report-the-plan-by-address-and-relative-location-in-status](./context.md#task-report-the-plan-by-address-and-relative-location-in-status)

**Acceptance criteria**:
- [ ] During an active plan or implement run, the status output shows the bare `plan_name`, `plan_document` of `plan`, and a `plan_path` such as `plans/<feature>/plan.md`.
- [ ] No absolute host path appears in either status output.
- [ ] Both commands' schema output lists `plan_document` and describes `plan_path` as config-relative.
- [ ] The feature name recorded in workflow state reads the feature's spec, `plan` document and changelog record with no string manipulation.

### Milestone 3: The new addressing and location convention are documented, with a migration note

#### - [ ] Task: Document addressing, locations and migration in the CLI repo
**Id:** f76db69e-9c1c-42dc-9a47-ff7b956cf8e4
**Repo:** spektacular
**Depends on:**
- 5e105e3b-47be-4a35-9719-a41556c34edd — Move shipped skills and step instructions to the new addressing
- b741574a-ce76-41f8-a559-82e188d1ec7f — Report list locations relative to the declaring configuration
- 364d550d-1546-4e57-98f8-b6b7ccdd5865 — Report the plan by address and relative location in status
**Execution:** agent

Update the README so it documents how specs, plan documents and changelog records are addressed. It also covers what the list and status commands report, the two new error codes, and the rule that reported locations are relative to the configuration file declaring the store. Add a breaking-change migration note to the CHANGELOG that maps each old spelling to its new form and says the old spellings are now refused. Update the examples in the store-access knowledge entry, through the knowledge CLI, if they spell a document address.

*Technical detail:* [context.md#task-document-addressing-locations-and-migration-in-the-cli-repo](./context.md#task-document-addressing-locations-and-migration-in-the-cli-repo)

**Content example** (README, under the existing status and list paragraph):

```markdown
### Addressing specs, plans and changelog records

Every document a feature produces is addressed by the feature's bare name, the same
name the workflow records, with no file extension:

    spektacular spec file read 000059_normalise-artifact-addressing
    spektacular plan file list 000059_normalise-artifact-addressing      # plan, context, research, test-plan
    spektacular plan file read 000059_normalise-artifact-addressing plan
    spektacular changelog file read 000059_normalise-artifact-addressing [--repo <name>]

Every `name` a list prints is accepted unchanged by read, write, delete and
set-document-status. `path` is where the store keeps the document, relative to the
folder holding the configuration file that declares the store (`specs/<name>.md`,
`plans/<name>/plan.md`, or `changelog/<project>/<name>.md` in a repo's changelog).

A name with an extension, or a plan written as one path (`<name>/plan.md`), is refused
with `unexpected_extension`; `plan file read <name>` with no document is refused with
`document_required`. Both refusals' `next_action` shows the correct command.
```

**Content example** (CHANGELOG entry):

```markdown
**Breaking change**: spec, plan and changelog document commands no longer accept file
extensions or joined plan paths. `spec file read <name>.md` is now `spec file read <name>`;
`plan file read <name>/plan.md` is now `plan file read <name> plan` (likewise `write`,
`delete`, `set-document-status`); `changelog file read <name>.md` is now
`changelog file read <name>`. The old spellings are refused with `unexpected_extension`,
whose `next_action` shows the new command. List `path` values and the `plan_path`
reported by `plan status` and `implement status` are now relative to the folder holding
the declaring config file (e.g. `specs/<name>.md`), and both status commands add
`plan_document`.
```

**Acceptance criteria**:
- [ ] The README describes bare-name addressing for specs, plan documents and changelog records, the list and status output, both new error codes and the config-relative location rule, with no em dashes.
- [ ] The CHANGELOG carries a breaking-change note mapping every old spelling to its new form and stating that old spellings are refused.
- [ ] Every command shown in the README and CHANGELOG succeeds, or is refused, exactly as documented when run against this repo.

#### - [ ] Task: Publish the document command reference and migration note on the docs site
**Id:** 36248681-b4f7-45f2-8df2-7c405d0b5efb
**Repo:** docs
**Depends on:**
- b741574a-ce76-41f8-a559-82e188d1ec7f — Report list locations relative to the declaring configuration
- 364d550d-1546-4e57-98f8-b6b7ccdd5865 — Report the plan by address and relative location in status
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
**Execution:** agent

Add a new documentation page, linked from the Resources navigation, that is the command reference for spec, plan and changelog documents. It covers the addressing rules, what the list commands print, both new error codes, and a migration section for external callers. Update the configuration reference so the spec, plan and changelog store keys say that reported locations are relative to the configuration file declaring the store. Update the plan-tasks page's status example to show `plan_name`, `plan_document` and the relative `plan_path`.

*Technical detail:* [context.md#task-publish-the-document-command-reference-and-migration-note-on-the-docs-site](./context.md#task-publish-the-document-command-reference-and-migration-note-on-the-docs-site)

**Content outline** (new page `Documents`, `src/pages/documents.mdx`):

1. **Hero**: "Addressing specs, plans and changelogs". Sub: "One name, the feature's, reaches every document a feature produces."
2. **Section `Addressing documents: the rules`** (`surface={false}`), prose plus a list:
   - A document is addressed by the feature's bare name, exactly as the workflow records it.
   - A plan document is the feature plus a document name: `plan`, `context`, `research`, `test-plan`.
   - A name never carries a file extension.
   - What a list prints as `name` is exactly what read, write, delete and set-document-status accept.
3. **Section `Specs: spec file`** (`surface={true}`), fenced example:
   ```bash
   spektacular spec file list
   spektacular spec file read 000059_normalise-artifact-addressing
   spektacular spec file write 000059_normalise-artifact-addressing --from .spektacular/tmp/spec.md
   ```
4. **Section `Plans: plan file`** (`surface={false}`), fenced example plus sample list output:
   ```bash
   spektacular plan file list 000059_normalise-artifact-addressing
   spektacular plan file read 000059_normalise-artifact-addressing plan
   ```
   ```json
   { "files": [ { "name": "plan", "path": "plans/000059_normalise-artifact-addressing/plan.md", "modified_at": "2026-09-25T12:00:00Z" } ] }
   ```
5. **Section `Changelog records: changelog file`** (`surface={true}`): the bare-name read with and without `--repo`, and the two `path` shapes (`changelog/<name>.md`, and `changelog/<project>/<name>.md` in a repo).
6. **Section `Where documents are stored: name and path`** (`surface={false}`): `name` is the address, `path` is the location relative to the folder holding the declaring config file. Also shows the `plan status` fields `plan_name`, `plan_document` and `plan_path`.
7. **ConfigurationKeys `Error codes`** (`surface={true}`), one `ConfigKey` per code:
   - `unexpected_extension`: the name carried an extension or a joined plan path. `next_action` shows the command correctly spelled. Includes a sample JSON envelope for `spec file read x.md`.
   - `document_required`: a plan document command got a feature and no document. `next_action` names `plan file list <feature>` and an example read.
8. **Section `Upgrading from earlier spellings`** (`surface={false}`): an old → new mapping table for read, write, delete, set-document-status and list across all three stores, a statement that the old forms are refused, and a note that list `path` and status `plan_path` are now config-relative.
9. **CtaBanner** linking to Configuration.

**Content example** (configuration.mdx, appended to each of the `spec`, `plan` and `changelog` ConfigKey bodies):

```mdx
  Locations the CLI reports for these documents (a list's `path`, status `plan_path`)
  are relative to the folder holding `config.yaml`, for example `specs/<name>.md`.
  Documents are addressed by name, never by this location: see [Documents](/documents/).
```

**Acceptance criteria**:
- [ ] A reader can find a Documents page from the Resources menu that explains how specs, plan documents and changelog records are addressed, what list prints, both new error codes, and how to migrate from the old spellings.
- [ ] The configuration reference states that reported locations are relative to the folder holding the configuration file that declares the store.
- [ ] The plan-tasks page's status example shows `plan_name`, `plan_document` and a relative `plan_path`.
- [ ] The site builds and type-checks cleanly, page bodies carry no layout HTML, and no authored text uses em dashes.
- [ ] Every command, output field and error code shown matches the CLI's real behaviour.
