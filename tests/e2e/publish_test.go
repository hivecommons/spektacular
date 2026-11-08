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

// Labels hive hard-suppresses (src/pkg/github/labels.go); no filed item may
// carry one, or the hive never sees it.
var hiveSuppressLabels = []string{"hold", "needs-human", "needs-decision", "needs-spec", "needs-direction"}

// minBodyChars is hive's clarity-gate floor (runs.triage.clarity_gate).
const minBodyChars = 80

// placeholderRE is what hive's clarity gate reads as an unfilled template;
// the marker's own angle brackets are stripped before the check.
var placeholderRE = regexp.MustCompile(`<[a-z][a-z -]*>`)

var binary string

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
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
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
	// No gh on PATH: the github target must not fall back to `gh auth token`.
	return append(env, "PATH=")
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
	m, ok := publish.ParseMarker(rfc.Body)
	require.True(t, ok, "the RFC carries the marker")
	require.True(t, m.IsRFC(), "the RFC marker has no task=")
	require.Equal(t, "testproj/spec/nightly-digest", m.ArtifactID)
	require.Equal(t, "", rfc.Parent)
	require.Empty(t, rfc.Blocked)
	lastLine := strings.TrimSpace(rfc.Body)
	lastLine = lastLine[strings.LastIndex(lastLine, "\n")+1:]
	require.Equal(t, m.String(), lastLine, "the marker is the last line of the body")

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
		require.GreaterOrEqual(t, len(it.Body), minBodyChars, "child body clears hive's clarity gate")
		require.Empty(t, placeholderRE.FindAllString(strings.Replace(it.Body, m.String(), "", 1), -1), "no unfilled placeholders in %s", ref)
		require.Contains(t, rfc.Body, it.Ref, "the RFC checklist names child %s", ref)
		for _, l := range it.Labels {
			require.NotContains(t, hiveSuppressLabels, l)
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

// assertRecordedAndIdempotent checks the frontmatter and that a re-run
// files nothing more.
func assertRecordedAndIdempotent(t *testing.T, root string, env []string, res result, target string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "specs", "nightly-digest.md"))
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Len(t, fm.Published, 1)
	require.Equal(t, target, fm.Published[0].Target)
	require.Equal(t, res.RFCURL, fm.Published[0].RFCURL)
	require.Len(t, fm.Published[0].Children, 2)

	again, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, again.Message)
	require.Empty(t, again.Created)
	require.Empty(t, again.Updated)
	require.ElementsMatch(t, []string{"rfc", "sched-1", "render-2"}, again.Unchanged)
}

func TestPublish_GitHubHandshake(t *testing.T) {
	gh := githubtest.New(t)
	root := project(t, "publish:\n  github:\n    repo: acme/demo\n")
	env := []string{"GITHUB_TOKEN=" + githubtest.Token, "SPEKTACULAR_GITHUB_API=" + gh.URL}

	dry, code := run(t, root, env, "nightly-digest", "--dry-run")
	require.Equal(t, 0, code, dry.Message)
	require.True(t, dry.DryRun)
	require.Empty(t, gh.Issues, "a dry run files nothing")
	require.Zero(t, gh.Count("GET /repos/acme/demo/labels"), "a dry run makes no network call")

	res, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, res.Message)
	require.Equal(t, "github", res.Target)
	require.Equal(t, "acme/demo", res.Repo)
	require.Empty(t, res.Warnings)

	items := map[string]filed{}
	byID := map[int64]string{}
	for _, is := range gh.Issues {
		byID[is.ID] = fmt.Sprintf("#%d", is.Number)
	}
	for n, is := range gh.Issues {
		ref := fmt.Sprintf("#%d", n)
		f := filed{Ref: ref, Title: is.Title, Body: is.Body, Labels: is.Labels, URL: is.URL}
		for parent, subs := range gh.Subs {
			for _, id := range subs {
				if id == is.ID {
					f.Parent = fmt.Sprintf("#%d", parent)
				}
			}
		}
		for _, id := range gh.Blocked[n] {
			f.Blocked = append(f.Blocked, byID[id])
		}
		items[ref] = f
	}
	assertHandshake(t, res, "#1", items)
	require.Equal(t, []string{"epic"}, items["#1"].Labels, "the parent label is on the RFC so hive never claims it")
	assertRecordedAndIdempotent(t, root, env, res, "github")
}

