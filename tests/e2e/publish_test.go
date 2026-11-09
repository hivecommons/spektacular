// Package e2e runs the built spektacular binary against fake GitHub, Jira
// and Linear servers and asserts the handshake docs/publish-hive-handshake.md
// promises a hive: one RFC it never claims, one child per unit of work with a
// marker and the parent/dependency relations hive reads, idempotent re-runs,
// and a dry run that touches no network. It needs no credentials and runs
// with `go test ./...` (and `make e2e`).
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/github/githubtest"
	"github.com/hivecommons/spektacular/internal/publish/jira/jiratest"
	"github.com/hivecommons/spektacular/internal/publish/linear/lineartest"
)

// minBodyChars is hive's clarity-gate floor (runs.triage.clarity_gate),
// applied to what a reader sees: HTML comments do not count.
const minBodyChars = 80

// placeholderRE is what hive's clarity gate reads as an unfilled template;
// the marker's own angle brackets are stripped before the check.
var placeholderRE = regexp.MustCompile(`<[a-z][a-z -]*>`)

// htmlCommentRE strips the marker and digest lines before a body is
// measured as a hive's clarity gate would measure it.
var htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->\n?`)

// configuredLabel goes on every filed item in each handshake, so the
// no-suppress-label check below runs over a real label on every target.
const configuredLabel = "spek"

var (
	binary string
	// toolPath is a PATH holding git alone, so provenance is recorded but
	// no gh is found: the github target must not fall back to `gh auth token`.
	toolPath string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "spek-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "spektacular")
	build := exec.Command("go", "build", "-o", binary, "../..")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building spektacular:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	if err := dependOnSources(); err != nil {
		fmt.Fprintln(os.Stderr, "reading the module's sources:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	toolPath = filepath.Join(dir, "bin")
	if git, err := exec.LookPath("git"); err == nil {
		_ = os.MkdirAll(toolPath, 0o755)
		_ = os.Symlink(git, filepath.Join(toolPath, "git"))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// dependOnSources stats every Go source and module file in the repository.
// This package imports none of cmd, so without it the test cache would
// hand back a stale pass after a change to the binary's code; go test
// records the files a test stats and reruns it when any of them changes.
func dependOnSources() error {
	return filepath.WalkDir("../..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" || name == "tests" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum" {
			_, err = os.Stat(path)
		}
		return err
	})
}

const specBody = `# Feature: Nightly digest email

## Overview

Operators want one email per day summarising what the hive filed, merged and
parked, so they can skim the night's work without opening the dashboard.

## Requirements

- [ ] **Digest schedule**: send one digest per day at the configured hour.
- [ ] **Digest content**: list filed, merged and parked items with links.

## Acceptance Criteria

- [ ] **Digest schedule**: a digest arrives within five minutes of the hour.
- [ ] **Digest content**: every item of the last 24 hours appears once.
`

// planBody gives the spec two tasks, the second depending on the first, so
// the dependency relation is exercised on every target.
const planBody = `# Plan: nightly-digest

## Milestones & Tasks

### Milestone 1: Digest can be sent

#### - [ ] Task: Add the digest scheduler
**Id:** sched-1
**Repo:** testproj
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] a digest job runs at the configured hour

#### - [ ] Task: Render the digest email
**Id:** render-2
**Repo:** testproj
**Depends on:**
- sched-1 — Add the digest scheduler
**Execution:** agent

