# Publish ⇄ Hive handshake

`spektacular publish` turns a final spec or epic into one **RFC** item plus a
**child** item per unit of work in a work source (GitHub, Jira or Linear,
behind one `Target` interface). A hive
attached to that work source must pick the children up, build each against
the spek's acceptance criteria, leave the RFC alone, and never route a child
into a fresh Spek `spec` run. This page is the contract both sides read.

Tracked in [spektacular#114](https://github.com/hivecommons/spektacular/issues/114),
part of the [#109 RFC](https://github.com/hivecommons/spektacular/issues/109).
Hive's implementation is tracked in [hive#17018](https://github.com/hivecommons/hive/issues/17018)
and shipped on `v5` in [hive#17019](https://github.com/hivecommons/hive/pull/17019).

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

Spek checks body hygiene before filing and refuses bad output with
`publish_body_hygiene`. Hive's optional clarity gate skips items with a
valid Spek publish marker, but Crux still checks GitHub issue writes for
template placeholders and mis-escaped newlines.

- every body (RFC and child) is **at least 80 characters** without its marker;
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

References checked against [Hive `v5` at `315ca92`](https://github.com/hivecommons/hive/tree/315ca92178501a297c66275bc10fdfec1b9a7a39)
and [Crux `main` at `9e941b1`](https://github.com/hivecommons/crux/tree/9e941b13cba3d518e6418375fe99d5abb66de412).

| Concern | Where | What publish does |
|---|---|---|
| Enumeration | Hive `src/pkg/github/client.go` (`fetchIssues`), `src/pkg/github/labels.go`, `project.issue_filter.require_labels` | Children are open issues in the configured repo. Use `publish.github.labels` for existing required labels. Don't apply `hold` or a parking label such as `needs-human`, `needs-spec`, `needs-decision`, `needs-direction` or `stage/045-needs-decision`. The marker does not override admission filters. |
| Stage derivation | Hive `src/pkg/github/issue_stage_labels.go` | Hive derives `stage/010-actionable` for eligible children and `stage/020-claimed` on a claim without PR evidence. Spek does not apply stage labels. |
| Reporter trust (opt-in) | Hive `src/pkg/config/reporter_trust.go`, `src/pkg/github/client.go` | GitHub sees the credential's publishing identity. An untrusted human reporter's child waits at `stage/015-triage-needed` until a maintainer applies `triage/accepted` (or a configured acceptance label). The normal required-label filter still applies. Hive/bot-filed issues use a separate gate. |
| Clarity gate (opt-in) | Hive `src/pkg/scheduler/clarity_gate.go`; Crux `proxy/github_proxy_violations.go` | A valid marker skips Hive clarity scoring, not Crux's write validation. See section 2. |
| Prior-art gate (opt-in) | Hive `runs.triage.prior_art`, `src/docs/spektacular.md` | The `RFC:` title makes the parent eligible for the prior-art check and its `stage/017-possible-existing` park. Don't give children RFC/epic titles or kind labels. |
| Umbrella never claimed | Hive `src/pkg/claims/neverclaim.go` | The marker without `task=` is sufficient on marker-aware Hive. Spek also applies the existing `publish.github.parent_label` (default `epic`) if present; it does not create labels. |
| Triage into a spec run (opt-in) | Hive `src/pkg/classify/triage.go` | The marker routes a child to direct-fix without `run/fix`, unless `run/spec` is explicitly applied. See section 4. |
|       Sub-issues / dependencies |                                                                                                                           `hive-open-issue --parent`, `--blocked-by` (`src/docs/hive-open-issue.md`) |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   Each child is added with `POST /repos/{o}/{r}/issues/{rfc}/sub_issues` (`sub_issue_id` = the child's database id); each `depends_on` edge with `POST /repos/{o}/{r}/issues/{child}/dependencies/blocked_by` (`issue_id` = the blocker's database id). Both are best effort: a failure is a warning in the result, and `--update` retries. |
|                      PR linkage |                                                                                                                                                          closing keywords `Fixes/Closes/Resolves #N` |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              The child is the issue the kick receives, so its number is what the PR closes. |
|                            Jira |                                                                     `src/docs/work-sources.md` → `type: jira`: `project in (<keys>) AND statusCategory != Done AND issuetype != Epic`, `hold_labels` |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        The RFC is an **Epic** (`publish.jira.parent_issue_type`) in `publish.jira.project_key`, so hive's default JQL never lists it; each child a **Story** (`publish.jira.child_issue_type`) with `parent` = the Epic on Cloud, or on Data Center, where `parent` is for sub-tasks only, the Epic Link custom field (`publish.jira.epic_link_field`, discovered from `GET field` when unset) = its key, created in the project's default status, whose category is not Done, with `publish.jira.labels` (whitespace → `-`) and never a hold label. Each `depends_on` edge is a `Blocks` issue link (blocker outward, dependant inward); a re-run reads `parent`/Epic Link and `issuelinks` back and refiles whatever is missing. The marker is the last line of the description **and** the issue property `spek.publish` (`{name, artifact_id, target, task, rfc, title, body_digest}`), written in the create request itself; the line before the marker is a search token `spekpub<sha256(artifact_id)[:16]>`, and a re-publish searches `project = <key> AND description ~ "<token>"` (the `issue.property[...]` JQL needs an app to index the property, so it is not used), then reads any issue the artifact's `published:` frontmatter recorded that the search did not return. Cloud uses REST v3 with ADF descriptions and `JIRA_EMAIL` + `JIRA_API_TOKEN`; Data Center (`publish.jira.deployment: datacenter`) REST v2 with wiki markup and `JIRA_PAT`, trusting `JIRA_CA_BUNDLE`. The site is `JIRA_BASE_URL`. |
|                          Linear |  `src/docs/work-sources.md` → `type: linear`: issues of the team in the configured states, `IsTracker` when the issue has children or its marker has no `task=`, `inverseRelations` of type `blocks` | The RFC is an issue in team `publish.linear.team`, each child a **sub-issue** of it (`parentId`), so hive sees the RFC as a tracker twice over and never claims it; the `epic` label (`publish.linear.parent_label`) goes on the RFC **only if the team has it**, `publish.linear.labels` on every issue, team or workspace labels; publish never creates labels. Issues are created in `publish.linear.state` when set (must exist), else the team's **Todo** state, else its first `unstarted` state, else the team default — Backlog/Triage is where a hive does not look. `publish.linear.assignee` (email, display name or name, resolved through `users`) is set on every issue when configured. Each `depends_on` edge is `issueRelationCreate(issueId: blocker, relatedIssueId: dependant, type: blocks)`, so the dependant's `inverseRelations` carries the blocker; a re-run reads `parent { id }` and `inverseRelations` back and refiles any missing parent or relation (`linked` in the result). The marker is the description's last line and the line before it is `<!-- spek:digest <sha256> -->`, the digest of the body as rendered, so a re-publish compares digests and does not depend on Linear returning the Markdown byte for byte; a re-publish finds items with `issues(filter: {team: {key: {eqIgnoreCase}}, description: {contains: "artifact_id=<id>"}}, includeArchived: true)`, paged, so an auto-archived completed RFC is still seen and refused as closed. `publish.linear.parent_as: project` instead files the RFC as a project on that team and children with `projectId`. Its full Markdown (including digest and marker) is stored in project `content`; project `description` is only a short summary. Project mode applies issue labels only to children, since project labels are a separate namespace. Projects are found by paging the team's projects (including archived) and reading their markers. Project membership is read back and repaired like the issue parent. Changing `parent_as` for a published artifact is refused to avoid a second RFC. Auth is `LINEAR_API_KEY` sent bare in `Authorization`, as hive does. |
| Crux proxy | Crux `proxy/rules.go`, `proxy/linear_rules.go`, `proxy/github_proxy_violations.go` | At `ModeIssuesOnly` or above, the rules admit GitHub issue create/update and the two native relation POSTs, and Linear `issueCreate`, `issueUpdate`, `issueRelationCreate`, `projectCreate` and `projectUpdate`. Reads are allowed at `ModeAdvisory`. No additional allow-table change is needed for these calls; the publishing agent still needs the corresponding write permission. |

The project API uses the public [Linear schema](https://github.com/linear/linear/blob/master/packages/sdk/src/schema.graphql):
`ProjectCreateInput.teamIds` selects the team, `content` holds full Markdown,
`IssueCreateInput.projectId` attaches children, and `projectUpdate` rewrites the
RFC checklist. `ProjectFilter` exposes no public description comparator, so
lookup pages `projects` filtered by `accessibleTeams.some.key` and checks the
marker locally. The RFC keeps its `RFC:` title in both modes; a project's
`rfc_key` is its UUID rather than an issue identifier.

## 4. The "already specced" signal: option (b), the marker

Of the three options #114 weighed — (a) an existing label such as `run/fix`,
(b) a body marker hive's triage learns to read, (c) a new conventional label —
publish implements **(b)**: the marker in section 1 is the signal.

- A child's marker has `task=`: triage keeps it on the **direct-fix path**
  even when a spec label or complexity tier would otherwise start a new
  run. An explicit `run/spec` still overrides it. The kick identifies
  the spec, artifact and task, links the RFC when Hive accepts its URL,
  and instructs the agent to build against the acceptance criteria and
  write `Fixes #<child>` in the PR body.
- The RFC's marker has no `task=`: Hive treats it as a **tracker**, not
  claimable work, even without the `epic` label.

On an older Hive that does not read the marker, `run/fix` in
`publish.github.labels` can keep children on the direct-fix path if the
label exists in the repo. The RFC needs the repo's configured never-claim
label (default `epic`) independently.

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

## 6. What is verified, and what is still owed under #114

`make e2e` (`tests/e2e`, also run by `go test ./...` and so by CI) builds the
binary and runs `spektacular publish` against in-process fakes of GitHub,
Jira Cloud, Jira Data Center and Linear. On every target it asserts the
Spek side of this contract: the RFC title starts `RFC:`, its marker has no
`task=` and ends the body, every child carries a marker with `task=` and
the RFC URL, hangs off the RFC (sub-issue, Epic parent or Data Center Epic
Link, Linear parent), clears the clarity gate (≥ 80 chars once the hidden
marker and digest lines are stripped, no `<placeholders>`), carries the
configured label and none of hive's suppress labels and never the parent
label, each `depends_on` edge is filed as the target's blocked-by relation,
the RFC checklist has a line for every child, the publication and its URLs
land in the spec's frontmatter, git provenance is recorded, a dry run makes
no network call, and a configured suppress label is refused before any call
is made. Idempotence is checked on both sides: a re-run reports everything
unchanged and the fake holds the same issue and relation counts; a changed
spec is refused as a conflict until `--update`, which updates the RFC in
place and files nothing new; a closed (Done, completed-and-archived) RFC is
refused. The Jira fake runs with a lagging search index throughout, so the
re-run is shown to find its issues through the recorded URLs and not the
search.

The fakes are models of the APIs from their documentation, not recordings
of them, so what they prove is that Spek sends the calls the documentation
asks for; they cannot prove that a real Jira Data Center build accepts the
Epic Link field as modelled or that Linear keeps HTML comments in a
description verbatim. Those are listed as caveats on
[#117](https://github.com/hivecommons/spektacular/pull/117) and
[#118](https://github.com/hivecommons/spektacular/pull/118).

### Live GitHub acceptance check

The live cases below are not verified by `make e2e`. Use a maintainer-approved
test repository attached to a Hive running the marker implementation
linked above. Do not publish test work into a production queue.

1. Use a test repository watched through GitHub Issues, with
   `runs.triage.enabled: true` and reporter trust initially off. Use a final
   test spec with a small, verifiable change and at least two tasks, one
   depending on the other. Configure an existing child label that matches
   `runs.triage.spec_labels` (and satisfy `require_labels` if set). Don't
   use `run/fix` or `run/spec`: either would mask the marker's routing.
2. Run `spektacular publish <name> --target github --repo <owner>/<test-repo>`.
   Check that the native sub-issue and blocked-by links exist in GitHub
   and that the publish result has no relation warnings. Record the RFC
   and child URLs.
3. On the runnable child, record `stage/010-actionable`, then
   `stage/020-claimed` (before the PR is started). Record the direct-fix
   kick with the RFC link, spec/task context and acceptance criteria
   instruction; check that no new Spek `spec` run was admitted for that
   child. Check that the RFC remains unclaimed while the child is worked.
   Inspect the resulting PR for the child number in `Fixes #<child>` and
   check its change against the child's acceptance criteria. The PR
   need not merge to collect this evidence.
4. Enable `project.issue_filter.reporter_trust.enabled` and publish a
   fresh artifact as a human whose association and login are not trusted.
   Don't pre-apply an acceptance label or publish through a bot/App for
   this case. Expect `stage/015-triage-needed` and no claim or kick. Have
   a maintainer apply `triage/accepted` (or the configured equivalent) to
   the runnable child. Repeat the actionable, claim, kick and PR checks
   from step 3. Record the publishing login, settings, issue/PR URLs and
   stage and kick evidence for both runs on #114.

