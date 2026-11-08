package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/jira/jiratest"
	"github.com/hivecommons/spektacular/internal/publish/linear/lineartest"
)

const publishSpecBody = `# Feature: Nightly digest email

<!-- Guidance for the author: replace <every placeholder>. -->

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

const publishPlanBody = `# Plan: nightly-digest

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

// publishProject is a project with one final spec, nightly-digest, and no
// plan. Network credentials point at nothing; each test that files sets them.
func publishProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	writeSpecCommandConfig(t, root, "")
	writeSpecFixture(t, root, "nightly-digest", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n"+publishSpecBody)
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GH_TOKEN", "")
	return root
}

func writePublishPlan(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, ".spektacular", "plans", name, "plan.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\ncreated_date: 2026-02-02\ndocument_status: final\nspec: "+name+"\n---\n\n"+publishPlanBody), 0o644))
}

func runPublishCmd(t *testing.T, args ...string) (string, int) {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, append([]string{"publish"}, args...)...)
	require.Empty(t, stderr)
	return stdout, code
}

func requirePublishRefusal(t *testing.T, stdout string, code int, want string) output.ErrorResponse {
	t.Helper()
	require.Equal(t, 1, code, stdout)
	var resp output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &resp), stdout)
	require.Equal(t, want, resp.Code, stdout)
	require.NotEmpty(t, resp.NextAction)
	return resp
}

