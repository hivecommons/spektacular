package jira_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/jira"
	"github.com/hivecommons/spektacular/internal/publish/jira/jiratest"
)

func testPublication(t *testing.T) publish.Publication {
	t.Helper()
	body := []byte(`# Feature: Nightly digest

## Overview

Operators want one email per day summarising what the hive filed, merged and
parked, so they can skim the night's work without opening the dashboard.

## Requirements

- [ ] **Schedule**: send one digest per day at the configured hour, in the operator's time zone.
- [ ] **Content**: list filed, merged and parked items with links back to each.

## Acceptance Criteria

- [ ] **Schedule**: a digest arrives within five minutes of the hour.
- [ ] **Content**: every item of the last 24 hours appears exactly once.
`)
	p, err := publish.Build(publish.Source{Kind: publish.KindSpec, Name: "nightly-digest", ArtifactID: "demo/spec/nightly-digest", Target: "jira", Body: body})
	require.NoError(t, err)
	p.Children[1].DependsOn = []string{"R1"}
	require.NoError(t, publish.Validate(p))
	return p
}

func TestTarget_PublishCreatesEpicThenStoriesAndLinks(t *testing.T) {
	for _, dc := range []bool{false, true} {
		name := "cloud"
		if dc {
			name = "datacenter"
		}
		t.Run(name, func(t *testing.T) {
			f := jiratest.New(t, dc)
			target := f.Target()
			p := testPublication(t)
			ctx := context.Background()

			existing, err := target.Find(ctx, p.ArtifactID)
			require.NoError(t, err)
			require.Nil(t, existing)

			res, err := target.Publish(ctx, p, nil, publish.PublishOptions{Labels: []string{"spek publish", "hive"}})
			require.NoError(t, err)
			require.Empty(t, res.Warnings)
			require.Equal(t, "DEMO", res.Repo)
			require.Equal(t, "DEMO-1", res.RFCKey)
			require.Equal(t, target.BaseURL+"/browse/DEMO-1", res.RFCURL)
			require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
			require.Equal(t, []publish.ResultChild{
				{ID: "R1", URL: target.BaseURL + "/browse/DEMO-2", Key: "DEMO-2"},
				{ID: "R2", URL: target.BaseURL + "/browse/DEMO-3", Key: "DEMO-3"},
			}, res.Children)

			epic := f.Issues["DEMO-1"]
			require.Equal(t, "Epic", epic.Type)
			require.Equal(t, "RFC: Nightly digest", epic.Summary)
			require.Equal(t, []string{"spek-publish", "hive"}, epic.Labels, "whitespace in labels becomes -")
			require.Equal(t, "", epic.Parent)
			require.Contains(t, f.DescriptionText("DEMO-1"), "DEMO-2 — Schedule")
			require.Contains(t, f.DescriptionText("DEMO-1"), "DEMO-3 — Content")
			require.Equal(t, map[string]any{"name": "nightly-digest", "artifact_id": "demo/spec/nightly-digest", "target": "jira", "title": "RFC: Nightly digest", "body_digest": epic.Property["body_digest"]}, epic.Property)
			require.NotEmpty(t, epic.Property["body_digest"])

			story := f.Issues["DEMO-3"]
			require.Equal(t, "Story", story.Type)
			require.Equal(t, "DEMO-1", story.Parent, "children sit under the Epic")
			require.Equal(t, []string{"spek-publish", "hive"}, story.Labels)
			text := f.DescriptionText("DEMO-3")
			require.Contains(t, text, "Blocked by DEMO-2 — Schedule")
			require.Contains(t, text, "Part of RFC: "+target.BaseURL+"/browse/DEMO-1")
			m, ok := publish.ParseMarker(text)
			require.True(t, ok, "the marker survives the description conversion")
			require.Equal(t, "R2", m.Task)
			require.Equal(t, target.BaseURL+"/browse/DEMO-1", m.RFC)
			require.Equal(t, "R2", story.Property["task"])
			require.Equal(t, target.BaseURL+"/browse/DEMO-1", story.Property["rfc"])

			require.Equal(t, []string{"DEMO-2 blocks DEMO-3"}, f.Links)
			api := "/rest/api/3/"
			if dc {
				api = "/rest/api/2/"
			}
			require.Equal(t, 3, f.Count("POST "+api+"issue"))
		})
	}
}

