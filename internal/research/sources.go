package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/store"
)

// KnowledgeSource searches the project's configured knowledge stores — every
// registered repo's store and the project's shared ones. It is the source a
// standalone run always has.
type KnowledgeSource struct {
	Set *knowledge.Set
	// Command is the CLI invocation prefix, used to tell the agent how to read
	// an entry in full.
	Command string
}

func (KnowledgeSource) Name() string { return "knowledge" }

func (k KnowledgeSource) Search(_ context.Context, terms []string, limit int) ([]Finding, error) {
	hits, err := k.Set.Search(strings.Join(terms, " "), knowledge.Selector{Tier: knowledge.TierAll})
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, min(len(hits), limit))
	for _, h := range hits {
		if len(findings) == limit {
			break
		}
		addr, _ := json.Marshal(map[string]string{"tier": h.Tier, "name": h.Name, "path": h.Path})
		findings = append(findings, Finding{
			Title:    h.Title,
			Cite:     fmt.Sprintf("knowledge:%s/%s/%s", h.Tier, h.Name, h.Path),
			Excerpts: h.Excerpts,
			Tags:     h.Tags,
			Score:    h.Score,
			Read:     fmt.Sprintf("%s knowledge read --data '%s'", k.Command, addr),
		})
	}
	return findings, nil
}

// ADRDirs are the repo-relative folders architecture decision records are
// conventionally kept in. Each registered repo root is checked for every one.
var ADRDirs = []string{
	"adr",
	"adrs",
	"doc/adr",
	"docs/adr",
	"docs/adrs",
	"docs/decisions",
	"docs/architecture/decisions",
	"architecture/decisions",
}

// ADRRepo is one registered repo whose ADR folders are searched.
type ADRRepo struct {
	Name string
	Root string
}

// ADRSource searches the ADR folders inside registered repos. It ranks on the
// knowledge formula, so an ADR and a knowledge entry about the same subject
// score on one scale.
type ADRSource struct {
	Repos []ADRRepo
}

func (ADRSource) Name() string { return "adr" }

func (a ADRSource) Search(_ context.Context, terms []string, limit int) ([]Finding, error) {
	type scored struct {
		hit  store.Hit
		repo string
		dir  string
		root string
	}
	var all []scored
	searched := 0
	for _, r := range a.Repos {
		for _, dir := range ADRDirs {
			abs := filepath.Join(r.Root, dir)
			info, err := os.Stat(abs)
			if err != nil || !info.IsDir() {
				continue
			}
			searched++
			hits, err := store.NewSourceStore(abs, "adr:"+r.Name).Search(terms, store.SearchOptions{})
			if err != nil {
				return nil, fmt.Errorf("searching %s: %w", abs, err)
			}
			for _, h := range hits {
				if !strings.EqualFold(filepath.Ext(h.Path), ".md") {
					continue
				}
				h.Score = knowledge.ScoreHit(terms, h)
				if h.Score == 0 {
					continue
				}
				all = append(all, scored{hit: h, repo: r.Name, dir: dir, root: r.Root})
			}
		}
	}
	if searched == 0 {
		return nil, skipped("no registered repo has an ADR folder (" + strings.Join(ADRDirs, ", ") + ")")
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].hit.Score > all[j].hit.Score })
	findings := []Finding{}
	for _, s := range all {
		if len(findings) == limit || s.hit.Score < knowledge.CutoffFloor(all[0].hit.Score) {
			break
		}
		rel := filepath.ToSlash(filepath.Join(s.dir, s.hit.Path))
		findings = append(findings, Finding{
			Title:    s.hit.Title,
			Cite:     fmt.Sprintf("adr:%s/%s", s.repo, rel),
			Excerpts: s.hit.Excerpts,
			Tags:     s.hit.Tags,
			Score:    s.hit.Score,
			Read:     filepath.Join(s.root, s.dir, s.hit.Path),
		})
	}
	return findings, nil
}

// HiveExportHeading opens every Hive knowledge export. A file that does not
// start with it is not an export: the Hive launcher applies the same check
// before it writes one.
const HiveExportHeading = "# Agent Knowledge"