func TestPublish_DryRunRendersWithoutNetwork(t *testing.T) {
	publishProject(t)
	// Any network call would fail loudly against a closed server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("--dry-run made a network call: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("SPEKTACULAR_GITHUB_API", srv.URL)

	stdout, code := runPublishCmd(t, "nightly-digest", "--dry-run", "--repo", "acme/demo")
	require.Equal(t, 0, code, stdout)
	require.True(t, strings.HasPrefix(stdout, "# RFC: Nightly digest email\n"), stdout)
	require.Contains(t, stdout, "<!-- spek:publish name=nightly-digest artifact_id=testproj/spec/nightly-digest target=github -->")
	require.Contains(t, stdout, "<!-- spek:publish name=nightly-digest artifact_id=testproj/spec/nightly-digest target=github task=R1 -->")
	require.NotContains(t, stdout, "Guidance for the author")

	stdout, code = runPublishCmd(t, "nightly-digest", "--dry-run", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Name        string              `json:"name"`
		Kind        string              `json:"kind"`
		ArtifactID  string              `json:"artifact_id"`
		Target      string              `json:"target"`
		Repo        string              `json:"repo"`
		DryRun      bool                `json:"dry_run"`
		Publication publish.Publication `json:"publication"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, "nightly-digest", res.Name)
	require.Equal(t, "spec", res.Kind)
	require.Equal(t, "testproj/spec/nightly-digest", res.ArtifactID)
	require.Equal(t, "github", res.Target)
	require.Equal(t, "acme/demo", res.Repo)
	require.True(t, res.DryRun)
	require.Equal(t, "RFC: Nightly digest email", res.Publication.RFC.Title)
	require.Len(t, res.Publication.Children, 2)
	require.Equal(t, "R1", res.Publication.Children[0].ID)
	require.Equal(t, []string{"Digest schedule: a digest arrives within five minutes of the hour."}, res.Publication.Children[0].AcceptanceCriteria)
}

func TestPublish_PlanTasksAreTheChildrenWhenAPlanExists(t *testing.T) {
	root := publishProject(t)
	writePublishPlan(t, root, "nightly-digest")

	stdout, code := runPublishCmd(t, "nightly-digest", "--dry-run", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Publication publish.Publication `json:"publication"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	require.Equal(t, publish.ChildrenFromPlan, res.Publication.ChildrenFrom)
	require.Equal(t, []string{"sched-1", "render-2"}, []string{res.Publication.Children[0].ID, res.Publication.Children[1].ID})
	require.Equal(t, []string{"sched-1"}, res.Publication.Children[1].DependsOn)
}

func TestPublish_Refusals(t *testing.T) {
	t.Run("not final", func(t *testing.T) {
		root := publishProject(t)
		writeSpecFixture(t, root, "wip", "---\ncreated_date: 2026-02-01\ndocument_status: draft\n---\n\n"+publishSpecBody)
		stdout, code := runPublishCmd(t, "wip", "--dry-run", "--repo", "acme/demo")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrNotFinal)
		require.Contains(t, resp.NextAction, "--allow-draft")

		stdout, code = runPublishCmd(t, "wip", "--dry-run", "--repo", "acme/demo", "--allow-draft")
		require.Equal(t, 0, code, stdout)
		require.True(t, strings.HasPrefix(stdout, "# RFC: [DRAFT] Nightly digest email\n"), stdout)
	})
	t.Run("target unconfigured", func(t *testing.T) {
		publishProject(t)
		stdout, code := runPublishCmd(t, "nightly-digest", "--dry-run", "--target", "trello")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "github, jira, linear")
	})
	t.Run("repo unknown", func(t *testing.T) {
		publishProject(t)
		t.Setenv("PATH", "")
		stdout, code := runPublishCmd(t, "nightly-digest")
		requirePublishRefusal(t, stdout, code, publish.ErrRepoUnknown)
	})
	t.Run("auth missing", func(t *testing.T) {
		publishProject(t)
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("PATH", "")
		stdout, code := runPublishCmd(t, "nightly-digest", "--repo", "acme/demo")
		requirePublishRefusal(t, stdout, code, publish.ErrAuthMissing)
	})
	t.Run("body hygiene", func(t *testing.T) {
		root := publishProject(t)
		writeSpecFixture(t, root, "holey", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n"+
			strings.Replace(publishSpecBody, "Operators want", "<team> want", 1))
		stdout, code := runPublishCmd(t, "holey", "--dry-run", "--repo", "acme/demo")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrBodyHygiene)
		require.Contains(t, resp.Message, "<team>")
	})
	t.Run("no children", func(t *testing.T) {
		root := publishProject(t)
		writeSpecFixture(t, root, "bare", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n# Feature: Bare\n\n## Overview\n\nJust words.\n")
		stdout, code := runPublishCmd(t, "bare", "--dry-run", "--repo", "acme/demo")
		requirePublishRefusal(t, stdout, code, publish.ErrNoChildren)
	})
	t.Run("not found", func(t *testing.T) {
		publishProject(t)
		stdout, code := runPublishCmd(t, "nope", "--dry-run", "--repo", "acme/demo")
		requirePublishRefusal(t, stdout, code, "artifact_not_found")
	})
	t.Run("name required", func(t *testing.T) {
		publishProject(t)
		stdout, code := runPublishCmd(t)
		requirePublishRefusal(t, stdout, code, "publish_name_required")
	})
}

