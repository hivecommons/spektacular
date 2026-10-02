package epic

import (
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

const epicBody = "## Overview\n\nOne oversized request.\n\n## Specs\n\n- auth\n- billing\n"

func TestRender_ParseRoundTripKeepsSpecsOrderAndDependencies(t *testing.T) {
	in := Epic{
		CreatedDate:    day(2026, time.September, 1),
		DocumentStatus: metadata.StatusFinal,
		ClosedDate:     day(2026, time.September, 20),
		Spec:           "auth",
		Specs: []EpicSpec{
			{Name: "auth", DependsOn: []string{}},
			{Name: "billing", DependsOn: []string{"auth"}},
			{Name: "reports", DependsOn: []string{"billing", "auth"}},
		},
		Sources: []metadata.SourceRef{
			{URI: "https://github.com/hivecommons/spektacular/issues/60", RetrievedDate: "2026-08-30"},
		},
		Body: []byte(epicBody),
	}

	raw, err := in.Render()
	require.NoError(t, err)
	got, err := Parse(raw)
	require.NoError(t, err)

	require.True(t, got.CreatedDate.Equal(day(2026, time.September, 1)))
	require.Equal(t, metadata.StatusFinal, got.DocumentStatus)
	require.True(t, got.ClosedDate.Equal(day(2026, time.September, 20)))
	require.Equal(t, "auth", got.Spec)
	require.Equal(t, []EpicSpec{
		{Name: "auth", DependsOn: []string{}},
		{Name: "billing", DependsOn: []string{"auth"}},
		{Name: "reports", DependsOn: []string{"billing", "auth"}},
	}, got.Specs)
	require.Equal(t, []metadata.SourceRef{
		{URI: "https://github.com/hivecommons/spektacular/issues/60", RetrievedDate: "2026-08-30"},
	}, got.Sources)
	require.Equal(t, epicBody, string(got.Body))
}

func TestParse_FlowStyleDependsOn(t *testing.T) {
	raw := "---\n" +
		"created_date: 2026-09-01\n" +
		"document_status: draft\n" +
		"spec: auth\n" +
		"specs:\n" +
		"  - name: auth\n" +
		"    depends_on: []\n" +
		"  - name: billing\n" +
		"    depends_on: [auth]\n" +
		"  - name: reports\n" +
		"    depends_on: [auth, billing]\n" +
		"---\n\n" +
		epicBody

	got, err := Parse([]byte(raw))
	require.NoError(t, err)
	require.True(t, got.CreatedDate.Equal(day(2026, time.September, 1)))
	require.Equal(t, metadata.StatusDraft, got.DocumentStatus)
	require.True(t, got.ClosedDate.IsZero())
	require.Equal(t, "auth", got.Spec)
	require.Equal(t, []EpicSpec{
		{Name: "auth", DependsOn: []string{}},
		{Name: "billing", DependsOn: []string{"auth"}},
		{Name: "reports", DependsOn: []string{"auth", "billing"}},
	}, got.Specs)
	require.Nil(t, got.Sources)
	require.Equal(t, epicBody, string(got.Body))
}

func TestRender_AlwaysWritesDependsOnAndSpecs(t *testing.T) {
	t.Run("empty and nil depends_on are both written as []", func(t *testing.T) {
		e := Epic{
			CreatedDate:    day(2026, time.September, 1),
			DocumentStatus: metadata.StatusDraft,
			Specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: nil},
			},
		}
		raw, err := e.Render()
		require.NoError(t, err)
		require.Equal(t, 2, strings.Count(string(raw), "depends_on: []\n"), "rendered:\n%s", raw)

		got, err := Parse(raw)
		require.NoError(t, err)
		require.NotNil(t, got.Specs[1].DependsOn, "nil in memory must read back as an empty list")
		require.Empty(t, got.Specs[1].DependsOn)
		require.NoError(t, Validate(got.Specs))
	})

	t.Run("epic with no specs writes specs: []", func(t *testing.T) {
		e := Epic{CreatedDate: day(2026, time.September, 1), DocumentStatus: metadata.StatusDraft}
		raw, err := e.Render()
		require.NoError(t, err)
		require.Contains(t, string(raw), "\nspecs: []\n")
	})
}

func TestRender_Shape(t *testing.T) {
	e := Epic{
		CreatedDate:    day(2026, time.September, 1),
		DocumentStatus: metadata.StatusDraft,
		Specs: []EpicSpec{
			{Name: "auth", DependsOn: []string{}},
			{Name: "billing", DependsOn: []string{"auth"}},
		},
		Body: []byte(epicBody),
	}
	raw, err := e.Render()
	require.NoError(t, err)
	want := "---\n" +
		"created_date: \"2026-09-01\"\n" +
		"document_status: draft\n" +
		"specs:\n" +
		"    - name: auth\n" +
		"      depends_on: []\n" +
		"    - name: billing\n" +
		"      depends_on:\n" +
		"        - auth\n" +
		"---\n\n" +
		epicBody
	require.Equal(t, want, string(raw))
}

func TestParse_NoFrontmatterReturnsBodyOnly(t *testing.T) {
	raw := []byte("# Just a body\n")
	got, err := Parse(raw)
	require.NoError(t, err)
	require.Equal(t, Epic{Body: raw}, got)
}

