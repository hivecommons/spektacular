package publish

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/plantask"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return raw
}

// golden compares got against testdata/<name>.golden, rewriting it under
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; run go test ./internal/publish -update")
	require.Equal(t, string(want), got)
}

func specSource(t *testing.T) Source {
	t.Helper()
	return Source{
		Kind:       KindSpec,
		Name:       "nightly-digest",
		ArtifactID: "demo/spec/nightly-digest",
		Target:     "github",
		Body:       fixture(t, "spec.md"),
		Provenance: Provenance{Project: "demo", Repo: "https://github.com/acme/demo", Commit: "abc1234", Version: "v0.0.0-test"},
	}
}

func planSource(t *testing.T) Source {
	t.Helper()
	src := specSource(t)
	plan := plantask.Parse(fixture(t, "plan.md"))
	require.Equal(t, plantask.FormatTasks, plan.Format)
	src.Plan = &plan
	src.PlanName = "nightly-digest"
	return src
}

func epicSource(t *testing.T) Source {
	t.Helper()
	return Source{
		Kind:       KindEpic,
		Name:       "operator-notifications",
		ArtifactID: "demo/epic/operator-notifications",
		Target:     "github",
		Body:       fixture(t, "epic.md"),
		Members: []Member{
			{Name: "nightly-digest", Body: fixture(t, "spec.md"), DependsOn: []string{}},
			{Name: "chat-alerts", Body: fixture(t, "member-chat.md"), DependsOn: []string{"nightly-digest"}},
		},
		DefaultRepo: "demo",
		Provenance:  Provenance{Project: "demo"},
	}
}

func TestBuild_SpecRequirementsBecomeChildren(t *testing.T) {
	p, err := Build(specSource(t))
	require.NoError(t, err)
	require.NoError(t, Validate(p))

	require.Equal(t, "RFC: Nightly digest email", p.RFC.Title)
	require.Equal(t, ChildrenFromRequirements, p.ChildrenFrom)
	require.Equal(t, "<!-- spek:publish name=nightly-digest artifact_id=demo/spec/nightly-digest target=github -->", p.RFC.Marker)
	require.True(t, strings.HasSuffix(p.RFC.Body, p.RFC.Marker+"\n"), "the marker is the RFC body's last line")
	require.NotContains(t, p.RFC.Body, "Guidance", "HTML comments are stripped")
	require.NotContains(t, p.RFC.Body, "## Amendments")

	require.Len(t, p.Children, 3)
	c := p.Children[2]
	require.Equal(t, "R3", c.ID)
	require.Equal(t, "Opt-out", c.Title)
	require.Equal(t, []string{"Opt-out: no digest arrives after the operator opts out."}, c.AcceptanceCriteria, "criteria are traced by bold title")
	require.Equal(t, "<!-- spek:publish name=nightly-digest artifact_id=demo/spec/nightly-digest target=github task=R3 -->", c.Marker)
	require.True(t, strings.HasSuffix(c.Body, c.Marker+"\n"))

	golden(t, "spec-requirements", RenderMarkdown(p))
}

func TestBuild_PlanTasksBecomeChildren(t *testing.T) {
	p, err := Build(planSource(t))
	require.NoError(t, err)
	require.NoError(t, Validate(p))

	require.Equal(t, ChildrenFromPlan, p.ChildrenFrom)
	require.Len(t, p.Children, 3)
	require.Equal(t, "render-2", p.Children[1].ID)
	require.Equal(t, []string{"sched-1"}, p.Children[1].DependsOn)
	require.Equal(t, "api", p.Children[1].Repo)
	require.Equal(t, "human — needs a design review", p.Children[2].Execution)
	require.Contains(t, p.Children[1].Body, "- Blocked by `sched-1` — Add the digest scheduler")
	require.Contains(t, p.RFC.Body, "- [ ] `render-2` — Render the digest email")

	golden(t, "spec-plan", RenderMarkdown(p))
}