func TestPublish_FilesThenRecordsThenRefusesThenUpdates(t *testing.T) {
	root := publishProject(t)
	gh := newPublishFake(t)
	t.Setenv("SPEKTACULAR_GITHUB_API", gh.url)

	stdout, code := runPublishCmd(t, "nightly-digest", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		RFCURL   string                `json:"rfc_url"`
		Repo     string                `json:"repo"`
		Children []publish.ResultChild `json:"children"`
		Created  []string              `json:"created"`
		Warnings []string              `json:"warnings"`
		Pub      *publish.Publication  `json:"publication"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, "https://github.com/acme/demo/issues/1", res.RFCURL)
	require.Equal(t, "acme/demo", res.Repo)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Len(t, res.Children, 2)
	require.Nil(t, res.Pub, "the full model is dry-run output only")
	require.Equal(t, []string{"epic"}, gh.issues[1].Labels, "the parent label exists so it is applied to the RFC")

	// Written back to the spec's frontmatter.
	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "specs", "nightly-digest.md"))
	require.NoError(t, err)
	fm, body, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, metadata.StatusFinal, fm.DocumentStatus, "other frontmatter survives")
	require.Equal(t, []metadata.Publication{{Target: "github", RFCURL: "https://github.com/acme/demo/issues/1", Children: []metadata.PublishedChild{
		{ID: "R1", URL: "https://github.com/acme/demo/issues/2"}, {ID: "R2", URL: "https://github.com/acme/demo/issues/3"},
	}}}, fm.Published)
	require.Contains(t, string(body), "## Requirements", "the body is untouched")

	// A re-run with nothing changed succeeds and creates nothing.
	before := gh.created()
	stdout, code = runPublishCmd(t, "nightly-digest", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var rerun struct {
		Created   []string `json:"created"`
		Unchanged []string `json:"unchanged"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rerun))
	require.Empty(t, rerun.Created)
	require.Equal(t, []string{"R1", "R2", "rfc"}, rerun.Unchanged)
	require.Equal(t, before, gh.created())

	// A change refuses without --update, naming what would change.
	writeSpecFixture(t, root, "nightly-digest", strings.Replace(string(raw),
		"with links.\n", "with links.\n- [ ] **Opt-out**: an operator can turn the digest off.\n", 1))
	stdout, code = runPublishCmd(t, "nightly-digest", "--repo", "acme/demo")
	resp := requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "R3 (create)")
	require.Contains(t, resp.NextAction, "--update")
	require.Equal(t, before, gh.created())

	stdout, code = runPublishCmd(t, "nightly-digest", "--repo", "acme/demo", "--update")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "created: R3")
	require.Equal(t, before+1, gh.created())
	require.Contains(t, gh.issues[1].Body, "- [ ] #4 — Opt-out")

	// A closed RFC is a conflict however the run is flagged.
	gh.issues[1].State = "closed"
	stdout, code = runPublishCmd(t, "nightly-digest", "--repo", "acme/demo", "--update")
	resp = requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "closed")
}