func TestTarget_FindThenRepublishIsIdempotent(t *testing.T) {
	f := jiratest.New(t, false)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	// A stray issue for another artifact must not be picked up.
	f.Add(&jiratest.Issue{ID: "99", Key: "DEMO-99", Property: map[string]any{"name": "other", "artifact_id": "demo/spec/other", "target": "jira"}})

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.NotNil(t, existing.RFC)
	require.Equal(t, "DEMO-1", existing.RFC.Key)
	require.Equal(t, "DEMO-1", existing.RFC.Ref)
	require.NotEmpty(t, existing.RFC.BodyDigest, "the body is compared by the recorded digest")
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, publish.Diff(p, existing), "what was filed matches what would be filed")

	created := f.Count("POST /rest/api/3/issue")
	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.Equal(t, []string{"R1", "R2", "rfc"}, res.Unchanged)
	require.Equal(t, created, f.Count("POST /rest/api/3/issue"), "nothing is created twice")
	require.Zero(t, f.Count("PUT /rest/api/3/issue/DEMO-2"), "an unchanged child is not touched")

	// A changed child title is updated in place; a new child is created and
	// linked; the Epic checklist grows.
	p.Children[0].Title = "Schedule the digest"
	p.Children = append(p.Children, publish.Child{})
	p.Children[2] = p.Children[1]
	p.Children[2].ID = "R3"
	p.Children[2].Title = "Opt out"
	p.Children[2].DependsOn = []string{"R2"}
	p.Children[2].Marker = strings.Replace(p.Children[1].Marker, "task=R2", "task=R3", 1)
	require.Equal(t, "rfc (body), R1 (title), R2 (body), R3 (create)", publish.DescribeChanges(publish.Diff(p, existing)))

	res, err = target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"R3"}, res.Created)
	require.ElementsMatch(t, []string{"R1", "R2", "rfc"}, res.Updated)
	require.Empty(t, res.Unchanged)
	require.Equal(t, "Schedule the digest", f.Issues["DEMO-2"].Summary)
	require.Equal(t, "Schedule the digest", f.Issues["DEMO-2"].Property["title"])
	require.Contains(t, f.DescriptionText("DEMO-1"), "DEMO-100 — Opt out")
	require.Equal(t, "DEMO-1", f.Issues["DEMO-100"].Parent)
	require.Equal(t, []string{"DEMO-2 blocks DEMO-3", "DEMO-3 blocks DEMO-100"}, f.Links)

	// A third run after the update is a no-op again.
	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Empty(t, publish.Diff(p, existing))
}

func TestTarget_FindSearchesTokenAndReadsDescriptionWithoutProperty(t *testing.T) {
	f := jiratest.New(t, true)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	// One issue lost its property: its marker is read from the description.
	f.Issues["DEMO-2"].Property = nil

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.Equal(t, 1, f.Count("GET /rest/api/2/search"), "one text JQL on the search token, no property JQL")
	require.Equal(t, "DEMO-1", existing.RFC.Key)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, existing.Children["R1"].BodyDigest)
	require.Contains(t, existing.Children["R1"].Body, "task=R1")
	require.NotEmpty(t, existing.Children["R2"].BodyDigest)
	require.Equal(t, "DEMO-1", existing.Children["R1"].Parent)
	require.Equal(t, []string{"DEMO-2"}, existing.Children["R2"].BlockedBy)

	// The converted body cannot be compared verbatim, so the child without
	// a property reports a body change; --update repairs it and the property.
	require.Equal(t, "R1 (body)", publish.DescribeChanges(publish.Diff(p, existing)))
	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"R1"}, res.Updated)
	require.NotNil(t, f.Issues["DEMO-2"].Property)
	require.Equal(t, 0, f.Count("PUT /rest/api/2/issue/DEMO-2/properties/"+jira.PropertyKey), "properties travel in the issue edit, not a separate write")
}

func TestTarget_SearchTokenIsPlainJQLText(t *testing.T) {
	tok := publish.SearchToken("demo/spec/digest-v2.md")
	require.Regexp(t, `^spekpub[0-9a-f]{16}$`, tok)
	require.NotEqual(t, tok, publish.SearchToken("demo/spec/digest-v3.md"))
}

func TestTarget_PropertiesAreSetOnCreate(t *testing.T) {
	f := jiratest.New(t, false)
	target := f.Target()
	p := testPublication(t)
	_, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	for _, key := range []string{"DEMO-1", "DEMO-2", "DEMO-3"} {
		require.NotNil(t, f.Issues[key].Property, key)
		require.Equal(t, 0, f.Count("PUT /rest/api/3/issue/"+key+"/properties/"+jira.PropertyKey))
	}
	require.Equal(t, p.ArtifactID, f.Issues["DEMO-1"].Property["artifact_id"])
}

