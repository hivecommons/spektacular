package linear_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/linear/lineartest"
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
	p, err := publish.Build(publish.Source{Kind: publish.KindSpec, Name: "nightly-digest", ArtifactID: "demo/spec/nightly-digest", Target: "linear", Body: body})
	require.NoError(t, err)
	p.Children[1].DependsOn = []string{"R1"}
	require.NoError(t, publish.Validate(p))
	return p
}

func TestTarget_PublishCreatesIssueThenSubIssuesAndRelations(t *testing.T) {
	f := lineartest.New(t)
	target := f.Target()
	target.State = lineartest.Todo
	p := testPublication(t)
	ctx := context.Background()

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Nil(t, existing)

	res, err := target.Publish(ctx, p, nil, publish.PublishOptions{ParentLabel: "epic", Labels: []string{"spek", "nope"}})
	require.NoError(t, err)
	require.Equal(t, []string{`configured label "nope" does not exist in team ENG; not applied (publish never creates labels)`}, res.Warnings)
	require.Equal(t, "ENG", res.Repo)
	require.Equal(t, "ENG-1", res.RFCKey)
	require.Equal(t, "https://linear.app/acme/issue/ENG-1", res.RFCURL)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, []publish.ResultChild{
		{ID: "R1", URL: "https://linear.app/acme/issue/ENG-2", Key: "ENG-2"},
		{ID: "R2", URL: "https://linear.app/acme/issue/ENG-3", Key: "ENG-3"},
	}, res.Children)

	rfc := f.ByIdentifier("ENG-1")
	require.Equal(t, "RFC: Nightly digest", rfc.Title)
	require.Equal(t, "", rfc.ParentID)
	require.Equal(t, []string{lineartest.LabelID("epic"), lineartest.LabelID("spek")}, rfc.LabelIDs, "parent label on the RFC only, missing ones skipped")
	require.Equal(t, lineartest.StateID(lineartest.Todo), rfc.StateID)
	require.Contains(t, rfc.Description, "ENG-2 — Schedule")
	require.Contains(t, rfc.Description, "ENG-3 — Content")
	require.True(t, strings.HasSuffix(strings.TrimSpace(rfc.Description), "target=linear -->"), "marker is the last line")

	child := f.ByIdentifier("ENG-3")
	require.Equal(t, rfc.ID, child.ParentID, "children are sub-issues of the RFC")
	require.Equal(t, []string{lineartest.LabelID("spek")}, child.LabelIDs)
	require.Contains(t, child.Description, "Blocked by ENG-2 — Schedule")
	require.Contains(t, child.Description, "Part of RFC: https://linear.app/acme/issue/ENG-1")
	m, ok := publish.ParseMarker(child.Description)
	require.True(t, ok)
	require.Equal(t, "R2", m.Task)
	require.Equal(t, "https://linear.app/acme/issue/ENG-1", m.RFC)

	require.Equal(t, []string{"ENG-2 blocks ENG-3"}, f.Relations)
	require.Equal(t, 3, f.Count("issueCreate"))
	require.Equal(t, 1, f.Count("teams"), "the team is read once per publish")
}

func TestTarget_FindThenRepublishIsIdempotent(t *testing.T) {
	f := lineartest.New(t)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	f.Add(&lineartest.Issue{Title: "other", Description: "x\n\n<!-- spek:publish name=other artifact_id=demo/spec/other target=linear -->\n"})

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.NotNil(t, existing.RFC)
	require.Equal(t, "uuid-1", existing.RFC.Key, "Key is the UUID mutations take")
	require.Equal(t, "ENG-1", existing.RFC.Ref)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, publish.Diff(p, existing), "Linear stores Markdown, so the body round-trips")

	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.Equal(t, []string{"R1", "R2", "rfc"}, res.Unchanged)
	require.Equal(t, 3, f.Count("issueCreate"), "nothing is created twice")
	require.Equal(t, 1, f.Count("issueUpdate"), "only the first run's RFC checklist rewrite")

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
	require.Equal(t, "Schedule the digest", f.ByIdentifier("ENG-2").Title)
	require.Contains(t, f.ByIdentifier("ENG-1").Description, "ENG-5 — Opt out")
	require.Equal(t, "uuid-1", f.ByIdentifier("ENG-5").ParentID)
	require.Equal(t, []string{"ENG-2 blocks ENG-3", "ENG-3 blocks ENG-5"}, f.Relations)

	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Empty(t, publish.Diff(p, existing))
}

func TestTarget_ClosedItemsAreReported(t *testing.T) {
	f := lineartest.New(t)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	f.ByIdentifier("ENG-1").StateType = "completed"
	f.ByIdentifier("ENG-2").StateType = "canceled"

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.True(t, existing.RFC.Closed)
	require.True(t, existing.Children["R1"].Closed)
	require.False(t, existing.Children["R2"].Closed)
}

func TestTarget_RelationFailuresAreWarnings(t *testing.T) {
	f := lineartest.New(t)
	f.FailRelations = true
	res, err := f.Target().Publish(context.Background(), testPublication(t), nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Len(t, res.Warnings, 1)
	require.Contains(t, res.Warnings[0], "could not mark ENG-3 as blocked by ENG-2")
	require.Contains(t, res.Warnings[0], "relation refused")
	require.Equal(t, 3, f.Count("issueCreate"), "the items were still filed")
}

func TestTarget_RefusesBeforeFilingWhenTeamOrStateIsUnknown(t *testing.T) {
	t.Run("team", func(t *testing.T) {
		f := lineartest.New(t)
		target := f.Target()
		target.TeamKey = "NOPE"
		_, err := target.Publish(context.Background(), testPublication(t), nil, publish.PublishOptions{})
		require.ErrorContains(t, err, `team "NOPE" does not exist`)
		require.Zero(t, f.Count("issueCreate"))
	})
	t.Run("state", func(t *testing.T) {
		f := lineartest.New(t)
		target := f.Target()
		target.State = "Shipped"
		_, err := target.Publish(context.Background(), testPublication(t), nil, publish.PublishOptions{})
		require.ErrorContains(t, err, `no workflow state named "Shipped"`)
		require.Zero(t, f.Count("issueCreate"))
	})
}

func TestTarget_BadAPIKey(t *testing.T) {
	f := lineartest.New(t)
	target := f.Target()
	target.APIKey = "wrong"
	_, err := target.Find(context.Background(), "demo/spec/nightly-digest")
	require.ErrorContains(t, err, "linear API 401")
	require.ErrorContains(t, err, "not authenticated")
}
