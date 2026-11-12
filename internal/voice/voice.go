// Package voice turns a spoken note — a recording on a voice recorder, a
// dictation, a transcript file — into the material a spec starts from. Each
// source fetches one Note; Draft maps the note onto the spec scaffold.
package voice

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// Note is one spoken note as every source returns it. Summary is Markdown
// when the recorder produced an AI summary (a Plaud note); Transcript is the
// spoken text, timestamped or not. Either may be empty, never both.
type Note struct {
	Title      string
	Summary    string
	Transcript string
	// URI is the note's stable address, recorded as the spec's source:
	// plaud://recordings/<id>, wisprflow://history/<id>, file://<path>.
	URI        string
	RecordedAt time.Time
}

// Query selects one note from a source. Latest picks the most recent; Title
// a case-insensitive match on the recording's name; Path a file or folder
// for sources that read disk.
type Query struct {
	Latest bool
	Title  string
	Path   string
}

// Source is one voice interface. Fetch returns the single note the query
// selects, or an error naming what was and was not found.
type Source interface {
	Name() string
	Fetch(ctx context.Context, q Query) (Note, error)
}

// Options configures the sources that need more than the query.
type Options struct {
	// Command is the MCP server command line for MCP-backed sources; empty
	// uses the source's default.
	Command string
	// Stdin is what the stdin source reads.
	Stdin io.Reader
	// Run executes a command line and returns its stdout; nil uses exec.
	// Tests substitute it.
	Run Runner
	// Dial opens an MCP session for MCP-backed sources; nil starts Command
	// as a child process. Tests substitute it.
	Dial Dialer
}

// Runner executes argv and returns stdout.
type Runner func(ctx context.Context, argv ...string) ([]byte, error)

// NotFoundError says the query matched nothing, listing what was there.
type NotFoundError struct {
	Source    string
	Query     Query
	Available []string
}

func (e *NotFoundError) Error() string {
	what := "the latest note"
	if e.Query.Title != "" {
		what = fmt.Sprintf("a note titled %q", e.Query.Title)
	}
	if len(e.Available) == 0 {
		return fmt.Sprintf("%s: no %s found: the source has no notes", e.Source, what)
	}
	return fmt.Sprintf("%s: no %s found; recent notes: %s", e.Source, what, strings.Join(e.Available, "; "))
}

// SourceNames lists the sources Open accepts.
var SourceNames = []string{"file", "plaud", "stdin", "wisprflow"}

// Open returns the named source.
func Open(name string, opts Options) (Source, error) {
	switch name {
	case "plaud":
		return newPlaud(opts), nil
	case "wisprflow":
		return newWisprFlow(opts), nil
	case "file":
		return fileSource{}, nil
	case "stdin":
		return stdinSource{r: opts.Stdin}, nil
	}
	return nil, fmt.Errorf("unknown voice source %q; sources: %s", name, strings.Join(SourceNames, ", "))
}

// pick chooses one entry by query from a list of candidates. titleOf and
// timeOf read each candidate; the result is its index.
func pick(source string, q Query, n int, titleOf func(int) string, timeOf func(int) time.Time) (int, error) {
	if n == 0 {
		return -1, &NotFoundError{Source: source, Query: q}
	}
	best := -1
	want := strings.ToLower(strings.TrimSpace(q.Title))
	for i := 0; i < n; i++ {
		if want != "" && !strings.Contains(strings.ToLower(titleOf(i)), want) {
			continue
		}
		if best < 0 || timeOf(i).After(timeOf(best)) {
			best = i
		}
	}
	if best < 0 {
		avail := make([]string, 0, n)
		for i := 0; i < n && i < 10; i++ {
			avail = append(avail, titleOf(i))
		}
		return -1, &NotFoundError{Source: source, Query: q, Available: avail}
	}
	return best, nil
}