// HiveSource searches the Hive knowledge export the Hive contributor launcher
// writes to ~/agent.md and refreshes in the background. Spektacular only reads
// that file: it never fetches the export itself, so it never handles the
// launcher's credentials.
type HiveSource struct {
	// UnderHive reports whether this process runs under Hive. The launcher
	// sets HIVE_HUB for every contributor session.
	UnderHive bool
	// ExportPath is where the launcher writes the export.
	ExportPath string
}

// NewHiveSource describes the Hive environment from the process environment.
func NewHiveSource() HiveSource {
	home, _ := os.UserHomeDir()
	return HiveSource{
		UnderHive:  os.Getenv("HIVE_HUB") != "",
		ExportPath: filepath.Join(home, "agent.md"),
	}
}

func (HiveSource) Name() string { return "hive" }

func (h HiveSource) Search(_ context.Context, terms []string, limit int) ([]Finding, error) {
	if !h.UnderHive {
		return nil, skipped("not running under Hive (HIVE_HUB is unset)")
	}
	content, err := os.ReadFile(h.ExportPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the Hive knowledge export %s is absent; the Hive launcher writes it when the hub serves one", h.ExportPath)
	}
	if err != nil {
		return nil, fmt.Errorf("reading the Hive knowledge export: %w", err)
	}
	if !strings.HasPrefix(string(content), HiveExportHeading) {
		return nil, fmt.Errorf("%s is not a Hive knowledge export (it does not start with %q)", h.ExportPath, HiveExportHeading)
	}

	type scored struct {
		entry hiveEntry
		hit   store.Hit
	}
	var all []scored
	for _, e := range parseHiveExport(string(content)) {
		// The entry is described with its title as its heading, exactly as a
		// knowledge file carries one, so a fact that states its subject only in
		// its title is still found by it.
		hit, ok := store.DescribeBytes(e.title, []byte("### "+e.title+"\n"+e.body), e.tags, terms)
		if !ok {
			continue
		}
		if hit.Score = knowledge.ScoreHit(terms, hit); hit.Score > 0 {
			all = append(all, scored{entry: e, hit: hit})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].hit.Score > all[j].hit.Score })
	findings := []Finding{}
	for _, s := range all {
		if len(findings) == limit || s.hit.Score < knowledge.CutoffFloor(all[0].hit.Score) {
			break
		}
		findings = append(findings, Finding{
			Title:    s.entry.title,
			Cite:     fmt.Sprintf("hive:%s/%s", s.entry.section, s.entry.title),
			Excerpts: s.hit.Excerpts,
			Tags:     s.hit.Tags,
			Score:    s.hit.Score,
			Read:     h.ExportPath,
		})
	}
	return findings, nil
}

// hiveEntry is one fact in a Hive knowledge export.
type hiveEntry struct {
	section string
	title   string
	body    string
	tags    []string
}

// parseHiveExport splits an export into its facts. The export groups facts
// under "## <type>" sections, gives each a "### <title>" heading, and ends a
// tagged fact with a "Tags: a, b" line. Anything before the first section is
// preamble and holds no fact.
func parseHiveExport(content string) []hiveEntry {
	var entries []hiveEntry
	var section string
	var cur *hiveEntry
	var body []string
	flush := func() {
		if cur != nil {
			cur.body = strings.TrimSpace(strings.Join(body, "\n"))
			entries = append(entries, *cur)
		}
		cur, body = nil, nil
	}
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "### "):
			flush()
			if section != "" {
				cur = &hiveEntry{section: section, title: strings.TrimSpace(line[4:])}
			}
		case strings.HasPrefix(line, "## "):
			flush()
			section = strings.TrimSpace(line[3:])
		case cur != nil && strings.HasPrefix(line, "Tags: "):
			for _, t := range strings.Split(line[len("Tags: "):], ",") {
				if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
					cur.tags = append(cur.tags, t)
				}
			}
		case cur != nil:
			body = append(body, line)
		}
	}
	flush()
	return entries
}

// Context7BaseURL is the only host the Context7 source talks to. The API key,
// when set, is sent nowhere else: redirects are refused.
const Context7BaseURL = "https://context7.com"

