package research

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

type stubSource struct {
	name     string
	findings []Finding
	err      error
}

func (s stubSource) Name() string { return s.name }
func (s stubSource) Search(context.Context, []string, int) ([]Finding, error) {
	return s.findings, s.err
}

// A skipped or failing source is reported with its reason and never stops the
// sources after it: this is what lets a standalone run degrade to local
// knowledge with no error.
func TestSeed_DegradesPerSourceAndKeepsGoing(t *testing.T) {
	reports := Seed(context.Background(), []Source{
		stubSource{name: "hive", err: skipped("not running under Hive")},
		stubSource{name: "context7", err: errors.New("HTTP 503")},
		stubSource{name: "knowledge", findings: []Finding{{Title: "t", Cite: "knowledge:repo/a/x.md"}}},
	}, []string{"q"}, 5)

	require.Len(t, reports, 3)
	require.Equal(t, Report{Source: "hive", Status: StatusSkipped, Reason: "not running under Hive", Findings: []Finding{}}, reports[0])
	require.Equal(t, Report{Source: "context7", Status: StatusUnavailable, Reason: "HTTP 503", Findings: []Finding{}}, reports[1])
	require.Equal(t, StatusUsed, reports[2].Status)
	require.Equal(t, "knowledge:repo/a/x.md", reports[2].Findings[0].Cite)
}

const exportFixture = `# Agent Knowledge

This file is auto-generated from the hive knowledge base.

## Decisions

### Use NATS for fan-out

Queues fan out through NATS rather than Redis streams.

Tags: messaging, nats

### Unrelated decision

Nothing to do with the query.

## Gotchas

### Retries need jitter

Retrying without jitter stampedes the queue.
`

func writeExport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// Under Hive, relevant export entries come back ranked and cited by section
// and title; an entry matching nothing is not returned.
func TestHiveSource_CitesRelevantExportEntries(t *testing.T) {
	src := HiveSource{UnderHive: true, ExportPath: writeExport(t, exportFixture)}

	findings, err := src.Search(context.Background(), knowledge.Terms("nats queue"), 5)
	require.NoError(t, err)
	require.Equal(t, "hive:Decisions/Use NATS for fan-out", findings[0].Cite, "the tagged, fully covering entry ranks first")
	require.Equal(t, []string{"messaging", "nats"}, findings[0].Tags)
	require.Equal(t, src.ExportPath, findings[0].Read)

	findings, err = src.Search(context.Background(), knowledge.Terms("queue"), 5)
	require.NoError(t, err)
	var cites []string
	for _, f := range findings {
		cites = append(cites, f.Cite)
	}
	require.ElementsMatch(t, []string{"hive:Decisions/Use NATS for fan-out", "hive:Gotchas/Retries need jitter"}, cites)
}

// An entry that names its subject only in its title is found by it.
func TestHiveSource_TitleCountsAsEvidence(t *testing.T) {
	src := HiveSource{UnderHive: true, ExportPath: writeExport(t, "# Agent Knowledge\n\n## Patterns\n\n### Chi router\n\nSee the service template.\n")}

	findings, err := src.Search(context.Background(), knowledge.Terms("chi"), 5)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "hive:Patterns/Chi router", findings[0].Cite)
}

func TestHiveSource_Degrades(t *testing.T) {
	t.Run("standalone is skipped", func(t *testing.T) {
		_, err := HiveSource{UnderHive: false, ExportPath: writeExport(t, exportFixture)}.Search(context.Background(), []string{"nats"}, 5)
		require.ErrorIs(t, err, ErrSkipped)
	})
	t.Run("under Hive without an export is unavailable", func(t *testing.T) {
		_, err := HiveSource{UnderHive: true, ExportPath: filepath.Join(t.TempDir(), "agent.md")}.Search(context.Background(), []string{"nats"}, 5)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrSkipped)
		require.Contains(t, err.Error(), "absent")
	})
	t.Run("a file that is not an export is refused", func(t *testing.T) {
		_, err := HiveSource{UnderHive: true, ExportPath: writeExport(t, "<!DOCTYPE html>\n<html>")}.Search(context.Background(), []string{"nats"}, 5)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrSkipped)
	})
}

