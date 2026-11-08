package github_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/github/githubtest"
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
	p, err := publish.Build(publish.Source{Kind: publish.KindSpec, Name: "nightly-digest", ArtifactID: "demo/spec/nightly-digest", Target: "github", Body: body})
	require.NoError(t, err)
	// R2 depends on R1 so the blocked-by path is exercised.
	p.Children[1].DependsOn = []string{"R1"}
	require.NoError(t, publish.Validate(p))
	return p
}

func TestTarget_PublishCreatesRFCThenChildrenAndLinks(t *testing.T) {
	f := githubtest.New(t)
	target := f.Target()
	p := testPublication(t)

	existing, err := target.Find(context.Background(), p.ArtifactID)
	require.NoError(t, err)
	require.Nil(t, existing)

	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{ParentLabel: "epic", Labels: []string{"spek", "bug"}})
	require.NoError(t, err)

	require.Equal(t, "https://github.com/acme/demo/issues/1", res.RFCURL)
	require.Equal(t, 1, res.RFCNumber)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, []publish.ResultChild{{ID: "R1", URL: "https://github.com/acme/demo/issues/2", Number: 2}, {ID: "R2", URL: "https://github.com/acme/demo/issues/3", Number: 3}}, res.Children)
	require.Len(t, res.Warnings, 1, "the one missing label is warned about once")
	require.Contains(t, res.Warnings[0], `"spek" does not exist`)

	rfc := f.Issues[1]
	require.Equal(t, "RFC: Nightly digest", rfc.Title)
	require.Equal(t, []string{"epic", "bug"}, rfc.Labels, "existing labels applied; the missing one never created")
	require.Contains(t, rfc.Body, "- [ ] #2 — Schedule")
	require.Contains(t, rfc.Body, "- [ ] #3 — Content")
	require.True(t, strings.HasSuffix(strings.TrimSpace(rfc.Body), p.RFC.Marker))

	child := f.Issues[3]
	require.Equal(t, []string{"bug"}, child.Labels, "the parent label goes on the RFC only")
	require.Contains(t, child.Body, "- Blocked by #2 — Schedule")
	require.Contains(t, child.Body, "- Part of RFC: https://github.com/acme/demo/issues/1")
	m, ok := publish.ParseMarker(child.Body)
	require.True(t, ok)
	require.Equal(t, "R2", m.Task)
	require.Equal(t, "https://github.com/acme/demo/issues/1", m.RFC)

	require.Equal(t, []int64{1002, 1003}, f.Subs[1], "children are sub-issues of the RFC, by database id")
	require.Equal(t, []int64{1002}, f.Blocked[3], "R2 is blocked by R1, by database id")
	require.Equal(t, 1, f.Count("GET /repos/acme/demo/labels"))
	require.Equal(t, 3, f.Count("POST /repos/acme/demo/issues"))
}

func TestTarget_FindThenRepublishIsIdempotent(t *testing.T) {
	f := githubtest.New(t)
	target := f.Target()
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	// A stray issue for another artifact must not be picked up.
	f.Issues[99] = &githubtest.Issue{ID: 9999, Number: 99, URL: "u", Body: "<!-- spek:publish name=other artifact_id=demo/spec/other target=github -->"}

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.NotNil(t, existing.RFC)
	require.Equal(t, 1, existing.RFC.Number)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, publish.Diff(p, existing), "what was filed matches what would be filed")

	created := f.Count("POST /repos/acme/demo/issues")
	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.Equal(t, []string{"R1", "R2", "rfc"}, res.Unchanged)
	require.Equal(t, created, f.Count("POST /repos/acme/demo/issues"), "nothing is created twice")

	// A changed child title is patched in place; a new child is created and
	// linked; the RFC checklist grows.
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
	require.Equal(t, "Schedule the digest", f.Issues[2].Title)
	require.Contains(t, f.Issues[1].Body, "- [ ] #4 — Opt out")
	require.Equal(t, []int64{1002, 1003, 1004}, f.Subs[1])
	require.Equal(t, []int64{1003}, f.Blocked[4])
}

func TestTarget_RelationFailuresAreWarnings(t *testing.T) {
	f := githubtest.New(t)
	target := f.Target()
	f.FailSubs = true
	p := testPublication(t)
	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Len(t, res.Children, 2)
	require.Len(t, res.Warnings, 2)
	require.Contains(t, res.Warnings[0], "could not add #2 as a sub-issue of #1")
	require.Contains(t, res.Warnings[0], "HTTP 422: sub-issues disabled")
}

func TestTarget_BadCredentialsFail(t *testing.T) {
	target := githubtest.New(t).Target()
	target.Token = "wrong"
	_, err := target.Find(context.Background(), "demo/spec/x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 401: Bad credentials")
}
