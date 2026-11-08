package jira_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
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

func TestTarget_FindFallsBackToDescriptionText(t *testing.T) {
	f := jiratest.New(t, true)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	// The site cannot search properties (fake default for Data Center) and
	// one issue lost its property: both paths read the description.
	f.Issues["DEMO-2"].Property = nil

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.Equal(t, 2, f.Count("GET /rest/api/2/search"), "property JQL refused with 400, then the text JQL")
	require.Equal(t, "DEMO-1", existing.RFC.Key)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, existing.Children["R1"].BodyDigest)
	require.Contains(t, existing.Children["R1"].Body, "task=R1")
	require.NotEmpty(t, existing.Children["R2"].BodyDigest)

	// The converted body cannot be compared verbatim, so the child without
	// a property reports a body change; --update repairs it and the property.
	require.Equal(t, "R1 (body)", publish.DescribeChanges(publish.Diff(p, existing)))
	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"R1"}, res.Updated)
	require.NotNil(t, f.Issues["DEMO-2"].Property)
}

func TestTarget_DataCenterEpicLinkFieldAndACField(t *testing.T) {
	f := jiratest.New(t, true)
	target := f.Target()
	target.EpicLinkField = "customfield_10014"
	target.AcceptanceCriteriaField = "customfield_10100"
	p := testPublication(t)
	_, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	story := f.Issues["DEMO-2"]
	require.Equal(t, "", story.Parent)
	require.Equal(t, "DEMO-1", story.Fields["customfield_10014"])
	require.Contains(t, story.Fields["customfield_10100"], "a digest arrives within five minutes of the hour")
	_, hasAC := f.Issues["DEMO-1"].Fields["customfield_10100"]
	require.False(t, hasAC, "the Epic carries no acceptance criteria field")
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