// ADRs in any conventional folder of any registered repo are found, cited by
// repo and repo-relative path, and ranked on the knowledge scale.
func TestADRSource_FindsAndCitesDecisionRecords(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(a, "docs", "adr"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(a, "docs", "adr", "0003-queue.md"), []byte("# Use NATS\n\nQueues use NATS.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(a, "docs", "adr", "notes.txt"), []byte("nats nats nats\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(b, "adr"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(b, "adr", "0001-http.md"), []byte("# HTTP\n\nUse chi.\n"), 0o644))

	src := ADRSource{Repos: []ADRRepo{{Name: "app", Root: a}, {Name: "web", Root: b}}}
	findings, err := src.Search(context.Background(), knowledge.Terms("nats"), 5)
	require.NoError(t, err)
	require.Len(t, findings, 1, "only markdown decision records are cited")
	require.Equal(t, "adr:app/docs/adr/0003-queue.md", findings[0].Cite)
	require.Equal(t, "Use NATS", findings[0].Title)
	require.Equal(t, filepath.Join(a, "docs", "adr", "0003-queue.md"), findings[0].Read)
	require.Greater(t, findings[0].Score, 0.0)
}

func TestADRSource_NoFolderIsSkipped(t *testing.T) {
	_, err := ADRSource{Repos: []ADRRepo{{Name: "app", Root: t.TempDir()}}}.Search(context.Background(), []string{"nats"}, 5)
	require.ErrorIs(t, err, ErrSkipped)
}

func newContext7(t *testing.T, handler http.HandlerFunc) Context7Source {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	src := NewContext7Source()
	src.Enabled = true
	src.APIKey = "k"
	src.BaseURL = srv.URL
	return src
}

// Context7 findings carry the library's documentation, fetched per library
// with the query as its topic, and never touch a library whose id could
// redirect the request elsewhere.
func TestContext7Source_SeedsFromLibraryDocs(t *testing.T) {
	var paths []string
	src := newContext7(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer k", r.Header.Get("Authorization"))
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2/libs/search":
			require.Equal(t, "cobra flags", r.URL.Query().Get("libraryName"))
			_, _ = w.Write([]byte(`{"results":[
				{"id":"@evil.example/x","title":"Evil"},
				{"id":"/spf13/cobra","title":"Cobra","description":"CLI library"}
			]}`))
		case "/spf13/cobra/llms.txt":
			require.Equal(t, "cobra flags", r.URL.Query().Get("topic"))
			_, _ = w.Write([]byte("### Mark flag as required\n\nUse MarkFlagRequired for required flags.\n"))
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
	})

	findings, err := src.Search(context.Background(), knowledge.Terms("cobra flags"), 5)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "context7:/spf13/cobra", findings[0].Cite)
	require.Equal(t, "Cobra", findings[0].Title)
	require.Contains(t, strings.Join(findings[0].Excerpts, "\n"), "MarkFlagRequired", "excerpts come from the docs, not the catalog")
	require.True(t, strings.HasPrefix(findings[0].Read, src.BaseURL+"/spf13/cobra/llms.txt?"))
	require.Equal(t, []string{"/api/v2/libs/search", "/spf13/cobra/llms.txt"}, paths)
}

func TestContext7Source_Degrades(t *testing.T) {
	t.Run("standalone without a key is skipped without a request", func(t *testing.T) {
		src := newContext7(t, func(http.ResponseWriter, *http.Request) { t.Fatal("no request expected") })
		src.Enabled, src.APIKey = false, ""
		_, err := src.Search(context.Background(), []string{"cobra"}, 5)
		require.ErrorIs(t, err, ErrSkipped)
	})
	t.Run("under Hive without a key queries anonymously", func(t *testing.T) {
		src := newContext7(t, func(w http.ResponseWriter, r *http.Request) {
			require.Empty(t, r.Header.Get("Authorization"))
			if r.URL.Path == "/api/v2/libs/search" {
				_, _ = w.Write([]byte(`{"results":[{"id":"/spf13/cobra"}]}`))
				return
			}
			_, _ = w.Write([]byte("cobra docs\n"))
		})
		src.APIKey = ""
		findings, err := src.Search(context.Background(), []string{"cobra"}, 5)
		require.NoError(t, err)
		require.Len(t, findings, 1)
	})
	t.Run("a redirect is not followed", func(t *testing.T) {
		src := newContext7(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest", http.StatusFound)
		})
		_, err := src.Search(context.Background(), []string{"cobra"}, 5)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrSkipped)
		require.Contains(t, err.Error(), "HTTP 302")
	})
	t.Run("docs failing for every library is unavailable", func(t *testing.T) {
		src := newContext7(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/libs/search" {
				_, _ = w.Write([]byte(`{"results":[{"id":"/spf13/cobra"}]}`))
				return
			}
			w.WriteHeader(http.StatusTooManyRequests)
		})
		_, err := src.Search(context.Background(), []string{"cobra"}, 5)
		require.Error(t, err)
		require.Contains(t, err.Error(), "HTTP 429")
	})
}

// A document described from memory reports exactly the evidence a file search
// reports for the same bytes, so in-memory sources rank on the same scale as
// stores.
func TestDescribeBytesMatchesFileSearch(t *testing.T) {
	dir := t.TempDir()
	content := "# Retry policy\n\nRetry with jitter.\nNever retry forever; retry budgets apply.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "retry.md"), []byte(content), 0o644))
	terms := knowledge.Terms("retry jitter")

	hits, err := store.NewFileStore(dir, "t").Search(terms, store.SearchOptions{})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	described, ok := store.DescribeBytes("retry.md", []byte(content), nil, terms)
	require.True(t, ok)
	require.Equal(t, hits[0], described)
}

// Failing docs count against the limit, so a long result list whose docs all
// fail costs at most limit doc requests rather than one per result.
func TestContext7Source_BoundsDocAttemptsByLimit(t *testing.T) {
	docRequests := 0
	src := newContext7(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/libs/search" {
			_, _ = w.Write([]byte(`{"results":[{"id":"/a/1"},{"id":"/a/2"},{"id":"/a/3"},{"id":"/a/4"},{"id":"/a/5"},{"id":"/a/6"}]}`))
			return
		}
		docRequests++
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	_, err := src.Search(context.Background(), []string{"x"}, 2)
	require.Error(t, err)
	require.Equal(t, 2, docRequests)
}