func TestParse_MalformedFrontmatterErrors(t *testing.T) {
	cases := map[string]string{
		"unterminated":        "---\ncreated_date: 2026-09-01\n",
		"invalid yaml":        "---\nspecs: [\n---\n\nbody\n",
		"specs not a list":    "---\nspecs: auth\n---\n\nbody\n",
		"bad created date":    "---\ncreated_date: yesterday\n---\n\nbody\n",
		"bad closed date":     "---\nclosed_date: someday\n---\n\nbody\n",
		"closing fence glued": "---\nspec: a\n---body\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(raw))
			require.Error(t, err)
		})
	}
}

func TestSpecNamesAndMember(t *testing.T) {
	e := Epic{Specs: []EpicSpec{
		{Name: "auth", DependsOn: []string{}},
		{Name: "billing", DependsOn: []string{"auth"}},
	}}
	require.Equal(t, []string{"auth", "billing"}, e.SpecNames())
	require.Equal(t, []string{}, Epic{}.SpecNames())

	got, ok := e.Member("billing")
	require.True(t, ok)
	require.Equal(t, EpicSpec{Name: "billing", DependsOn: []string{"auth"}}, got)

	got, ok = e.Member("missing")
	require.False(t, ok)
	require.Equal(t, EpicSpec{}, got)
}

func statusPtr(s metadata.DocumentStatus) *metadata.DocumentStatus { return &s }

func TestStamp(t *testing.T) {
	created := day(2026, time.August, 1)
	closed := day(2026, time.August, 15)
	today := day(2026, time.October, 1)

	t.Run("first write stamps created date today and draft", func(t *testing.T) {
		next := Epic{DocumentStatus: metadata.StatusFinal, ClosedDate: closed, Spec: "auth"}
		got, err := Stamp(nil, next, nil, today.Add(13*time.Hour))
		require.NoError(t, err)
		require.True(t, got.CreatedDate.Equal(today))
		require.Equal(t, metadata.StatusDraft, got.DocumentStatus)
		require.True(t, got.ClosedDate.IsZero())
		require.Equal(t, "auth", got.Spec)
	})

	t.Run("existing without created date is a first write", func(t *testing.T) {
		got, err := Stamp(&Epic{DocumentStatus: metadata.StatusFinal}, Epic{}, nil, today)
		require.NoError(t, err)
		require.True(t, got.CreatedDate.Equal(today))
		require.Equal(t, metadata.StatusDraft, got.DocumentStatus)
	})

	t.Run("later write keeps created date, status and closed date", func(t *testing.T) {
		existing := &Epic{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}
		next := Epic{CreatedDate: today, DocumentStatus: metadata.StatusDraft, Body: []byte("new")}
		got, err := Stamp(existing, next, nil, today)
		require.NoError(t, err)
		require.True(t, got.CreatedDate.Equal(created))
		require.Equal(t, metadata.StatusFinal, got.DocumentStatus)
		require.True(t, got.ClosedDate.Equal(closed))
		require.Equal(t, "new", string(got.Body))
	})

	t.Run("final stamps closed date today", func(t *testing.T) {
		existing := &Epic{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
		got, err := Stamp(existing, Epic{}, statusPtr(metadata.StatusFinal), today)
		require.NoError(t, err)
		require.Equal(t, metadata.StatusFinal, got.DocumentStatus)
		require.True(t, got.ClosedDate.Equal(today))
		require.True(t, got.CreatedDate.Equal(created))
	})

	t.Run("final on first write stamps both dates today", func(t *testing.T) {
		got, err := Stamp(nil, Epic{}, statusPtr(metadata.StatusFinal), today)
		require.NoError(t, err)
		require.True(t, got.CreatedDate.Equal(today))
		require.True(t, got.ClosedDate.Equal(today))
	})

	t.Run("a later final keeps the original closed date", func(t *testing.T) {
		existing := &Epic{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}
		got, err := Stamp(existing, Epic{}, statusPtr(metadata.StatusFinal), today)
		require.NoError(t, err)
		require.True(t, got.ClosedDate.Equal(closed))
	})

	t.Run("back to draft clears the closed date", func(t *testing.T) {
		existing := &Epic{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}
		got, err := Stamp(existing, Epic{}, statusPtr(metadata.StatusDraft), today)
		require.NoError(t, err)
		require.Equal(t, metadata.StatusDraft, got.DocumentStatus)
		require.True(t, got.ClosedDate.IsZero())
		require.True(t, got.CreatedDate.Equal(created))
	})

	t.Run("invalid status is refused", func(t *testing.T) {
		existing := &Epic{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
		_, err := Stamp(existing, Epic{}, statusPtr("bogus"), today)
		require.Error(t, err)
	})

	t.Run("stamped epic renders and reads back its dates", func(t *testing.T) {
		got, err := Stamp(nil, Epic{Specs: []EpicSpec{{Name: "auth", DependsOn: []string{}}}}, statusPtr(metadata.StatusFinal), today)
		require.NoError(t, err)
		raw, err := got.Render()
		require.NoError(t, err)
		require.Contains(t, string(raw), "created_date: \"2026-10-01\"\n")
		require.Contains(t, string(raw), "closed_date: \"2026-10-01\"\n")
		back, err := Parse(raw)
		require.NoError(t, err)
		require.True(t, back.CreatedDate.Equal(today))
		require.True(t, back.ClosedDate.Equal(today))
		require.Equal(t, metadata.StatusFinal, back.DocumentStatus)
	})
}
