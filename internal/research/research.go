// Package research seeds a spec's research step with what the project, and the
// environment it runs in, already know.
//
// A Source is one place prior knowledge lives: the configured knowledge stores,
// the ADR folders inside registered repos, the Hive knowledge export when
// running under Hive, and Context7 library docs. Every source answers the same
// question — "what do you hold about this query?" — and returns findings that
// carry a citation, so the notes a spec is drafted from can name where each
// piece of prior knowledge came from.
//
// Sources are pluggable in the same way storage providers are: Seed takes a
// list, and adding a source is implementing the interface and appending it.
// What is deliberately not pluggable is failure. Only the local knowledge
// stores are the project's own; every other source depends on the environment,
// so a source that is not configured reports itself skipped and one that fails
// reports itself unavailable, each with a reason, and Seed carries on. A spec
// started standalone therefore degrades to local knowledge without an error.
package research

import (
	"context"
	"errors"
	"fmt"
)

// Status is what became of one source during a seed.
type Status string

const (
	// StatusUsed means the source was searched. It may still have held nothing.
	StatusUsed Status = "used"
	// StatusSkipped means the source is not configured in this environment.
	StatusSkipped Status = "skipped"
	// StatusUnavailable means the source is configured but could not be
	// searched: a missing export, a network failure, a refused request.
	StatusUnavailable Status = "unavailable"
)

// Finding is one piece of prior knowledge a source holds about the query.
type Finding struct {
	// Title is the entry's heading, or its locator when it has none.
	Title string `json:"title"`
	// Cite is the stable citation to record in research notes, prefixed with
	// the source name: "knowledge:<tier>/<name>/<path>", "adr:<repo>/<path>",
	// "hive:<section>/<title>" or "context7:<library id>".
	Cite string `json:"cite"`
	// Excerpts are the strongest matching lines, or a library description.
	Excerpts []string `json:"excerpts"`
	// Tags are the entry's declared tags; empty when it declares none.
	Tags []string `json:"tags"`
	// Score is the knowledge ranking score. Context7 results are ordered by
	// Context7 and carry none.
	Score float64 `json:"score,omitempty"`
	// Read tells the agent how to fetch the full body: a CLI command, a file
	// path, or a URL.
	Read string `json:"read"`
}

// Report is the outcome of one source.
type Report struct {
	Source   string    `json:"source"`
	Status   Status    `json:"status"`
	Reason   string    `json:"reason,omitempty"`
	Findings []Finding `json:"findings"`
}

// Source is one place prior knowledge lives.
type Source interface {
	// Name is the source's identifier, also the prefix of its citations.
	Name() string
	// Search returns up to limit findings for the lower-cased query terms,
	// strongest first. An error wrapping ErrSkipped marks the source as not
	// configured here; any other error marks it unavailable.
	Search(ctx context.Context, terms []string, limit int) ([]Finding, error)
}

// ErrSkipped is wrapped by a source that is not configured in this
// environment. The wrapping message is the reason reported.
var ErrSkipped = errors.New("skipped")

// skipped builds an ErrSkipped carrying reason.
func skipped(reason string) error {
	return fmt.Errorf("%s: %w", reason, ErrSkipped)
}

// Seed searches every source in order and reports each one. It never fails
// because a source did: the reason goes on that source's report and the rest
// still run.
func Seed(ctx context.Context, sources []Source, terms []string, limit int) []Report {
	reports := make([]Report, 0, len(sources))
	for _, src := range sources {
		r := Report{Source: src.Name(), Status: StatusUsed, Findings: []Finding{}}
		findings, err := src.Search(ctx, terms, limit)
		switch {
		case errors.Is(err, ErrSkipped):
			r.Status = StatusSkipped
			r.Reason = reasonOf(err)
		case err != nil:
			r.Status = StatusUnavailable
			r.Reason = err.Error()
		case findings != nil:
			r.Findings = findings
		}
		reports = append(reports, r)
	}
	return reports
}

// reasonOf strips the ErrSkipped suffix from a skip error's message.
func reasonOf(err error) string {
	msg := err.Error()
	suffix := ": " + ErrSkipped.Error()
	if len(msg) > len(suffix) && msg[len(msg)-len(suffix):] == suffix {
		return msg[:len(msg)-len(suffix)]
	}
	return msg
}