func TestPublish_JiraHandshake(t *testing.T) {
	for _, dc := range []bool{false, true} {
		name := "cloud"
		if dc {
			name = "datacenter"
		}
		t.Run(name, func(t *testing.T) {
			j := jiratest.New(t, dc)
			cfg := "publish:\n  default_target: jira\n  jira:\n    project_key: DEMO\n"
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
			require.Empty(t, j.Issues, "a dry run files nothing")

			res, code := run(t, root, env, "nightly-digest")
			require.Equal(t, 0, code, res.Message)
			require.Equal(t, "jira", res.Target)
			require.Equal(t, "DEMO", res.Repo)
			require.Empty(t, res.Warnings)

			items := map[string]filed{}
			for key, is := range j.Issues {
				f := filed{Ref: key, Title: is.Summary, Body: j.DescriptionText(key), Labels: is.Labels, Parent: is.Parent, URL: j.URL + "/browse/" + key}
				for _, l := range j.Links {
					if blocker, blocked, ok := strings.Cut(l, " blocks "); ok && blocked == key {
						f.Blocked = append(f.Blocked, blocker)
					}
				}
				items[key] = f
			}
			assertHandshake(t, res, "DEMO-1", items)
			require.Equal(t, "Epic", j.Issues["DEMO-1"].Type, "the RFC is an Epic, which hive's default JQL excludes")
			require.Equal(t, "Story", j.Issues["DEMO-2"].Type)
			require.False(t, j.Issues["DEMO-2"].Done)
			assertRecordedAndIdempotent(t, root, env, res, "jira")
		})
	}
}

func TestPublish_LinearHandshake(t *testing.T) {
	l := lineartest.New(t)
	root := project(t, "publish:\n  default_target: linear\n  linear:\n    team: ENG\n")
	env := []string{"LINEAR_API_KEY=" + lineartest.APIKey, "LINEAR_API_URL=" + l.URL}

	dry, code := run(t, root, env, "nightly-digest", "--dry-run")
	require.Equal(t, 0, code, dry.Message)
	require.Empty(t, l.Issues, "a dry run files nothing")
	require.Zero(t, l.Count("teams"), "a dry run makes no network call")

	res, code := run(t, root, env, "nightly-digest")
	require.Equal(t, 0, code, res.Message)
	require.Equal(t, "linear", res.Target)
	require.Equal(t, "ENG", res.Repo)
	require.Empty(t, res.Warnings)

	items := map[string]filed{}
	for _, is := range l.Issues {
		f := filed{Ref: is.Identifier, Title: is.Title, Body: is.Description, URL: "https://linear.app/acme/issue/" + is.Identifier}
		for _, id := range is.LabelIDs {
			f.Labels = append(f.Labels, strings.TrimPrefix(id, "label-"))
		}
		if is.ParentID != "" {
			f.Parent = l.Issues[is.ParentID].Identifier
		}
		for _, r := range l.Relations {
			if blocker, blocked, ok := strings.Cut(r, " blocks "); ok && blocked == is.Identifier {
				f.Blocked = append(f.Blocked, blocker)
			}
		}
		items[is.Identifier] = f
	}
	assertHandshake(t, res, "ENG-1", items)
	require.Equal(t, []string{"epic"}, items["ENG-1"].Labels, "the parent label is on the RFC")
	assertRecordedAndIdempotent(t, root, env, res, "linear")
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
