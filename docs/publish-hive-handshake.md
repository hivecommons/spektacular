# Publish ⇄ Hive handshake

`spektacular publish` turns a final spec or epic into one **RFC** item plus a
**child** item per unit of work in a work source (GitHub, Jira or Linear,
behind one `Target` interface). A hive
attached to that work source must pick the children up, build each against
the spek's acceptance criteria, leave the RFC alone, and never route a child
into a fresh Spek `spec` run. This page is the contract both sides read.

Tracked in [spektacular#114](https://github.com/hivecommons/spektacular/issues/114),
part of the [#109 RFC](https://github.com/hivecommons/spektacular/issues/109).
The Hive-side change that reads the marker is being filed concurrently in
`hivecommons/hive`; see hivecommons/hive RFC (link TBD).

## 1. The marker

Every item publish files carries one hidden HTML comment as the **last line
of its body**. The grammar is identical on the RFC and its children; the
difference is which options are present.

```
<!-- spek:publish name=<name> artifact_id=<id> target=<t> -->
<!-- spek:publish name=<name> artifact_id=<id> target=<t> task=<task-or-spec-id> rfc=<url> -->
```

| Option | On | Value |
|---|---|---|
| `name` | both | the spec or epic's bare name, as `spektacular status <name>` takes it |
| `artifact_id` | both | `<project>/<kind>/<name>`, e.g. `spektacular/spec/000112_publish`; unique across the projects publishing into one repository, and what a re-publish searches for (`in:body "spek:publish" "artifact_id=<id>"`) |
| `target` | both | `github`, `jira` or `linear` |
| `task` | children only | the plan task id, the member spec name (an epic's child) or `R<n>` (a requirement of a spec with no plan) |
| `rfc` | children only | the RFC item's URL; absent in `--dry-run` output, present on every filed child |

Options are `key=value`, space separated, values contain no whitespace, and
unknown options must be ignored so either side can add one. A marker
**without `task=`** is the RFC: the umbrella a hive must never claim. A marker
**with `task=`** is a child: it was specced by Spek and stays on the
direct-fix path.

Hive's own markers (`<!-- hive:parent n -->`, `<!-- hive:clarity-gate -->`)
are separate comments and may coexist in the same body.

## 2. Body hygiene

Hive's clarity gate (`runs.triage.clarity_gate`, `crux/issueshape`) parks an
issue it cannot act on with `needs-spec`. Publish refuses, with
`publish_body_hygiene`, any body that would be parked, so what reaches the
work source is always actionable:

- every body (RFC and child) is **at least 80 characters** without its marker
  (hive `min_body_chars`);
- **no unsubstituted `<placeholder>`**: an angle-bracketed word or phrase that
  is not an HTML tag. The scaffold's guidance comments are stripped before
  rendering, so they never reach a body;
- **no unchosen option list** such as `high/medium/low` or `high | medium | low`;
- every child has **at least one acceptance criterion**.

Beyond the gate, the bodies are laid out so a hive's kick has what it needs
without opening Spek's files:

- **RFC body**: the spek's sections in scaffold order (`Overview`,
  `Requirements`, `Constraints`, `Acceptance Criteria`, `Technical Approach`,
  `Success Metrics`, `Non-Goals`, then any others; the amendment log and an
  epic's specs list are dropped), a `## Children` checklist of `- [ ] #n —
  title` (child ids before the children are filed), a provenance footer
  (`Published by Spek · Spek spec <name> · artifact_id <id> · project · source@commit · spektacular version`),
  then the marker.
- **Child body**: `## What to build`, `## Context` (the spec's overview, for a
  task or requirement), `## Acceptance criteria` traced to the child (a task's
  own criteria; a member spec's criteria; for a requirement, the criteria
  whose bold title matches the requirement's, else the one in the same
  position, else all of them), `## Where` (repo, milestone, execution),
  `## Depends on` (`- Blocked by #n — title`), `## Links` (the RFC's URL and
  the spek's name and artifact_id), then the marker.

Titles: the RFC is `RFC: <spek title>` (`RFC: [DRAFT] <title>` when
`--allow-draft` publishes a non-final artifact); a child's title is the task
title, the member spec's title, or the requirement's bold title.

## 3. What Hive looks for, and what publish does

From `hivecommons/hive` at the time of writing; verify against current main.

| Concern | Where in hive | What publish does |
|---|---|---|
| Enumeration | `src/pkg/github/client.go` `fetchIssues`; `src/pkg/github/labels.go` hard-suppresses `hold`, `needs-human`, `needs-decision`, `needs-spec`, `needs-direction`; `project.issue_filter.require_labels` | Children are open issues in the target repo with none of the suppress labels. `publish.github.labels` applies a required label when the repo has it. |
| Stage derivation | `src/pkg/github/issue_stage_labels.go` → `stage/010-actionable` | Nothing; derived. |
| Reporter trust (opt-in) | `src/pkg/config/reporter_trust.go`; `stage/015-triage-needed`; acceptance label `triage/accepted` | The publishing identity is whoever owns the token (`GITHUB_TOKEN`, `GH_TOKEN`, `gh auth token`). Publish from a trusted login or App, else a maintainer adds the triage label. |
| Clarity gate (opt-in) | `runs.triage.clarity_gate`, `crux/issueshape`, `min_body_chars` 80, `<!-- hive:clarity-gate -->`, parks with `needs-spec`; `approved-direction` skips scoring | Section 2. |
| Prior-art gate (opt-in) | `runs.triage.prior_art`; title `RFC:` / `[RFC]` / `Epic:`; parks with `stage/017-possible-existing` | Expected on the RFC: its title starts `RFC:`. Children carry plain titles and no rfc/epic kind. |
| Umbrella never claimed | `governor.claims.never_claim_labels` (default `epic`), `src/pkg/claims/neverclaim.go` | The `epic` label (`publish.github.parent_label`) goes on the RFC **only if the repo has it**; publish never creates labels and warns when it is missing. The marker without `task=` is the second signal. |
| Triage into a spec run (opt-in) | `runs.triage.spec_labels`; `run/fix` forces direct-fix; `src/docs/spektacular.md` → "Triage" | Section 4. |
| Sub-issues / dependencies | `hive-open-issue --parent`, `--blocked-by` (`src/docs/hive-open-issue.md`) | Each child is added with `POST /repos/{o}/{r}/issues/{rfc}/sub_issues` (`sub_issue_id` = the child's database id); each `depends_on` edge with `POST /repos/{o}/{r}/issues/{child}/dependencies/blocked_by` (`issue_id` = the blocker's database id). Both are best effort: a failure is a warning in the result, and `--update` retries. |
| PR linkage | closing keywords `Fixes/Closes/Resolves #N` | The child is the issue the kick receives, so its number is what the PR closes. |
| Jira | `src/docs/work-sources.md` → `type: jira`: `project in (<keys>) AND statusCategory != Done AND issuetype != Epic`, `hold_labels` | The RFC is an **Epic** (`publish.jira.parent_issue_type`) in `publish.jira.project_key`, so hive's default JQL never lists it; each child a **Story** (`publish.jira.child_issue_type`) with `parent` = the Epic on Cloud, or on Data Center, where `parent` is for sub-tasks only, the Epic Link custom field (`publish.jira.epic_link_field`, discovered from `GET field` when unset) = its key, created in the project's default status, whose category is not Done, with `publish.jira.labels` (whitespace → `-`) and never a hold label. Each `depends_on` edge is a `Blocks` issue link (blocker outward, dependant inward); a re-run reads `parent`/Epic Link and `issuelinks` back and refiles whatever is missing. The marker is the last line of the description **and** the issue property `spek.publish` (`{name, artifact_id, target, task, rfc, title, body_digest}`), written in the create request itself; the line before the marker is a search token `spekpub<sha256(artifact_id)[:16]>`, and a re-publish searches `project = <key> AND description ~ "<token>"` (the `issue.property[...]` JQL needs an app to index the property, so it is not used), then reads any issue the artifact's `published:` frontmatter recorded that the search did not return. Cloud uses REST v3 with ADF descriptions and `JIRA_EMAIL` + `JIRA_API_TOKEN`; Data Center (`publish.jira.deployment: datacenter`) REST v2 with wiki markup and `JIRA_PAT`, trusting `JIRA_CA_BUNDLE`. The site is `JIRA_BASE_URL`. |
| Linear | `src/docs/work-sources.md` → `type: linear`: issues of the team in the configured states, `IsTracker` when the issue has children or its marker has no `task=`, `inverseRelations` of type `blocks` | The RFC is an issue in team `publish.linear.team`, each child a **sub-issue** of it (`parentId`), so hive sees the RFC as a tracker twice over and never claims it; the `epic` label (`publish.linear.parent_label`) goes on the RFC **only if the team has it**, `publish.linear.labels` on every issue, team or workspace labels; publish never creates labels. Issues are created in `publish.linear.state` when set (must exist), else the team's **Todo** state, else its first `unstarted` state, else the team default — Backlog/Triage is where a hive does not look. `publish.linear.assignee` (email, display name or name, resolved through `users`) is set on every issue when configured. Each `depends_on` edge is `issueRelationCreate(issueId: blocker, relatedIssueId: dependant, type: blocks)`, so the dependant's `inverseRelations` carries the blocker; a re-run reads `parent { id }` and `inverseRelations` back and refiles any missing parent or relation (`linked` in the result). The marker is the description's last line and the line before it is `<!-- spek:digest <sha256> -->`, the digest of the body as rendered, so a re-publish compares digests and does not depend on Linear returning the Markdown byte for byte; a re-publish finds items with `issues(filter: {team: {key: {eqIgnoreCase}}, description: {contains: "artifact_id=<id>"}}, includeArchived: true)`, paged, so an auto-archived completed RFC is still seen and refused as closed. Not yet: `parent_as: project`. Auth is `LINEAR_API_KEY` sent bare in `Authorization`, as hive does. |
| Crux proxy | `crux/proxy/linear_rules.go`; `crux/issueshape` on `POST /repos/*/issues` | Publishing from inside a hive needs `POST /repos/*/issues`, `PATCH /repos/*/issues/*`, `POST …/sub_issues`, `POST …/dependencies/blocked_by`, `GET /repos/*/labels` and `GET /search/issues` allowed at the publishing agent's mode. On Linear it needs `issueCreate`, `issueUpdate` and `issueRelationCreate`, all allowed at `ModeIssuesOnly` today; queries pass. |

## 4. The "already specced" signal: option (b), the marker

Of the three options #114 weighed — (a) an existing label such as `run/fix`,
(b) a body marker hive's triage learns to read, (c) a new conventional label —
publish implements **(b)**: the marker in section 1 is the signal.

- A child's marker has `task=`: hive's triage keeps it on the **direct-fix
  path** and never opens a `spec` run for it, and the kick can inline the
  `rfc=` link and the spek's `name`/`artifact_id` as context.
- The RFC's marker has no `task=`: it is the **umbrella**, never claimed,
  whether or not the `epic` label existed to apply.

Option (b) is the only one that also carries the spek link into the kick.
The small hive change (reading the marker in the triage pass, `runs.triage`,
`src/docs/spektacular.md` → "Triage") is being filed concurrently in
`hivecommons/hive`; see hivecommons/hive RFC (link TBD). Until it lands, a
hive with `runs.triage.spec_labels` on treats a published child like any
other issue: `run/fix` via `publish.github.labels` keeps it direct-fix.

## 5. Idempotence

A re-run searches the repo for the `artifact_id` marker (`GET /search/issues`,
`repo:o/r is:issue in:body "spek:publish" "artifact_id=<id>"`, pull requests
skipped), parses the markers of every hit, and compares what it would file
against what is filed (bodies normalised for line endings and trailing
whitespace, the RFC checklist and child `rfc=` filled from the existing
numbers). Nothing changed is a success with everything `unchanged`. A change
without `--update` is refused with `publish_conflict` listing the items and
the kind of change (`create`, `title`, `body`); with `--update` the filed items
are patched in place and missing children created and linked. A closed RFC is
`publish_conflict` regardless. `--dry-run` makes no network call at all.

The result is written back to the artifact's frontmatter:

```yaml
published:
  - target: github
    rfc_url: https://github.com/o/r/issues/10
    children:
      - id: sched-1
        url: https://github.com/o/r/issues/11
```

`status <name> --format json` reports it under `epic.published` and
`specs[].published`.

## 6. Still owed under #114

- End-to-end on a test repo with a hive attached (`runs.triage` on): children
  reach `stage/010-actionable`, one is claimed and kicked direct-fix, the RFC
  stays unclaimed, the PR says `Fixes #<child>`.
- The same with `issue_filter.reporter_trust.enabled: true`, documenting what
  an untrusted publishing identity sees.
- The hive and crux changes, cross-linked from #109.
