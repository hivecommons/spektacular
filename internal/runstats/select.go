package runstats

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Kinds are the workflow kinds a selection may name.
var Kinds = []string{"spec", "plan", "implement", "repo"}

// Selection error codes.
const (
	CodeBadKind      = "bad_kind"
	CodeBadRunID     = "bad_run_id"
	CodeRunNotFound  = "run_not_found"
	CodeRunAmbiguous = "run_ambiguous"
)

// ErrNoRuns is returned when no run matches the selection.
var ErrNoRuns = errors.New("no run matches the selection")

// ErrTooFew is returned when a comparison has fewer than two runs.
var ErrTooFew = errors.New("fewer than two runs to compare")

// SelectionError is a selection the logs cannot satisfy: an unknown kind, a
// malformed run id, or a run id that matches no run or several.
type SelectionError struct {
	Code  string
	Value string
	// Matches are the run ids an ambiguous prefix matched.
	Matches []string
}

func (e *SelectionError) Error() string {
	switch e.Code {
	case CodeBadKind:
		return fmt.Sprintf("unknown workflow kind %q; accepted kinds: %s", e.Value, strings.Join(Kinds, ", "))
	case CodeBadRunID:
		return fmt.Sprintf("run id %q is not a run id or a prefix of one: give 8 to 32 hex characters", e.Value)
	case CodeRunNotFound:
		return fmt.Sprintf("no recorded run has an id starting %q", e.Value)
	case CodeRunAmbiguous:
		return fmt.Sprintf("run id prefix %q matches %d runs: %s", e.Value, len(e.Matches), strings.Join(e.Matches, ", "))
	case CodeBadSelection:
		return "--outside-runs lists only the records written outside any workflow run, so it cannot be combined with a run selection"
	}
	return fmt.Sprintf("bad selection %q", e.Value)
}

// Filter selects runs. Every field set narrows the selection; Runs names runs
// by full id or unique prefix.
type Filter struct {
	Name, Kind, Agent string
	Runs              []string
}

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{8,32}$`)

// Candidates returns the runs the filter selects. When runs are named, they
// come in the order given (each once), narrowed by the other fields;
// otherwise every run matching the name, kind and agent, newest first. An
// unknown kind, or a named run that is malformed, unknown or ambiguous, is a
// *SelectionError. No match is an empty list, not an error.
func (l *Logs) Candidates(f Filter) ([]*Run, error) {
	if f.Kind != "" && !slices.Contains(Kinds, f.Kind) {
		return nil, &SelectionError{Code: CodeBadKind, Value: f.Kind}
	}
	matches := func(r *Run) bool {
		return (f.Name == "" || r.Name == f.Name) &&
			(f.Kind == "" || r.Kind == f.Kind) &&
			(f.Agent == "" || r.Agent == f.Agent)
	}

	if len(f.Runs) == 0 {
		var out []*Run
		for _, r := range l.Runs {
			if matches(r) {
				out = append(out, r)
			}
		}
		return out, nil
	}

	var named []*Run
	for _, raw := range f.Runs {
		r, err := l.resolve(raw)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(named, r) {
			named = append(named, r)
		}
	}
	var out []*Run
	for _, r := range named {
		if matches(r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// resolve finds the one run whose id starts with raw.
func (l *Logs) resolve(raw string) (*Run, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	if !runIDPattern.MatchString(id) {
		return nil, &SelectionError{Code: CodeBadRunID, Value: raw}
	}
	var found []*Run
	for _, r := range l.Runs {
		if strings.HasPrefix(r.ID, id) {
			found = append(found, r)
		}
	}
	switch len(found) {
	case 0:
		return nil, &SelectionError{Code: CodeRunNotFound, Value: raw}
	case 1:
		return found[0], nil
	}
	ids := make([]string, len(found))
	for i, r := range found {
		ids[i] = r.ID
	}
	return nil, &SelectionError{Code: CodeRunAmbiguous, Value: raw, Matches: ids}
}

// ForReport picks the run a single-run report covers: the first candidate,
// which with no named runs is the most recent.
func ForReport(c []*Run) (*Run, error) {
	if len(c) == 0 {
		return nil, ErrNoRuns
	}
	return c[0], nil
}

// ForCompare picks the runs a comparison covers: the named runs in the order
// given when two or more are named, otherwise the two most recent candidates
// of the most recent candidate's kind, oldest first. Fewer than two is
// ErrTooFew.
func ForCompare(c []*Run, f Filter) ([]*Run, error) {
	switch {
	case len(f.Runs) >= 2:
		if len(c) < 2 {
			return nil, ErrTooFew
		}
		return c, nil
	case len(f.Runs) == 1:
		return nil, ErrTooFew
	}
	if len(c) == 0 {
		return nil, ErrTooFew
	}
	kind := ofKind(c, c[0].Kind)
	if len(kind) < 2 {
		return nil, ErrTooFew
	}
	return []*Run{kind[1], kind[0]}, nil
}

// ForSummary picks the runs a summary covers: the named runs in the order
// given, otherwise every candidate of the most recent candidate's kind,
// oldest first. None is ErrNoRuns.
func ForSummary(c []*Run, f Filter) ([]*Run, error) {
	if len(c) == 0 {
		return nil, ErrNoRuns
	}
	if len(f.Runs) > 0 {
		return c, nil
	}
	kind := ofKind(c, c[0].Kind)
	slices.Reverse(kind)
	return kind, nil
}

// ofKind is the runs in c of kind, in c's order.
func ofKind(c []*Run, kind string) []*Run {
	var out []*Run
	for _, r := range c {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}