**Acceptance criteria**:
- [ ] filed, merged and parked items each have a section
`

// project writes a Spek project with one final spec and its plan, plus the
// publish config given, and returns its root.
func project(t *testing.T, publishConfig string) string {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "specs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "plans", "nightly-digest"), 0o755))
	cfg := fmt.Sprintf("schema: %d\nskills_version: dev\nname: testproj\nrepos:\n  - name: testproj\n    location: .\n%s", config.CurrentProjectSchema, publishConfig)
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, config.ProjectConfigFileName), []byte(cfg), 0o644))
	rc := config.NewDefaultRepoConfig()
	rc.Source = config.DefaultRepoSource
	require.NoError(t, rc.ToYAMLFile(filepath.Join(dataDir, config.RepoConfigFileName)))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "specs", "nightly-digest.md"),
		[]byte("---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n"+specBody), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "plans", "nightly-digest", "plan.md"),
		[]byte("---\ncreated_date: 2026-02-02\ndocument_status: final\nspec: nightly-digest\n---\n\n"+planBody), 0o644))
	// A commit to record as provenance, as a real project has.
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=e2e", "-c", "user.email=e2e@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		git := exec.Command("git", args...)
		git.Dir = root
		git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := git.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return root
}

// result is the publish command's JSON output.
type result struct {
	Error      bool                  `json:"error"`
	Code       string                `json:"code"`
	Message    string                `json:"message"`
	NextAction string                `json:"next_action"`
	Target     string                `json:"target"`
	Repo       string                `json:"repo"`
	RFCURL     string                `json:"rfc_url"`
	Children   []publish.ResultChild `json:"children"`
	Created    []string              `json:"created"`
	Updated    []string              `json:"updated"`
	Unchanged  []string              `json:"unchanged"`
	Warnings   []string              `json:"warnings"`
	DryRun     bool                  `json:"dry_run"`
}

// run executes `spektacular publish` in root with env added to a scrubbed
// environment, and decodes its JSON output.
func run(t *testing.T, root string, env []string, args ...string) (result, int) {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"publish", "--format", "json"}, args...)...)
	cmd.Dir = root
	cmd.Env = append(scrubbedEnv(), env...)
	out, err := cmd.Output()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
		require.Empty(t, string(exit.Stderr), "stderr is quiet in json mode")
	} else {
		require.NoError(t, err, string(out))
	}
	var res result
	require.NoError(t, json.Unmarshal(out, &res), string(out))
	return res, code
}

// scrubbedEnv is the process environment without any publish credential, so
// a test only files where it pointed the binary.
func scrubbedEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		switch {
		case k == "GITHUB_TOKEN", k == "GH_TOKEN", strings.HasPrefix(k, "JIRA_"), strings.HasPrefix(k, "LINEAR_"), strings.HasPrefix(k, "SPEKTACULAR_"):
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH="+toolPath)
}

// filed is one item as a fake holds it, in target-neutral terms.
type filed struct {
	Ref     string
	URL     string
	Title   string
	Body    string
	Labels  []string
	Parent  string   // Ref of the parent, "" for the RFC
	Blocked []string // Refs of the blockers
}

// assertHandshake checks what every target must have filed for the spec.
func assertHandshake(t *testing.T, res result, rfcRef string, items map[string]filed) {
	t.Helper()
	require.False(t, res.Error, res.Message)
	require.Equal(t, []string{"rfc", "sched-1", "render-2"}, res.Created)
	require.Len(t, res.Children, 2)
	require.Len(t, items, 3)

	rfc := items[rfcRef]
	require.True(t, strings.HasPrefix(rfc.Title, "RFC: "), "the RFC title is what hive's prior-art gate reads: %q", rfc.Title)
	require.Regexp(t, "commit [`{]*[0-9a-f]{7,}", rfc.Body, "the commit is recorded, so git was on the binary's PATH")
	m, ok := publish.ParseMarker(rfc.Body)
	require.True(t, ok, "the RFC carries the marker")
	require.True(t, m.IsRFC(), "the RFC marker has no task=")
	require.Equal(t, "testproj/spec/nightly-digest", m.ArtifactID)
	require.Equal(t, "", rfc.Parent)
	require.Empty(t, rfc.Blocked)
	// Jira's renderers fold the search-token line into the marker's
	// paragraph, so the marker ends the body rather than owning its line.
	require.True(t, strings.HasSuffix(strings.TrimSpace(rfc.Body), m.String()), "the marker ends the body: %q", rfc.Body)

	childRefs := map[string]string{}
	for ref, it := range items {
		if ref == rfcRef {
			continue
		}
		m, ok := publish.ParseMarker(it.Body)
		require.True(t, ok, "child %s carries the marker", ref)
		require.NotEmpty(t, m.Task, "child markers carry task=")
		require.Equal(t, res.RFCURL, m.RFC, "child markers carry the RFC URL")
		childRefs[m.Task] = ref
		require.False(t, strings.HasPrefix(it.Title, "RFC:"), "children are not RFCs: %q", it.Title)
		require.Equal(t, rfcRef, it.Parent, "child %s hangs off the RFC", ref)
		visible := htmlCommentRE.ReplaceAllString(it.Body, "")
		require.GreaterOrEqual(t, len(strings.TrimSpace(visible)), minBodyChars, "child body clears hive's clarity gate without its hidden lines: %q", visible)
		require.Empty(t, placeholderRE.FindAllString(visible, -1), "no unfilled placeholders in %s", ref)
		require.Regexp(t, `(?m)^([-*] )?\[ \] `+regexp.QuoteMeta(it.Ref)+` — `, rfc.Body, "the RFC checklist has a line for child %s", ref)
		require.Contains(t, it.Labels, configuredLabel, "the configured label is on child %s", ref)
		for _, l := range it.Labels {
			require.NotContains(t, publish.HiveSuppressLabels, l)
			require.NotEqual(t, "epic", l, "children never carry the parent label")
		}
	}
	require.Len(t, childRefs, 2)
	require.Equal(t, []string{childRefs["sched-1"]}, items[childRefs["render-2"]].Blocked, "render-2 is blocked by sched-1")
	require.Empty(t, items[childRefs["sched-1"]].Blocked)
	for _, c := range res.Children {
		require.Equal(t, items[childRefs[c.ID]].URL, c.URL, "result URLs match what was filed")
	}
	require.Equal(t, rfc.URL, res.RFCURL)
}

