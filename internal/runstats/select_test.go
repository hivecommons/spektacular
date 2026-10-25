package runstats

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// selectionLogs holds, newest first:
//
//	e5 implement b codex   (newest)
//	d4 spec      a claude
//	c3 plan      a codex
//	b2 spec      b claude
//	a1 spec      a claude  (oldest)
//
// Two ids share the prefix "abcdef01".
func selectionLogs() *Logs {
	mk := func(id, kind, name, agent string, start float64) *Run {
		return &Run{ID: id, Kind: kind, Name: name, Agent: agent, Start: at(start), End: at(start + 1)}
	}
	return &Logs{Runs: []*Run{
		mk("e5000000000000000000000000000005", "implement", "b", "codex", 50),
		mk("abcdef01000000000000000000000004", "spec", "a", "claude", 40),
		mk("c3000000000000000000000000000003", "plan", "a", "codex", 30),
		mk("abcdef01990000000000000000000002", "spec", "b", "claude", 20),
		mk("a1000000000000000000000000000001", "spec", "a", "claude", 10),
	}}
}

func ids(runs []*Run) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID[:2]
	}
	return out
}

func TestCandidatesNarrowTheRuns(t *testing.T) {
	logs := selectionLogs()
	cases := []struct {
		name string
		f    Filter
		want []string
	}{
		{"no selection, newest first", Filter{}, []string{"e5", "ab", "c3", "ab", "a1"}},
		{"by name", Filter{Name: "a"}, []string{"ab", "c3", "a1"}},
		{"by kind", Filter{Kind: "spec"}, []string{"ab", "ab", "a1"}},
		{"by agent", Filter{Agent: "codex"}, []string{"e5", "c3"}},
		{"by kind and agent", Filter{Kind: "plan", Agent: "codex"}, []string{"c3"}},
		{"by name, kind and agent", Filter{Name: "b", Kind: "spec", Agent: "claude"}, []string{"ab"}},
		{"by full run id", Filter{Runs: []string{"c3000000000000000000000000000003"}}, []string{"c3"}},
		{"by unique prefix", Filter{Runs: []string{"a1000000"}}, []string{"a1"}},
		{"upper-case prefix", Filter{Runs: []string{"A1000000"}}, []string{"a1"}},
		{"named runs in the order given, once each", Filter{Runs: []string{"a1000000", "e5000000", "a1000000"}}, []string{"a1", "e5"}},
		{"named runs narrowed by the other fields", Filter{Kind: "spec", Runs: []string{"a1000000", "e5000000"}}, []string{"a1"}},
		{"nothing matches", Filter{Name: "zzz"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := logs.Candidates(tc.f)
			require.NoError(t, err)
			require.Equal(t, tc.want, ids(got))
		})
	}
}

func TestCandidatesRefusals(t *testing.T) {
	logs := selectionLogs()
	cases := []struct {
		name    string
		f       Filter
		code    string
		matches []string
	}{
		{"unknown kind", Filter{Kind: "epic"}, CodeBadKind, nil},
		{"too short", Filter{Runs: []string{"a1"}}, CodeBadRunID, nil},
		{"not hex", Filter{Runs: []string{"zzzzzzzz"}}, CodeBadRunID, nil},
		{"too long", Filter{Runs: []string{"a10000000000000000000000000000010"}}, CodeBadRunID, nil},
		{"unknown id", Filter{Runs: []string{"ffffffff"}}, CodeRunNotFound, nil},
		{"ambiguous prefix", Filter{Runs: []string{"abcdef01"}}, CodeRunAmbiguous,
			[]string{"abcdef01000000000000000000000004", "abcdef01990000000000000000000002"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := logs.Candidates(tc.f)
			var se *SelectionError
			require.True(t, errors.As(err, &se), "got %v", err)
			require.Equal(t, tc.code, se.Code)
			require.Equal(t, tc.matches, se.Matches)
			require.NotEmpty(t, se.Error())
		})
	}
}

func TestDefaultsPerMode(t *testing.T) {
	logs := selectionLogs()
	all, err := logs.Candidates(Filter{})
	require.NoError(t, err)

	r, err := ForReport(all)
	require.NoError(t, err)
	require.Equal(t, "e5", r.ID[:2], "a report takes the latest run")

	// The latest run is the only implement run: a comparison needs two.
	_, err = ForCompare(all, Filter{})
	require.ErrorIs(t, err, ErrTooFew)

	specs, err := logs.Candidates(Filter{Name: "a"})
	require.NoError(t, err)
	got, err := ForCompare(specs, Filter{Name: "a"})
	require.NoError(t, err)
	// The latest candidate is spec ab(40); the two latest spec runs, oldest first.
	require.Equal(t, []string{"a1", "ab"}, ids(got))

	sum, err := ForSummary(specs, Filter{Name: "a"})
	require.NoError(t, err)
	require.Equal(t, []string{"a1", "ab"}, ids(sum), "a summary takes every run of the latest run's kind, oldest first")

	sum, err = ForSummary(all, Filter{})
	require.NoError(t, err)
	require.Equal(t, []string{"e5"}, ids(sum))
}

func TestModesWithNamedRuns(t *testing.T) {
	logs := selectionLogs()
	f := Filter{Runs: []string{"e5000000", "a1000000"}}
	c, err := logs.Candidates(f)
	require.NoError(t, err)
	got, err := ForCompare(c, f)
	require.NoError(t, err)
	require.Equal(t, []string{"e5", "a1"}, ids(got), "named runs compare in the order given, across kinds")
	sum, err := ForSummary(c, f)
	require.NoError(t, err)
	require.Equal(t, []string{"e5", "a1"}, ids(sum))

	one := Filter{Runs: []string{"e5000000"}}
	c, err = logs.Candidates(one)
	require.NoError(t, err)
	_, err = ForCompare(c, one)
	require.ErrorIs(t, err, ErrTooFew)

	narrowed := Filter{Kind: "spec", Runs: []string{"e5000000", "a1000000"}}
	c, err = logs.Candidates(narrowed)
	require.NoError(t, err)
	_, err = ForCompare(c, narrowed)
	require.ErrorIs(t, err, ErrTooFew)
}

func TestNoRuns(t *testing.T) {
	_, err := ForReport(nil)
	require.ErrorIs(t, err, ErrNoRuns)
	_, err = ForSummary(nil, Filter{})
	require.ErrorIs(t, err, ErrNoRuns)
	_, err = ForCompare(nil, Filter{})
	require.ErrorIs(t, err, ErrTooFew)
}