// context7Timeout bounds one Context7 request so a slow service cannot stall
// the research step.
const context7Timeout = 15 * time.Second

// Context7Source resolves the query to Context7 library documentation. It
// runs under Hive, whose research seeding Context7 is part of, and wherever
// CONTEXT7_API_KEY — the variable Hive's own Context7 integration reads — is
// set. The key is optional, as it is in Hive: it raises Context7's rate limit
// but is not needed to query it. A standalone run without a key makes no
// network call unasked.
type Context7Source struct {
	// Enabled reports whether this environment asked for Context7.
	Enabled bool
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewContext7Source describes Context7 from the process environment.
func NewContext7Source() Context7Source {
	key := os.Getenv("CONTEXT7_API_KEY")
	return Context7Source{
		Enabled: key != "" || os.Getenv("HIVE_HUB") != "",
		APIKey:  key,
		BaseURL: Context7BaseURL,
		Client: &http.Client{
			Timeout: context7Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (Context7Source) Name() string { return "context7" }

type context7Library struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (c Context7Source) Search(ctx context.Context, terms []string, limit int) ([]Finding, error) {
	if !c.Enabled {
		return nil, skipped("not running under Hive and CONTEXT7_API_KEY is unset")
	}
	query := strings.Join(terms, " ")
	raw, err := c.get(ctx, "/api/v2/libs/search?"+url.Values{"libraryName": {query}, "query": {query}}.Encode(), 1<<20)
	if err != nil {
		return nil, fmt.Errorf("context7 search: %w", err)
	}
	var body struct {
		Results []context7Library `json:"results"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("context7 search: decoding response: %w", err)
	}
	findings := []Finding{}
	var lastErr error
	// Docs are fetched for at most limit libraries, successful or not, so a
	// long result list whose docs fail cannot turn one research call into a
	// request per result.
	attempts := 0
	for _, lib := range body.Results {
		if attempts == limit {
			break
		}
		if !validContext7ID(lib.ID) {
			continue
		}
		attempts++
		docsPath := lib.ID + "/llms.txt?" + url.Values{"topic": {query}, "tokens": {context7DocTokens}}.Encode()
		docs, err := c.get(ctx, docsPath, context7MaxDocBytes)
		if err != nil {
			lastErr = fmt.Errorf("context7 docs for %s: %w", lib.ID, err)
			continue
		}
		// The documentation itself is the finding: its strongest matching
		// lines are the excerpts, ranked by the same line rule a knowledge hit
		// uses. A library whose docs match no term is still returned by
		// Context7's own relevance, with its opening description instead.
		excerpts := []string{}
		if hit, ok := store.DescribeBytes(lib.ID, docs, nil, terms); ok {
			excerpts = hit.Excerpts
		}
		if len(excerpts) == 0 && lib.Description != "" {
			excerpts = append(excerpts, lib.Description)
		}
		title := lib.Title
		if title == "" {
			title = lib.ID
		}
		findings = append(findings, Finding{
			Title:    title,
			Cite:     "context7:" + lib.ID,
			Excerpts: excerpts,
			Tags:     []string{},
			Read:     c.BaseURL + docsPath,
		})
	}
	if len(findings) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return findings, nil
}

// validContext7ID reports whether id is safe to splice into a docs path on the
// fixed Context7 host: a single leading "/" (so it cannot supply its own
// authority) and only the characters Context7 library ids use (so it cannot
// smuggle in a query, fragment or traversal).
func validContext7ID(id string) bool {
	if !strings.HasPrefix(id, "/") || strings.HasPrefix(id, "//") || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-", r)) {
			return false
		}
	}
	return true
}

// context7DocTokens bounds how much documentation Context7 returns per
// library, and context7MaxDocBytes caps what is read of it regardless.
const (
	context7DocTokens   = "2000"
	context7MaxDocBytes = 256 << 10
)

// get fetches path from the Context7 base URL, reading at most max bytes. The
// API key, when there is one, goes only to that base URL. Redirects are
// refused by the client, so a non-200 answer is reported rather than followed.
func (c Context7Source) get(ctx context.Context, path string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, max))
}