func TestTarget_FindReadsHintedIssuesTheIndexHasNotCaughtUpWith(t *testing.T) {
	f := jiratest.New(t, false)
	f.SearchLag = true
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	res, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Nil(t, existing, "the search index has not seen the new issues")

	urls := []string{res.RFCURL}
	for _, c := range res.Children {
		urls = append(urls, c.URL)
	}
	target.Hint(urls)
	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.Equal(t, "DEMO-1", existing.RFC.Key)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, publish.Diff(p, existing))

	// A hint for an issue that no longer exists is skipped, not an error.
	target.Hint([]string{f.URL + "/browse/DEMO-404"})
	_, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
}

func TestTarget_PublishFailingPartWayReportsWhatWasFiled(t *testing.T) {
	f := jiratest.New(t, false)
	f.FailCreateAfter = 3
	target := f.Target()
	p := testPublication(t)
	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 500")
	require.NotNil(t, res)
	require.Equal(t, f.URL+"/browse/DEMO-1", res.RFCURL)
	require.Len(t, res.Children, 1)
	require.Equal(t, "R1", res.Children[0].ID)
}

func TestTarget_ReconcilesParentAndLinksOnEveryRun(t *testing.T) {
	f := jiratest.New(t, false)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	f.FailLinks = true
	res, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Len(t, res.Warnings, 1)
	require.Empty(t, f.Links)
	// An operator detached a child from the Epic by hand.
	f.Issues["DEMO-2"].Parent = ""

	f.FailLinks = false
	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Empty(t, publish.Diff(p, existing), "a missing link is not a content change")
	res, err = target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.ElementsMatch(t, []string{"rfc", "R1", "R2"}, res.Unchanged)
	require.Equal(t, []string{"DEMO-2 under DEMO-1", "DEMO-3 blocked by DEMO-2"}, res.Linked)
	require.Equal(t, "DEMO-1", f.Issues["DEMO-2"].Parent)
	require.Equal(t, []string{"DEMO-2 blocks DEMO-3"}, f.Links)

	// Once repaired, a further run touches nothing.
	before := f.Calls()
	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	res, err = target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Linked)
	require.Equal(t, before+4, f.Calls(), "the search and one property read per issue; no edits")
}

func TestTarget_DataCenterDiscoversEpicLinkField(t *testing.T) {
	f := jiratest.New(t, true)
	target := f.Target()
	target.AcceptanceCriteriaField = "customfield_10100"
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, f.Count("GET /rest/api/2/field"), "the Epic Link field is discovered once")
	require.Equal(t, jiratest.EpicLinkField, target.EpicLinkField)
	story := f.Issues["DEMO-2"]
	require.Equal(t, "DEMO-1", story.Parent)
	_, sentParent := story.Fields["parent"]
	require.False(t, sentParent, "Data Center Stories take the Epic Link field, not parent")
	require.Equal(t, "DEMO-1", story.Fields[jiratest.EpicLinkField])
	require.Contains(t, story.Fields["customfield_10100"], "a digest arrives within five minutes of the hour")
	_, hasAC := f.Issues["DEMO-1"].Fields["customfield_10100"]
	require.False(t, hasAC, "the Epic carries no acceptance criteria field")

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Equal(t, "DEMO-1", existing.Children["R1"].Parent, "Find reads the Epic Link field back")
	_, err = target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, f.Count("GET /rest/api/2/field"), "the discovered field is kept")
}

func TestTarget_DataCenterRefusesParentOnStories(t *testing.T) {
	f := jiratest.New(t, true)
	target := f.Target()
	target.EpicLinkField = "customfield_99999" // not the site's field: the fake ignores it
	p := testPublication(t)
	_, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, "", f.Issues["DEMO-2"].Parent, "a wrong field silently links nothing")

	// Sending parent itself is what the real site refuses.
	bad := jiratest.New(t, true)
	bt := bad.Target()
	bt.EpicLinkField = "parent"
	_, err = bt.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not belong to appropriate hierarchy")
}

func TestTarget_LinkFailuresAreWarnings(t *testing.T) {
	f := jiratest.New(t, false)
	target := f.Target()
	f.FailLinks = true
	p := testPublication(t)
	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Len(t, res.Children, 2)
	require.Len(t, res.Warnings, 1)
	require.Contains(t, res.Warnings[0], "could not link DEMO-3 as blocked by DEMO-2")
	require.Contains(t, res.Warnings[0], "HTTP 404: No issue link type with name 'Blocks' found.")
}

func TestTarget_BadCredentialsFail(t *testing.T) {
	target := jiratest.New(t, false).Target()
	target.APIToken = "wrong"
	_, err := target.Find(context.Background(), "demo/spec/x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 401: Unauthorized")
}