// counts is what a fake holds: issues and relations (parent + blocked
// by), so a re-run can be shown to add none on the fake's side as well as
// in the binary's output.
type counts struct{ Issues, Relations int }

// assertRecordedAndIdempotent checks the frontmatter, that a re-run files
// nothing more (per the binary and per the fake), that a changed spec is
// refused without --update and applied with it, and that a closed RFC is
// refused however the run is flagged.
func assertRecordedAndIdempotent(t *testing.T, root string, env []string, res result, target string, count func() counts, closeRFC func()) {
	t.Helper()
	specPath := filepath.Join(root, ".spektacular", "specs", "nightly-digest.md")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Len(t, fm.Published, 1)
	require.Equal(t, target, fm.Published[0].Target)
	require.Equal(t, res.RFCURL, fm.Published[0].RFCURL)
	require.Len(t, fm.Published[0].Children, 2)

	before := count()
	require.Equal(t, counts{Issues: 3, Relations: 3}, before, "RFC + 2 children; 2 parent relations + 1 blocked-by")
	again, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, again.Message)
	require.Empty(t, again.Created)
	require.Empty(t, again.Updated)
	require.ElementsMatch(t, []string{"rfc", "sched-1", "render-2"}, again.Unchanged)
	require.Equal(t, before, count(), "the fake holds exactly what it held: nothing was filed twice")

	// A changed body is a conflict until --update, which applies it in
	// place and files nothing new. Non-Goals is a section only the RFC
	// carries, so the children stay as they are.
	edited := strings.TrimRight(string(raw), "\n") + "\n\n## Non-Goals\n\n- Per-operator digest preferences.\n"
	require.NoError(t, os.WriteFile(specPath, []byte(edited), 0o644))
	conflict, code := run(t, root, env, "nightly-digest")
	require.NotEqual(t, 0, code)
	require.Equal(t, publish.ErrConflict, conflict.Code)
	require.Contains(t, conflict.Message, "rfc (body)")
	require.Contains(t, conflict.NextAction, "--update")
	require.Equal(t, before, count())

	updated, code := run(t, root, env, "nightly-digest", "--update")
	require.Equal(t, 0, code, updated.Message)
	require.Empty(t, updated.Created)
	require.Equal(t, []string{"rfc"}, updated.Updated)
	require.ElementsMatch(t, []string{"sched-1", "render-2"}, updated.Unchanged)
	require.Equal(t, before, count())
	settled, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, settled.Message)
	require.Empty(t, settled.Updated, "the update is read back as the new baseline")

	// Once the RFC is closed on the target, nothing is filed or changed.
	closeRFC()
	closed, code := run(t, root, env, "nightly-digest", "--update")
	require.NotEqual(t, 0, code)
	require.Equal(t, publish.ErrConflict, closed.Code)
	require.Contains(t, closed.Message, "closed")
	require.Equal(t, before, count())
}