func TestBuild_EpicSpecsBecomeChildren(t *testing.T) {
	p, err := Build(epicSource(t))
	require.NoError(t, err)
	require.NoError(t, Validate(p))

	require.Equal(t, "RFC: Operator notifications", p.RFC.Title)
	require.Equal(t, ChildrenFromEpic, p.ChildrenFrom)
	require.NotContains(t, p.RFC.Body, "## Specs", "the specs list is replaced by the children checklist")
	require.Len(t, p.Children, 2)
	require.Equal(t, "chat-alerts", p.Children[1].ID)
	require.Equal(t, "Chat alerts", p.Children[1].Title)
	require.Equal(t, "demo", p.Children[1].Repo, "the project's single repo is the default")
	require.Equal(t, []string{"nightly-digest"}, p.Children[1].DependsOn)

	golden(t, "epic", RenderMarkdown(p))
}

func TestBuild_DraftTitlesTheRFC(t *testing.T) {
	src := specSource(t)
	src.Draft = true
	p, err := Build(src)
	require.NoError(t, err)
	require.Equal(t, "RFC: [DRAFT] Nightly digest email", p.RFC.Title)
}

func TestRender_FinalReferencesReplaceIDs(t *testing.T) {
	p, err := Build(planSource(t))
	require.NoError(t, err)
	refs := map[string]Ref{"sched-1": {Ref: "#11", URL: "https://github.com/acme/demo/issues/11"}}

	rfc := p.RFCBody(refs)
	require.Contains(t, rfc, "- [ ] #11 — Add the digest scheduler")
	require.Contains(t, rfc, "- [ ] `render-2` — Render the digest email", "a child with no ref is listed by id")

	child := p.Children[1].Render(p, "https://github.com/acme/demo/issues/10", refs)
	require.Contains(t, child, "- Blocked by #11 — Add the digest scheduler")
	require.Contains(t, child, "- Part of RFC: https://github.com/acme/demo/issues/10")
	m, ok := ParseMarker(child)
	require.True(t, ok)
	require.Equal(t, Marker{Name: "nightly-digest", ArtifactID: "demo/spec/nightly-digest", Target: "github", Task: "render-2", RFC: "https://github.com/acme/demo/issues/10"}, m)
}

func TestMarker_RoundTrip(t *testing.T) {
	cases := []Marker{
		{Name: "n", ArtifactID: "p/spec/n", Target: "github"},
		{Name: "n", ArtifactID: "p/spec/n", Target: "github", Task: "R1"},
		{Name: "n", ArtifactID: "p/epic/n", Target: "github", Task: "member", RFC: "https://github.com/o/r/issues/1"},
	}
	for _, m := range cases {
		got, ok := ParseMarker("some body\n\n" + m.String() + "\n")
		require.True(t, ok, m.String())
		require.Equal(t, m, got)
	}
	require.Equal(t, "<!-- spek:publish name=n artifact_id=p/spec/n target=github -->", cases[0].String())
	require.Equal(t, "<!-- spek:publish name=n artifact_id=p/spec/n target=github task=R1 -->", cases[1].String())
	require.True(t, cases[0].IsRFC())
	require.False(t, cases[1].IsRFC())

	_, ok := ParseMarker("<!-- hive:parent 12 -->")
	require.False(t, ok)
	_, ok = ParseMarker("<!-- spek:publish name=n -->")
	require.False(t, ok, "a marker missing artifact_id or target is not one")
}