func TestPublish_EpicPublishesSpecsAsChildren(t *testing.T) {
	root := publishProject(t)
	writeSpecFixture(t, root, "chat-alerts", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n# Feature: Chat alerts\n\n## Overview\n\nPost a message to the chat channel when an item is parked, so an operator can unpark it.\n\n## Requirements\n\n- [ ] **Park alert**: post within a minute of an item being parked.\n\n## Acceptance Criteria\n\n- [ ] the message names the item and why it was parked.\n")
	epicPath := filepath.Join(root, ".spektacular", "epics", "notify.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(epicPath), 0o755))
	require.NoError(t, os.WriteFile(epicPath, []byte("---\ncreated_date: 2026-02-01\ndocument_status: final\nspecs:\n  - name: nightly-digest\n    depends_on: []\n  - name: chat-alerts\n    depends_on:\n      - nightly-digest\n---\n\n# Epic: Operator notifications\n\n## Overview\n\nOperators hear about the hive's work through the channels they already read.\n\n## Specs\n\n- nightly-digest\n- chat-alerts\n"), 0o644))
	gh := newPublishFake(t)
	t.Setenv("SPEKTACULAR_GITHUB_API", gh.url)

	stdout, code := runPublishCmd(t, "notify", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Kind    string   `json:"kind"`
		Created []string `json:"created"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	require.Equal(t, "epic", res.Kind)
	require.Equal(t, []string{"rfc", "nightly-digest", "chat-alerts"}, res.Created)
	require.Equal(t, []int64{1002}, gh.blocked[3], "chat-alerts is blocked by nightly-digest")
	m, ok := publish.ParseMarker(gh.issues[3].Body)
	require.True(t, ok)
	require.Equal(t, "chat-alerts", m.Task)
	require.Equal(t, "https://github.com/acme/demo/issues/1", m.RFC)

	raw, err := os.ReadFile(epicPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "published:\n    - target: github\n      rfc_url: https://github.com/acme/demo/issues/1\n")
	require.Contains(t, string(raw), "    - name: chat-alerts\n      depends_on:\n        - nightly-digest\n", "the specs list survives")

	// Naming a member spec publishes that spec alone.
	stdout, code = runPublishCmd(t, "chat-alerts", "--dry-run", "--repo", "acme/demo", "--format", "json")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, `"artifact_id": "testproj/spec/chat-alerts"`)
}

// publishJiraProject is publishProject configured for the jira target with
// Cloud credentials in the environment and the site pointed at the fake.
func publishJiraProject(t *testing.T, dataCenter bool) (string, *jiratest.Server) {
	t.Helper()
	root := publishProject(t)
	deployment := ""
	if dataCenter {
		deployment = "    deployment: datacenter\n"
	}
	writeSpecCommandConfig(t, root, "publish:\n  default_target: jira\n  jira:\n    project_key: DEMO\n    labels: [spek publish]\n"+deployment)
	writeSpecFixture(t, root, "nightly-digest", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n"+publishSpecBody)
	srv := jiratest.New(t, dataCenter)
	t.Setenv("JIRA_BASE_URL", srv.URL)
	if dataCenter {
		t.Setenv("JIRA_PAT", jiratest.PAT)
		t.Setenv("JIRA_EMAIL", "")
		t.Setenv("JIRA_API_TOKEN", "")
	} else {
		t.Setenv("JIRA_PAT", "")
		t.Setenv("JIRA_EMAIL", jiratest.Email)
		t.Setenv("JIRA_API_TOKEN", jiratest.APIToken)
	}
	t.Setenv("JIRA_CA_BUNDLE", "")
	return root, srv
}

func TestPublish_JiraDryRunMakesNoNetworkCall(t *testing.T) {
	_, srv := publishJiraProject(t, false)
	stdout, code := runPublishCmd(t, "nightly-digest", "--dry-run", "--format", "json")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, `"target": "jira"`)
	require.Contains(t, stdout, `"repo": "DEMO"`)
	require.Contains(t, stdout, "target=jira task=R1 --\\u003e")
	require.Empty(t, srv.Issues)
	require.Zero(t, srv.Calls(), "a dry run touches the site in no way")
}

func TestPublish_JiraRefusalsNameTheMissingSetting(t *testing.T) {
	t.Run("project key", func(t *testing.T) {
		root, _ := publishJiraProject(t, false)
		writeSpecCommandConfig(t, root, "publish:\n  default_target: jira\n")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "publish.jira.project_key")
		require.Contains(t, resp.NextAction, "publish.jira.project_key")
	})
	t.Run("base url", func(t *testing.T) {
		publishJiraProject(t, false)
		t.Setenv("JIRA_BASE_URL", "")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "JIRA_BASE_URL")
	})
	t.Run("cloud credential", func(t *testing.T) {
		publishJiraProject(t, false)
		t.Setenv("JIRA_API_TOKEN", "")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrAuthMissing)
		require.Contains(t, resp.Message, "JIRA_EMAIL and JIRA_API_TOKEN")
		require.Contains(t, resp.NextAction, "DEMO")
	})
	t.Run("data center credential", func(t *testing.T) {
		publishJiraProject(t, true)
		t.Setenv("JIRA_PAT", "")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrAuthMissing)
		require.Contains(t, resp.Message, "JIRA_PAT")
	})
	t.Run("ca bundle", func(t *testing.T) {
		publishJiraProject(t, true)
		t.Setenv("JIRA_CA_BUNDLE", filepath.Join(t.TempDir(), "missing.pem"))
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "JIRA_CA_BUNDLE")
	})
	t.Run("plain http", func(t *testing.T) {
		root, _ := publishJiraProject(t, false)
		t.Setenv("JIRA_BASE_URL", "http://jira.example.com")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "plain http")
		require.Contains(t, resp.NextAction, "publish.jira.insecure_http")

		// Opting in is explicit; the request then fails only on reaching the host.
		writeSpecCommandConfig(t, root, "publish:\n  default_target: jira\n  jira:\n    project_key: DEMO\n    insecure_http: true\n")
		stdout, code = runPublishCmd(t, "nightly-digest", "--dry-run")
		require.Equal(t, 0, code, stdout)
	})
}

func TestPublish_JiraPartialFailureIsRecordedAndResumed(t *testing.T) {
	root, srv := publishJiraProject(t, false)
	srv.SearchLag = true // nothing filed is searchable until Reindex
	srv.FailCreateAfter = 3
	stdout, code := runPublishCmd(t, "nightly-digest", "--format", "json")
	resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetFailed)
	require.Contains(t, resp.Message, "failed part way")
	require.Len(t, srv.Issues, 2, "the Epic and R1 were filed before the failure")

	specPath := filepath.Join(root, ".spektacular", "specs", "nightly-digest.md")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, []metadata.Publication{{Target: "jira", RFCURL: srv.URL + "/browse/DEMO-1", Children: []metadata.PublishedChild{
		{ID: "R1", URL: srv.URL + "/browse/DEMO-2"},
	}}}, fm.Published, "what was filed is recorded even though the run failed")

	// The re-run reads the recorded issues directly, so it reuses them
	// although the search index has not caught up, and files only R2.
	srv.FailCreateAfter = 0
	stdout, code = runPublishCmd(t, "nightly-digest", "--update", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Created   []string `json:"created"`
		Unchanged []string `json:"unchanged"`
		Linked    []string `json:"linked"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, []string{"R2"}, res.Created)
	require.ElementsMatch(t, []string{"R1"}, res.Unchanged)
	require.Len(t, srv.Issues, 3, "no duplicate of the Epic or R1")
	require.Equal(t, "DEMO-1", srv.Issues["DEMO-3"].Parent)

	raw, err = os.ReadFile(specPath)
	require.NoError(t, err)
	fm, _, err = metadata.Split(raw)
	require.NoError(t, err)
	require.Len(t, fm.Published, 1)
	require.Len(t, fm.Published[0].Children, 2)
}

func TestPublish_JiraFilesEpicAndStoriesThenUpdates(t *testing.T) {
	root, srv := publishJiraProject(t, false)

	stdout, code := runPublishCmd(t, "nightly-digest", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Target   string                `json:"target"`
		Repo     string                `json:"repo"`
		RFCURL   string                `json:"rfc_url"`
		RFCKey   string                `json:"rfc_key"`
		Children []publish.ResultChild `json:"children"`
		Created  []string              `json:"created"`
		Warnings []string              `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, "jira", res.Target)
	require.Equal(t, "DEMO", res.Repo)
	require.Equal(t, "DEMO-1", res.RFCKey)
	require.Equal(t, srv.URL+"/browse/DEMO-1", res.RFCURL)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, []publish.ResultChild{
		{ID: "R1", URL: srv.URL + "/browse/DEMO-2", Key: "DEMO-2"},
		{ID: "R2", URL: srv.URL + "/browse/DEMO-3", Key: "DEMO-3"},
	}, res.Children)
	require.Empty(t, res.Warnings)
	require.Equal(t, "Epic", srv.Issues["DEMO-1"].Type)
	require.Equal(t, "Story", srv.Issues["DEMO-2"].Type)
	require.Equal(t, "DEMO-1", srv.Issues["DEMO-2"].Parent)
	require.Equal(t, []string{"spek-publish"}, srv.Issues["DEMO-2"].Labels)

	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "specs", "nightly-digest.md"))
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, []metadata.Publication{{Target: "jira", RFCURL: srv.URL + "/browse/DEMO-1", Children: []metadata.PublishedChild{
		{ID: "R1", URL: srv.URL + "/browse/DEMO-2"}, {ID: "R2", URL: srv.URL + "/browse/DEMO-3"},
	}}}, fm.Published)

	// Unchanged re-run is a no-op; a change refuses, then --update applies.
	stdout, code = runPublishCmd(t, "nightly-digest")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "unchanged: R1, R2, rfc")
	require.Equal(t, 3, srv.Count("POST /rest/api/3/issue"))

	writeSpecFixture(t, root, "nightly-digest", strings.Replace(string(raw),
		"with links.\n", "with links.\n- [ ] **Opt-out**: an operator can turn the digest off.\n", 1))
	stdout, code = runPublishCmd(t, "nightly-digest")
	resp := requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "R3 (create)")
	require.Equal(t, 3, srv.Count("POST /rest/api/3/issue"))

	stdout, code = runPublishCmd(t, "nightly-digest", "--update")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "created: R3")
	require.Equal(t, 4, srv.Count("POST /rest/api/3/issue"))
	require.Contains(t, srv.DescriptionText("DEMO-1"), "DEMO-4 — Opt-out")

	// A Done Epic is a conflict however the run is flagged.
	srv.Issues["DEMO-1"].Done = true
	stdout, code = runPublishCmd(t, "nightly-digest", "--update")
	resp = requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "closed")
}

func TestPublish_JiraDataCenterUsesV2(t *testing.T) {
	_, srv := publishJiraProject(t, true)
	stdout, code := runPublishCmd(t, "nightly-digest", "--target", "jira")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "Published spec nightly-digest to DEMO")
	require.Equal(t, 3, srv.Count("POST /rest/api/2/issue"))
	require.IsType(t, "", srv.Issues["DEMO-1"].Description, "Data Center descriptions are wiki text")
}

// publishLinearProject is publishProject configured for the linear target
// with the API key in the environment and the endpoint pointed at the fake.
func publishLinearProject(t *testing.T) (string, *lineartest.Server) {
	t.Helper()
	root := publishProject(t)
	writeSpecCommandConfig(t, root, "publish:\n  default_target: linear\n  linear:\n    team: ENG\n    state: Todo\n    labels: [spek]\n")
	writeSpecFixture(t, root, "nightly-digest", "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n\n"+publishSpecBody)
	srv := lineartest.New(t)
	t.Setenv("LINEAR_API_URL", srv.URL)
	t.Setenv("LINEAR_API_KEY", lineartest.APIKey)
	return root, srv
}

func TestPublish_LinearDryRunMakesNoNetworkCall(t *testing.T) {
	_, srv := publishLinearProject(t)
	stdout, code := runPublishCmd(t, "nightly-digest", "--dry-run", "--format", "json")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, `"target": "linear"`)
	require.Contains(t, stdout, `"repo": "ENG"`)
	require.Empty(t, srv.Issues)
	require.Zero(t, srv.Calls(), "a dry run touches the API in no way")
}

func TestPublish_LinearAssigneeAndArchivedRFC(t *testing.T) {
	root, srv := publishLinearProject(t)
	writeSpecCommandConfig(t, root, "publish:\n  default_target: linear\n  linear:\n    team: eng\n    assignee: "+lineartest.UserEmail+"\n")
	stdout, code := runPublishCmd(t, "nightly-digest", "--format", "json")
	require.Equal(t, 0, code, stdout)
	for _, is := range srv.Snapshot() {
		require.Equal(t, lineartest.UserID, is.AssigneeID, is.Identifier)
		require.Equal(t, lineartest.StateID(lineartest.Todo), is.StateID, "Todo by default, where a hive reads")
	}

	// A completed, auto-archived RFC is still found and refused as closed
	// rather than filed again.
	rfc := srv.ByIdentifier("ENG-1")
	rfc.StateType = "completed"
	rfc.Archived = true
	stdout, code = runPublishCmd(t, "nightly-digest", "--update")
	resp := requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "closed")
	require.Equal(t, 3, srv.Count("issueCreate"))
}

func TestPublish_LinearRefusalsNameTheMissingSetting(t *testing.T) {
	t.Run("team", func(t *testing.T) {
		root, _ := publishLinearProject(t)
		writeSpecCommandConfig(t, root, "publish:\n  default_target: linear\n")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetUnconfigured)
		require.Contains(t, resp.Message, "publish.linear.team")
	})
	t.Run("api key", func(t *testing.T) {
		publishLinearProject(t)
		t.Setenv("LINEAR_API_KEY", "")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrAuthMissing)
		require.Contains(t, resp.Message, "LINEAR_API_KEY")
		require.Contains(t, resp.NextAction, "ENG")
	})
	t.Run("unknown team is a publish failure before anything is filed", func(t *testing.T) {
		root, srv := publishLinearProject(t)
		writeSpecCommandConfig(t, root, "publish:\n  default_target: linear\n  linear:\n    team: NOPE\n")
		stdout, code := runPublishCmd(t, "nightly-digest")
		resp := requirePublishRefusal(t, stdout, code, publish.ErrTargetFailed)
		require.Contains(t, resp.Message, `team "NOPE" does not exist`)
		require.Empty(t, srv.Issues)
	})
}

func TestPublish_LinearFilesIssueAndSubIssuesThenUpdates(t *testing.T) {
	root, srv := publishLinearProject(t)

	stdout, code := runPublishCmd(t, "nightly-digest", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		Target   string                `json:"target"`
		Repo     string                `json:"repo"`
		RFCURL   string                `json:"rfc_url"`
		RFCKey   string                `json:"rfc_key"`
		Children []publish.ResultChild `json:"children"`
		Created  []string              `json:"created"`
		Warnings []string              `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, "linear", res.Target)
	require.Equal(t, "ENG", res.Repo)
	require.Equal(t, "ENG-1", res.RFCKey)
	require.Equal(t, "https://linear.app/acme/issue/ENG-1", res.RFCURL)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, []publish.ResultChild{
		{ID: "R1", URL: "https://linear.app/acme/issue/ENG-2", Key: "ENG-2"},
		{ID: "R2", URL: "https://linear.app/acme/issue/ENG-3", Key: "ENG-3"},
	}, res.Children)
	require.Empty(t, res.Warnings)
	rfc := srv.ByIdentifier("ENG-1")
	require.Equal(t, []string{lineartest.LabelID("epic"), lineartest.LabelID("spek")}, rfc.LabelIDs, "default parent label on the RFC")
	require.Equal(t, lineartest.StateID("Todo"), rfc.StateID)
	child := srv.ByIdentifier("ENG-2")
	require.Equal(t, rfc.ID, child.ParentID)
	require.Equal(t, []string{lineartest.LabelID("spek")}, child.LabelIDs)

	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "specs", "nightly-digest.md"))
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, []metadata.Publication{{Target: "linear", RFCURL: "https://linear.app/acme/issue/ENG-1", Children: []metadata.PublishedChild{
		{ID: "R1", URL: "https://linear.app/acme/issue/ENG-2"}, {ID: "R2", URL: "https://linear.app/acme/issue/ENG-3"},
	}}}, fm.Published)

	stdout, code = runPublishCmd(t, "nightly-digest")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "unchanged: R1, R2, rfc")
	require.Equal(t, 3, srv.Count("issueCreate"))

	writeSpecFixture(t, root, "nightly-digest", strings.Replace(string(raw),
		"with links.\n", "with links.\n- [ ] **Opt-out**: an operator can turn the digest off.\n", 1))
	stdout, code = runPublishCmd(t, "nightly-digest")
	resp := requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "R3 (create)")
	require.Equal(t, 3, srv.Count("issueCreate"))

	stdout, code = runPublishCmd(t, "nightly-digest", "--update")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "created: R3")
	require.Equal(t, 4, srv.Count("issueCreate"))
	require.Contains(t, srv.ByIdentifier("ENG-1").Description, "ENG-4 — Opt-out")

	srv.ByIdentifier("ENG-1").StateType = "completed"
	stdout, code = runPublishCmd(t, "nightly-digest", "--update")
	resp = requirePublishRefusal(t, stdout, code, publish.ErrConflict)
	require.Contains(t, resp.Message, "closed")
}