func TestPublish_GitHubHandshake(t *testing.T) {
	gh := githubtest.New(t)
	gh.Labels = append(gh.Labels, configuredLabel)
	root := project(t, "publish:\n  github:\n    repo: acme/demo\n    labels: ["+configuredLabel+"]\n")
	env := []string{"GITHUB_TOKEN=" + githubtest.Token, "SPEKTACULAR_GITHUB_API=" + gh.URL}

	dry, code := run(t, root, env, "nightly-digest", "--dry-run")
	require.Equal(t, 0, code, dry.Message)
	require.True(t, dry.DryRun)
	require.Zero(t, gh.Calls(), "a dry run makes no network call")

	res, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, res.Message)
	require.Equal(t, "github", res.Target)
	require.Equal(t, "acme/demo", res.Repo)
	require.Empty(t, res.Warnings)

	st := gh.Snapshot()
	items := map[string]filed{}
	byID := map[int64]string{}
	for _, is := range st.Issues {
		byID[is.ID] = fmt.Sprintf("#%d", is.Number)
	}
	for n, is := range st.Issues {
		ref := fmt.Sprintf("#%d", n)
		f := filed{Ref: ref, Title: is.Title, Body: is.Body, Labels: is.Labels, URL: is.URL}
		for parent, subs := range st.Subs {
			for _, id := range subs {
				if id == is.ID {
					f.Parent = fmt.Sprintf("#%d", parent)
				}
			}
		}
		for _, id := range st.Blocked[n] {
			f.Blocked = append(f.Blocked, byID[id])
		}
		items[ref] = f
	}
	assertHandshake(t, res, "#1", items)
	require.ElementsMatch(t, []string{"epic", configuredLabel}, items["#1"].Labels, "the parent label is on the RFC so hive never claims it")
	count := func() counts {
		st := gh.Snapshot()
		c := counts{Issues: len(st.Issues)}
		for _, ids := range st.Subs {
			c.Relations += len(ids)
		}
		for _, ids := range st.Blocked {
			c.Relations += len(ids)
		}
		return c
	}
	assertRecordedAndIdempotent(t, root, env, res, "github", count, func() { gh.Update(1, func(is *githubtest.Issue) { is.State = "closed" }) })
}

func TestPublish_JiraHandshake(t *testing.T) {
	for _, dc := range []bool{false, true} {
		name := "cloud"
		if dc {
			name = "datacenter"
		}
		t.Run(name, func(t *testing.T) {
			j := jiratest.New(t, dc)
			// The search index lags, as a real site's does: nothing filed is
			// searchable until Reindex, so a re-run must get by on the issues
			// the frontmatter recorded.
			j.SearchLag = true
			cfg := "publish:\n  default_target: jira\n  jira:\n    project_key: DEMO\n    labels: [" + configuredLabel + "]\n"
			env := []string{"JIRA_BASE_URL=" + j.URL}
			if dc {
				cfg += "    deployment: datacenter\n"
				env = append(env, "JIRA_PAT="+jiratest.PAT)
			} else {
				env = append(env, "JIRA_EMAIL="+jiratest.Email, "JIRA_API_TOKEN="+jiratest.APIToken)
			}
			root := project(t, cfg)

			dry, code := run(t, root, env, "nightly-digest", "--dry-run")
			require.Equal(t, 0, code, dry.Message)
			require.Zero(t, j.Calls(), "a dry run makes no network call")

			res, code := run(t, root, env, "nightly-digest")
			require.Equal(t, 0, code, res.Message)
			require.Equal(t, "jira", res.Target)
			require.Equal(t, "DEMO", res.Repo)
			require.Empty(t, res.Warnings)

			issues := j.Snapshot()
			items := map[string]filed{}
			for key, is := range issues {
				f := filed{Ref: key, Title: is.Summary, Body: j.DescriptionText(key), Labels: is.Labels, Parent: is.Parent, URL: j.URL + "/browse/" + key}
				for _, l := range j.LinkList() {
					if blocker, blocked, ok := strings.Cut(l, " blocks "); ok && blocked == key {
						f.Blocked = append(f.Blocked, blocker)
					}
				}
				items[key] = f
			}
			assertHandshake(t, res, "DEMO-1", items)
			require.Equal(t, "Epic", issues["DEMO-1"].Type, "the RFC is an Epic, which hive's default JQL excludes")
			require.Equal(t, "Story", issues["DEMO-2"].Type)
			require.False(t, issues["DEMO-2"].Done)
			_, sentParent := issues["DEMO-2"].Fields["parent"]
			if dc {
				require.False(t, sentParent, "Data Center Stories are linked by the Epic Link field, which the site refuses parent for")
				require.Equal(t, "DEMO-1", issues["DEMO-2"].Fields[jiratest.EpicLinkField])
			} else {
				require.True(t, sentParent)
			}
			require.NotNil(t, issues["DEMO-2"].Property, "the marker is also in the issue property")
			count := func() counts {
				c := counts{Issues: len(j.Snapshot()), Relations: len(j.LinkList())}
				for _, is := range j.Snapshot() {
					if is.Parent != "" {
						c.Relations++
					}
				}
				return c
			}
			assertRecordedAndIdempotent(t, root, env, res, "jira", count, func() { j.Update("DEMO-1", func(is *jiratest.Issue) { is.Done = true }) })
		})
	}
}