func TestValidate_Refusals(t *testing.T) {
	base := func(t *testing.T) Publication {
		p, err := Build(planSource(t))
		require.NoError(t, err)
		return p
	}
	code := func(err error) string {
		var perr *Error
		require.ErrorAs(t, err, &perr)
		require.NotEmpty(t, perr.NextAction)
		return perr.Code
	}

	t.Run("no children", func(t *testing.T) {
		src := specSource(t)
		src.Body = []byte("# Feature: Empty\n\n## Overview\n\nNothing here yet.\n")
		p, err := Build(src)
		require.NoError(t, err)
		require.Equal(t, ErrNoChildren, code(Validate(p)))
	})
	t.Run("unknown dependency", func(t *testing.T) {
		p := base(t)
		p.Children[0].DependsOn = []string{"nope"}
		require.Equal(t, ErrDependencyCycle, code(Validate(p)))
	})
	t.Run("cycle", func(t *testing.T) {
		p := base(t)
		p.Children[0].DependsOn = []string{"optout-3"}
		require.Equal(t, ErrDependencyCycle, code(Validate(p)))
	})
	t.Run("short body", func(t *testing.T) {
		p := base(t)
		p.Children[0].Body = "tiny\n" + p.Children[0].Marker
		err := Validate(p)
		require.Equal(t, ErrBodyHygiene, code(err))
		require.Contains(t, err.Error(), "fewer than 80 characters")
	})
	t.Run("placeholder", func(t *testing.T) {
		p := base(t)
		p.RFC.Body = strings.Replace(p.RFC.Body, "Operators want", "<team> want", 1)
		err := Validate(p)
		require.Equal(t, ErrBodyHygiene, code(err))
		require.Contains(t, err.Error(), "<team>")
	})
	t.Run("html tag is not a placeholder", func(t *testing.T) {
		p := base(t)
		p.RFC.Body = strings.Replace(p.RFC.Body, "Operators want", "Operators <br> want", 1)
		require.NoError(t, Validate(p))
	})
	t.Run("unchosen option list", func(t *testing.T) {
		p := base(t)
		p.Children[1].Body = strings.Replace(p.Children[1].Body, "## What to build", "Priority: high/medium/low\n\n## What to build", 1)
		require.Equal(t, ErrBodyHygiene, code(Validate(p)))
	})
	t.Run("child without criteria", func(t *testing.T) {
		p := base(t)
		p.Children[0].AcceptanceCriteria = nil
		require.Equal(t, ErrBodyHygiene, code(Validate(p)))
	})
}

func TestDiff(t *testing.T) {
	p, err := Build(planSource(t))
	require.NoError(t, err)
	rfcURL := "https://github.com/acme/demo/issues/10"
	refs := map[string]Ref{}
	existing := &Existing{Children: map[string]Filed{}}
	for i, c := range p.Children {
		n := 11 + i
		refs[c.ID] = Ref{Ref: "#" + strconv.Itoa(n), URL: "https://github.com/acme/demo/issues/" + strconv.Itoa(n)}
	}
	for _, c := range p.Children {
		existing.Children[c.ID] = Filed{Number: 11, URL: refs[c.ID].URL, Ref: refs[c.ID].Ref, Title: c.Title, Body: c.Render(p, rfcURL, refs) + "  \r\n"}
	}
	existing.RFC = &Filed{Number: 10, URL: rfcURL, Ref: "#10", Title: p.RFC.Title, Body: p.RFCBody(refs)}

	require.Nil(t, Diff(p, nil), "nothing filed means nothing to compare")
	require.Empty(t, Diff(p, existing), "an unchanged publication diffs clean despite whitespace")

	p.Children[1].Title = "Render the digest email, nicely"
	p.Children = append(p.Children, p.Children[0])
	p.Children[3].ID = "new-4"
	changes := Diff(p, existing)
	require.Equal(t, "rfc (body), render-2 (title), optout-3 (body), new-4 (create)", DescribeChanges(changes),
		"the renamed child changes the RFC checklist and the body of the child blocked by it")
}

func TestSplitSections_IgnoresFencedHeadings(t *testing.T) {
	secs := splitSections("intro\n## A\n\n```\n## not a heading\n```\n\n## B\nb\n")
	require.Len(t, secs, 3)
	require.Equal(t, "A", secs[1].Heading)
	require.Contains(t, secs[1].Body, "## not a heading")
	require.Equal(t, "B", secs[2].Heading)
}

func TestListItems_ContinuationAndBold(t *testing.T) {
	items := listItems("- [ ] **One**: first\n  continued\n- plain item. With more.\n\nprose\n- [x] **Two**\n")
	require.Len(t, items, 3)
	require.Equal(t, listItem{Title: "One", Bold: "One", Text: "One: first continued"}, items[0])
	require.Equal(t, "plain item", items[1].Title)
	require.Equal(t, listItem{Title: "Two", Bold: "Two", Text: "Two"}, items[2])
}