func TestPublish_SchemaPublishesFlags(t *testing.T) {
	publishProject(t)
	stdout, code := runPublishCmd(t, "--schema")
	require.Equal(t, 0, code, stdout)
	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	for _, flag := range []string{"target", "repo", "dry-run", "update", "allow-draft", "format"} {
		require.Contains(t, schema.Flags, flag)
	}
	require.Contains(t, schema.Output.Properties, "rfc_url")
	require.Contains(t, schema.Output.Properties, "publication")
}

func TestParseGitHubRepo(t *testing.T) {
	for in, want := range map[string]string{
		"acme/demo":                             "acme/demo",
		"https://github.com/acme/demo":          "acme/demo",
		"https://github.com/acme/demo.git":      "acme/demo",
		"git@github.com:acme/demo.git":          "acme/demo",
		"ssh://git@github.com/acme/demo":        "acme/demo",
		"https://gitlab.com/acme/demo":          "",
		"demo":                                  "",
		"https://github.com/acme/demo/issues/3": "",
	} {
		owner, repo, ok := parseGitHubRepo(in)
		if want == "" {
			require.False(t, ok, in)
			continue
		}
		require.True(t, ok, in)
		require.Equal(t, want, owner+"/"+repo, in)
	}
}

// publishFake is the slice of GitHub's REST API the verb touches.
type publishFake struct {
	mu      sync.Mutex
	url     string
	issues  map[int]*publishFakeIssue
	next    int
	blocked map[int][]int64
	posts   int
}