func TestPublish_LinearHandshake(t *testing.T) {
	l := lineartest.New(t)
	root := project(t, "publish:\n  default_target: linear\n  linear:\n    team: ENG\n    labels: ["+configuredLabel+"]\n")
	env := []string{"LINEAR_API_KEY=" + lineartest.APIKey, "LINEAR_API_URL=" + l.URL}

	dry, code := run(t, root, env, "nightly-digest", "--dry-run")
	require.Equal(t, 0, code, dry.Message)
	require.Zero(t, l.Calls(), "a dry run makes no network call")

	res, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, res.Message)
	require.Equal(t, "linear", res.Target)
	require.Equal(t, "ENG", res.Repo)
	require.Empty(t, res.Warnings)

	issues := l.Snapshot()
	byID := map[string]string{}
	for _, is := range issues {
		byID[is.ID] = is.Identifier
	}
	items := map[string]filed{}
	for _, is := range issues {
		f := filed{Ref: is.Identifier, Title: is.Title, Body: is.Description, URL: "https://linear.app/acme/issue/" + is.Identifier}
		for _, id := range is.LabelIDs {
			f.Labels = append(f.Labels, strings.TrimPrefix(id, "label-"))
		}
		f.Parent = byID[is.ParentID]
		for _, r := range l.RelationList() {
			if blocker, blocked, ok := strings.Cut(r, " blocks "); ok && blocked == is.Identifier {
				f.Blocked = append(f.Blocked, blocker)
			}
		}
		require.Equal(t, lineartest.StateID(lineartest.Todo), is.StateID, "%s is in Todo, where a hive reads", is.Identifier)
		items[is.Identifier] = f
	}
	assertHandshake(t, res, "ENG-1", items)
	require.ElementsMatch(t, []string{"epic", configuredLabel}, items["ENG-1"].Labels, "the parent label is on the RFC")
	count := func() counts {
		c := counts{Relations: len(l.RelationList())}
		for _, is := range l.Snapshot() {
			c.Issues++
			if is.ParentID != "" {
				c.Relations++
			}
		}
		return c
	}
	assertRecordedAndIdempotent(t, root, env, res, "linear", count, func() {
		l.Update("ENG-1", func(rfc *lineartest.Issue) {
			rfc.StateType = "completed"
			rfc.Archived = true // Linear auto-archives completed issues; Find must still see it
		})
	})
}