type publishFakeIssue struct {
	ID     int64
	Number int
	Title  string
	Body   string
	State  string
	Labels []string
}

func (i *publishFakeIssue) json() map[string]any {
	labels := []map[string]string{}
	for _, l := range i.Labels {
		labels = append(labels, map[string]string{"name": l})
	}
	return map[string]any{"id": i.ID, "number": i.Number, "html_url": fmt.Sprintf("https://github.com/acme/demo/issues/%d", i.Number), "title": i.Title, "body": i.Body, "state": i.State, "labels": labels}
}

func newPublishFake(t *testing.T) *publishFake {
	t.Helper()
	f := &publishFake{issues: map[int]*publishFakeIssue{}, next: 1, blocked: map[int][]int64{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func (f *publishFake) created() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}

var (
	fakeIssueRE   = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)$`)
	fakeSubRE     = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)/sub_issues$`)
	fakeBlockedRE = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)/dependencies/blocked_by$`)
	fakeArtifact  = regexp.MustCompile(`"artifact_id=([^"]+)"`)
)

func (f *publishFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer test-token" {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	write := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	path := r.URL.Path
	switch {
	case path == "/repos/acme/demo/labels":
		write(200, []map[string]string{{"name": "epic"}})
	case path == "/repos/acme/demo/issues" && r.Method == http.MethodPost:
		n := f.next
		f.next++
		f.posts++
		is := &publishFakeIssue{ID: int64(1000 + n), Number: n, Title: body["title"].(string), Body: body["body"].(string), State: "open"}
		if raw, ok := body["labels"].([]any); ok {
			for _, l := range raw {
				is.Labels = append(is.Labels, l.(string))
			}
		}
		f.issues[n] = is
		write(201, is.json())
	case fakeIssueRE.MatchString(path) && r.Method == http.MethodPatch:
		n, _ := strconv.Atoi(fakeIssueRE.FindStringSubmatch(path)[1])
		is := f.issues[n]
		is.Title = body["title"].(string)
		is.Body = body["body"].(string)
		write(200, is.json())
	case fakeSubRE.MatchString(path):
		write(201, map[string]any{})
	case fakeBlockedRE.MatchString(path):
		n, _ := strconv.Atoi(fakeBlockedRE.FindStringSubmatch(path)[1])
		f.blocked[n] = append(f.blocked[n], int64(body["issue_id"].(float64)))
		write(201, map[string]any{})
	case path == "/search/issues":
		m := fakeArtifact.FindStringSubmatch(r.URL.Query().Get("q"))
		items := []map[string]any{}
		for n := 1; n < f.next; n++ {
			if m != nil && strings.Contains(f.issues[n].Body, "artifact_id="+m[1]+" ") {
				items = append(items, f.issues[n].json())
			}
		}
		write(200, map[string]any{"items": items})
	default:
		write(404, map[string]string{"message": "no route " + r.Method + " " + path})
	}
}