func TestPublish_SuppressLabelsAreRefusedBeforeFiling(t *testing.T) {
	gh := githubtest.New(t)
	root := project(t, "publish:\n  github:\n    repo: acme/demo\n    labels: [spek, hold]\n")
	env := []string{"GITHUB_TOKEN=" + githubtest.Token, "SPEKTACULAR_GITHUB_API=" + gh.URL}
	res, code := run(t, root, env, "nightly-digest")
	require.NotEqual(t, 0, code)
	require.Equal(t, publish.ErrTargetUnconfigured, res.Code)
	require.Contains(t, res.Message, `"hold"`)
	require.Contains(t, res.NextAction, "publish.github.labels")
	require.Zero(t, gh.Calls(), "refused before any network call")

	l := lineartest.New(t)
	root = project(t, "publish:\n  default_target: linear\n  linear:\n    team: ENG\n    parent_label: needs-human\n")
	res, code = run(t, root, []string{"LINEAR_API_KEY=" + lineartest.APIKey, "LINEAR_API_URL=" + l.URL}, "nightly-digest")
	require.NotEqual(t, 0, code)
	require.Equal(t, publish.ErrTargetUnconfigured, res.Code)
	require.Contains(t, res.NextAction, "publish.linear.parent_label")
	require.Zero(t, l.Calls())
}

func TestPublish_RefusalsCarryNextAction(t *testing.T) {
	root := project(t, "publish:\n  default_target: jira\n")
	res, code := run(t, root, nil, "nightly-digest")
	require.NotEqual(t, 0, code)
	require.True(t, res.Error)
	require.Equal(t, publish.ErrTargetUnconfigured, res.Code)
	require.Contains(t, res.NextAction, "publish.jira.project_key")

	root = project(t, "publish:\n  github:\n    repo: acme/demo\n")
	res, code = run(t, root, nil, "nightly-digest")
	require.NotEqual(t, 0, code)
	require.Equal(t, publish.ErrAuthMissing, res.Code)
	require.NotEmpty(t, res.NextAction)
}

func TestPublish_LinearProjectHandshake(t *testing.T) {
	l := lineartest.New(t)
	root := project(t, "publish:\n  default_target: linear\n  linear:\n    team: ENG\n    parent_as: project\n    assignee: "+lineartest.UserEmail+"\n    labels: ["+configuredLabel+"]\n")
	env := []string{"LINEAR_API_KEY=" + lineartest.APIKey, "LINEAR_API_URL=" + l.URL}
	dry, code := run(t, root, env, "nightly-digest", "--dry-run")
	require.Equal(t, 0, code, dry.Message)
	require.Zero(t, l.Calls())
	res, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, res.Message)
	require.Empty(t, res.Warnings)
	projects := l.ProjectSnapshot()
	require.Len(t, projects, 1)
	rfc := projects[0]
	items := map[string]filed{rfc.ID: {Ref: rfc.ID, Title: rfc.Name, Body: rfc.Content, URL: res.RFCURL}}
	for _, is := range l.Snapshot() {
		it := filed{Ref: is.Identifier, Title: is.Title, Body: is.Description,
			URL: "https://linear.app/acme/issue/" + is.Identifier, Parent: is.ProjectID}
		for _, id := range is.LabelIDs {
			it.Labels = append(it.Labels, strings.TrimPrefix(id, "label-"))
		}
		for _, r := range l.RelationList() {
			if blocker, blocked, ok := strings.Cut(r, " blocks "); ok && blocked == is.Identifier {
				it.Blocked = append(it.Blocked, blocker)
			}
		}
		require.Empty(t, is.ParentID, "project children are regular issues")
		require.Equal(t, lineartest.StateID(lineartest.Todo), is.StateID)
		require.Equal(t, lineartest.UserID, is.AssigneeID, "assigned_only Hive enumeration can see the children")
		items[is.Identifier] = it
	}
	assertHandshake(t, res, rfc.ID, items)
	count := func() counts {
		c := counts{Issues: len(l.ProjectSnapshot()), Relations: len(l.RelationList())}
		for _, is := range l.Snapshot() {
			c.Issues++
			if is.ProjectID != "" {
				c.Relations++
			}
		}
		return c
	}
	assertRecordedAndIdempotent(t, root, env, res, "linear", count, func() {
		l.UpdateProject(rfc.ID, func(p *lineartest.Project) { p.StatusType, p.Archived = "completed", true })
	})
}
