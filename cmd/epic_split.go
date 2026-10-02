package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/steps/spec"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// epic split turns one complete spec into an epic of complete specs in a
// single operation. The agent and the user have already agreed every resulting
// spec and where each piece of content goes; the CLI's job is everything that
// must not stop half-way: allocating the new specs' IDs, writing each of them
// complete and final, rewriting the split spec as its narrowed self, writing
// or extending the epic and linking every spec to it. A failure at any point
// restores every document it touched.

var epicSplitCmd = &cobra.Command{
	Use:   "split",
	Short: "Split a complete spec into an epic of complete specs, from one staged JSON description",
	Args:  cobra.NoArgs,
	RunE:  runEpicSplit,
}

// splitBody is every section of one resulting spec. Each list is rendered as
// that section's items; Overview is prose.
type splitBody struct {
	Overview           string   `json:"overview"`
	Requirements       []string `json:"requirements"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Constraints        []string `json:"constraints"`
	TechnicalApproach  []string `json:"technical_approach"`
	SuccessMetrics     []string `json:"success_metrics"`
	NonGoals           []string `json:"non_goals"`
}

// splitSpec is one resulting spec. The spec being split is named by name; a
// new spec is named by title, and its depends_on may name other new specs by
// title, which the CLI rewrites to their allocated names.
type splitSpec struct {
	Name      string        `json:"name"`
	Title     string        `json:"title"`
	ID        string        `json:"id"`
	DependsOn []string      `json:"depends_on"`
	Scope     string        `json:"scope"`
	Sources   []sourceInput `json:"sources"`
	Body      splitBody     `json:"body"`
}

// splitInput is the staged split description.
type splitInput struct {
	Spec     string        `json:"spec"`
	Overview string        `json:"overview"`
	Sources  []sourceInput `json:"sources"`
	Specs    []splitSpec   `json:"specs"`
	// ConfirmCompletedEpic is the user's go-ahead to split a spec in an epic
	// whose specs are all implemented, adding new specs to it.
	ConfirmCompletedEpic bool `json:"confirm_completed_epic"`
}

var splitBodySchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"overview":            {Type: "string"},
	"requirements":        {Type: "array", Items: &schemaProp{Type: "string"}},
	"acceptance_criteria": {Type: "array", Items: &schemaProp{Type: "string"}},
	"constraints":         {Type: "array", Items: &schemaProp{Type: "string"}},
	"technical_approach":  {Type: "array", Items: &schemaProp{Type: "string"}},
	"success_metrics":     {Type: "array", Items: &schemaProp{Type: "string"}},
	"non_goals":           {Type: "array", Items: &schemaProp{Type: "string"}},
}}

var epicSplitInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec":                   {Type: "string", Description: "the complete spec being split; it becomes the epic's first spec"},
		"overview":               {Type: "string", Description: "the epic's overview; required when the split creates the epic"},
		"sources":                {Type: "array", Items: sourceItemSchema, Description: "provenance moving from the split spec to the epic"},
		"confirm_completed_epic": {Type: "boolean", Description: "true only after the user agrees to add specs to an epic whose specs are all implemented"},
		"specs": {Type: "array", Description: "every resulting spec: the split spec by name, each new spec by title", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"name":       {Type: "string", Description: "the spec being split (only it is named)"},
			"title":      {Type: "string", Description: "a new spec's name before its ID is allocated"},
			"id":         {Type: "string", Description: "a new spec's ID, only when spec.id_method is external"},
			"depends_on": {Type: "array", Items: &schemaProp{Type: "string"}, Description: "names or titles of specs in the epic it depends on"},
			"scope":      {Type: "string", Description: "one-line scope for the epic's specs table; defaults to the overview's first sentence"},
			"sources":    {Type: "array", Items: sourceItemSchema},
			"body":       splitBodySchema,
		}}},
	},
	Required: []string{"spec", "specs"},
}

var epicSplitOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"epic":    {Type: "string"},
		"path":    {Type: "string"},
		"created": {Type: "array", Items: &schemaProp{Type: "string"}},
		"linked":  {Type: "array", Items: &schemaProp{Type: "string"}},
	},
}

func splitInvalid(cfg config.Config, message string) error {
	return output.NewError("epic_split_invalid", message).
		WithNextAction(fmt.Sprintf("fix the staged split description and reissue `%s epic split --from <that file>`; run `%s epic split --schema` for its shape", cfg.Command, cfg.Command))
}

// validateSplit checks the description before anything is read or written
// beyond the split spec itself.
func validateSplit(cfg config.Config, in splitInput) error {
	if in.Spec == "" {
		return splitInvalid(cfg, `the description has no "spec": name the complete spec being split`)
	}
	if len(in.Specs) < 2 {
		return splitInvalid(cfg, fmt.Sprintf("a split needs at least two resulting specs; the description has %d", len(in.Specs)))
	}
	narrowed := 0
	titles := map[string]bool{}
	for i, s := range in.Specs {
		label := s.Title
		switch {
		case s.Name != "" && s.Title != "":
			return splitInvalid(cfg, fmt.Sprintf("specs[%d] has both a name and a title; name only the spec being split, and give each new spec a title", i))
		case s.Name != "":
			if s.Name != in.Spec {
				return splitInvalid(cfg, fmt.Sprintf("specs[%d] names %q; only the spec being split (%q) is named, every new spec is given a title", i, s.Name, in.Spec))
			}
			narrowed++
			label = s.Name
		case s.Title == "":
			return splitInvalid(cfg, fmt.Sprintf("specs[%d] has no title; every new spec needs one", i))
		default:
			if titles[s.Title] {
				return splitInvalid(cfg, fmt.Sprintf("title %q is used by more than one new spec", s.Title))
			}
			titles[s.Title] = true
		}
		if strings.TrimSpace(s.Body.Overview) == "" {
			return splitInvalid(cfg, fmt.Sprintf("spec %q has an empty overview; every resulting spec needs its own", label))
		}
		if len(nonBlank(s.Body.AcceptanceCriteria)) == 0 {
			return splitInvalid(cfg, fmt.Sprintf("spec %q has no acceptance criteria; every resulting spec needs at least one it can be verified by on its own", label))
		}
	}
	if narrowed != 1 {
		return splitInvalid(cfg, fmt.Sprintf("the spec being split (%q) must appear exactly once in specs, by name, as its narrowed self; it appears %d times", in.Spec, narrowed))
	}
	return nil
}

func nonBlank(items []string) []string {
	var out []string
	for _, s := range items {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func runEpicSplit(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  epicSplitInputSchema,
			Output: epicSplitOutputSchema,
			Flags:  map[string]*schemaProp{"from": {Type: "string"}},
		}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	fromPath, _ := cmd.Flags().GetString("from")
	if fromPath == "" {
		return output.NewError("epic_from_required",
			"--from is required: a split description is read from a staged JSON file").
			WithNextAction(fmt.Sprintf("stage the description under .spektacular/tmp/ and reissue `%s epic split --from <that file>`", cfg.Command))
	}
	raw, err := os.ReadFile(fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	var in splitInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return splitInvalid(cfg, fmt.Sprintf("the description is not valid JSON: %v", err))
	}
	if err := validateSplit(cfg, in); err != nil {
		return err
	}
	today := time.Now().UTC()

	split, err := readSpecFile(st, cfg, in.Spec)
	if errors.Is(err, store.ErrNotFound) {
		return output.NewError("epic_split_invalid", fmt.Sprintf("no spec named %q is stored in this project", in.Spec)).
			WithResource(in.Spec).
			WithNextAction(fmt.Sprintf("run `%s spec file list` to see the stored specs, and name the complete spec being split", cfg.Command))
	}
	if err != nil {
		return err
	}

	// The target epic: the one the split spec already belongs to, or a new
	// epic that takes the split spec's name (no renaming, design Decision 2).
	epicName := split.epic()
	if epicName == "" {
		epicName = in.Spec
	}
	var existing *epic.Epic
	current, _, err := readEpic(cfg, st, epicName)
	switch {
	case err == nil:
		existing = &current
	case isCode(err, "epic_not_found"):
		if strings.TrimSpace(in.Overview) == "" {
			return splitInvalid(cfg, `the description has no "overview"; a split that creates the epic needs the overall overview, which moves from the split spec to the epic`)
		}
	default:
		return err
	}

	if existing != nil {
		retry := fmt.Sprintf("the same `%s epic split` with \"confirm_completed_epic\": true added to the staged description", cfg.Command)
		if err := refuseCompletedEpic(cfg, st, epicName, in.ConfirmCompletedEpic, retry); err != nil {
			return err
		}
	}

	epicSources, err := stampSources(cfg, in.Sources, today)
	if err != nil {
		return err
	}

	// Validate the graph before anything is written, with new specs named by
	// their titles.
	graph := splitGraph(existing, in, nil)
	if err := epic.Validate(graph); err != nil {
		return err
	}

	// Write every new spec, one at a time so a counter ID sees the previous
	// one, then the narrowed spec. Everything is inside one transaction.
	t := newDocTxn(st)
	names := map[string]string{} // title -> allocated name
	var created []string
	for _, s := range in.Specs {
		if s.Title == "" {
			continue
		}
		resolved, err := spec.ResolveIdentifier(spec.IdentifierRequest{
			Name:     s.Title,
			ID:       s.ID,
			Method:   cfg.Spec.IDMethod,
			SpecDir:  cfg.Spec.Config.Directory,
			Store:    st,
			Now:      specIdentifierNow,
			RandomID: specIdentifierRandomID,
		})
		if err != nil {
			return t.abort(splitInvalid(cfg, fmt.Sprintf("cannot name new spec %q: %v", s.Title, err)))
		}
		sources, err := stampSources(cfg, s.Sources, today)
		if err != nil {
			return t.abort(err)
		}
		body, err := renderSplitSpec(resolved.Name, s.Body)
		if err != nil {
			return t.fail(err)
		}
		final := metadata.StatusFinal
		merged, err := metadata.Merge(nil, body, metadata.UpdateOptions{DocumentStatus: &final, Sources: &sources, Today: today})
		if err != nil {
			return t.fail(err)
		}
		if err := t.write(spec.SpecFilePath(cfg.Spec.Config.Directory, resolved.Name), merged); err != nil {
			return t.fail(err)
		}
		names[s.Title] = resolved.Name
		created = append(created, resolved.Name)
	}

	for _, s := range in.Specs {
		if s.Name == "" {
			continue
		}
		body, err := renderSplitSpec(in.Spec, s.Body)
		if err != nil {
			return t.fail(err)
		}
		opts := metadata.UpdateOptions{Today: today}
		if sources := narrowedSources(split, s, epicSources, today); sources != nil {
			opts.Sources = sources
		}
		merged, err := metadata.Merge(split.raw, body, opts)
		if err != nil {
			return t.fail(err)
		}
		if err := t.write(split.path, merged); err != nil {
			return t.fail(err)
		}
	}

	next := epic.Epic{Spec: in.Spec, Sources: epicSources}
	overview := in.Overview
	if existing != nil {
		next.Spec = existing.Spec
		next.Sources = mergeSources(existing.Sources, epicSources)
		if strings.TrimSpace(overview) == "" {
			overview = epicOverview(existing.Body)
		}
	}
	next.Specs = splitGraph(existing, in, names)
	if err := epic.Validate(next.Specs); err != nil {
		return t.abort(err)
	}
	next.Body = renderEpicBody(epicName, overview, next.Specs, splitScopes(existing, in, names))
	if next, err = epic.Stamp(existing, next, nil, today); err != nil {
		return t.fail(err)
	}

	links := epicLinks{epic: epicName, newList: next.SpecNames()}
	if existing != nil {
		links.oldList = existing.SpecNames()
	}
	if err := checkEpicLinks(st, cfg, links); err != nil {
		return t.abort(err)
	}
	rendered, err := next.Render()
	if err != nil {
		return t.fail(err)
	}
	linked, _, err := applyEpicLinks(t, cfg, links, rendered)
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"epic":    epicName,
		"path":    reportedLocation(centralLocationBase, epicPath(cfg, epicName)),
		"created": nonNil(created),
		"linked":  nonNil(linked),
	})
}

// splitGraph builds the epic's specs list: the existing epic's entries in
// their order (the split spec's entry replaced by the description's), then
// each new spec. names maps titles to allocated names; nil leaves titles in
// place, for validating before allocation.
func splitGraph(existing *epic.Epic, in splitInput, names map[string]string) []epic.EpicSpec {
	rename := func(n string) string {
		if allocated, ok := names[n]; ok {
			return allocated
		}
		return n
	}
	entry := func(s splitSpec) epic.EpicSpec {
		name := s.Name
		if name == "" {
			name = rename(s.Title)
		}
		deps := make([]string, 0, len(s.DependsOn))
		for _, d := range s.DependsOn {
			deps = append(deps, rename(d))
		}
		return epic.EpicSpec{Name: name, DependsOn: deps}
	}
	var out []epic.EpicSpec
	placed := false
	if existing != nil {
		for _, e := range existing.Specs {
			if e.Name == in.Spec {
				for _, s := range in.Specs {
					if s.Name != "" {
						out = append(out, entry(s))
					}
				}
				placed = true
				continue
			}
			out = append(out, e)
		}
	}
	for _, s := range in.Specs {
		if s.Name != "" && !placed {
			out = append(out, entry(s))
		}
	}
	for _, s := range in.Specs {
		if s.Name == "" {
			out = append(out, entry(s))
		}
	}
	return out
}

// splitScopes gives each spec's one-line scope for the epic's specs table: a
// spec in the description uses its scope, or its overview's first sentence;
// an existing member keeps the scope its row already had.
func splitScopes(existing *epic.Epic, in splitInput, names map[string]string) map[string]string {
	scopes := map[string]string{}
	if existing != nil {
		for name, scope := range epicTableScopes(existing.Body) {
			scopes[name] = scope
		}
	}
	for _, s := range in.Specs {
		name := s.Name
		if name == "" {
			name = names[s.Title]
		}
		scope := strings.TrimSpace(s.Scope)
		if scope == "" {
			scope = firstSentence(s.Body.Overview)
		}
		scopes[name] = scope
	}
	return scopes
}

// narrowedSources decides the split spec's own sources. An explicit list on
// its entry wins. Otherwise sources moving to the epic leave the spec, and any
// other source it had (one only about the narrowed part) stays. nil means no
// change.
func narrowedSources(split specFile, s splitSpec, toEpic []metadata.SourceRef, today time.Time) *[]metadata.SourceRef {
	if s.Sources != nil {
		out := make([]metadata.SourceRef, 0, len(s.Sources))
		for _, src := range s.Sources {
			out = append(out, metadata.SourceRef{URI: src.URI, RetrievedDate: src.RetrievedDate})
		}
		for i := range out {
			if out[i].RetrievedDate == "" {
				out[i].RetrievedDate = today.Format(metadata.DateFormat)
			}
		}
		return &out
	}
	if split.fm == nil || len(toEpic) == 0 {
		return nil
	}
	moved := map[string]bool{}
	for _, src := range toEpic {
		moved[src.URI] = true
	}
	kept := []metadata.SourceRef{}
	for _, src := range split.fm.Sources {
		if !moved[src.URI] {
			kept = append(kept, src)
		}
	}
	return &kept
}

// mergeSources appends the sources in add that existing does not already
// record, by uri.
func mergeSources(existing, add []metadata.SourceRef) []metadata.SourceRef {
	out := append([]metadata.SourceRef{}, existing...)
	seen := map[string]bool{}
	for _, s := range existing {
		seen[s.URI] = true
	}
	for _, s := range add {
		if !seen[s.URI] {
			out = append(out, s)
			seen[s.URI] = true
		}
	}
	return out
}

// specSections is the order and content of a spec's sections, matching the
// spec scaffold. checkbox marks the sections rendered as `- [ ] **…**` items.
func specSections(b splitBody) []struct {
	heading  string
	items    []string
	prose    string
	checkbox bool
} {
	return []struct {
		heading  string
		items    []string
		prose    string
		checkbox bool
	}{
		{heading: "## Overview", prose: b.Overview},
		{heading: "## Requirements", items: b.Requirements, checkbox: true},
		{heading: "## Constraints", items: b.Constraints},
		{heading: "## Acceptance Criteria", items: b.AcceptanceCriteria, checkbox: true},
		{heading: "## Technical Approach", items: b.TechnicalApproach},
		{heading: "## Success Metrics", items: b.SuccessMetrics},
		{heading: "## Non-Goals", items: b.NonGoals},
	}
}

// renderSplitSpec renders a complete spec body from the spec scaffold, keeping
// its guidance comments and filling every section. A requirement or criterion
// whose text runs over several lines keeps its first line as the bold title
// and indents the rest as detail, the format the scaffold asks for.
func renderSplitSpec(name string, b splitBody) ([]byte, error) {
	scaffold, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": name})
	if err != nil {
		return nil, err
	}
	content := map[string]string{}
	for _, s := range specSections(b) {
		content[s.heading] = renderSection(s.prose, s.items, s.checkbox)
	}
	var out strings.Builder
	lines := strings.Split(scaffold, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		body, isSection := content[line]
		if !isSection {
			out.WriteString(line)
			if i < len(lines)-1 {
				out.WriteString("\n")
			}
			continue
		}
		out.WriteString(line + "\n\n")
		if body != "" {
			out.WriteString(body + "\n\n")
		}
		// Skip the scaffold's empty placeholder lines up to the next
		// section's guidance comment.
		for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
			i++
		}
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), nil
}

func renderSection(prose string, items []string, checkbox bool) string {
	if prose != "" {
		return strings.TrimSpace(prose)
	}
	var lines []string
	for _, item := range nonBlank(items) {
		item = strings.TrimSpace(item)
		if !checkbox {
			lines = append(lines, "- "+strings.ReplaceAll(item, "\n", "\n  "))
			continue
		}
		title, detail, _ := strings.Cut(item, "\n")
		title = strings.Trim(strings.TrimSpace(title), "*")
		entry := "- [ ] **" + title + "**"
		for _, d := range strings.Split(detail, "\n") {
			if d = strings.TrimSpace(d); d != "" {
				entry += "\n  " + d
			}
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

// renderEpicBody writes the epic's two sections: its overview, and a table of
// its specs with each one's scope.
func renderEpicBody(name, overview string, specs []epic.EpicSpec, scopes map[string]string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Epic: %s\n\n## Overview\n\n%s\n\n## Specs\n\n| # | Spec | Scope |\n|---|---|---|\n", name, strings.TrimSpace(overview))
	for i, s := range specs {
		fmt.Fprintf(&b, "| %d | %s | %s |\n", i+1, s.Name, strings.ReplaceAll(scopes[s.Name], "|", `\|`))
	}
	return []byte(b.String())
}

// epicOverview extracts the text of an epic body's "## Overview" section.
func epicOverview(body []byte) string {
	_, after, ok := strings.Cut(string(body), "## Overview")
	if !ok {
		return ""
	}
	if i := strings.Index(after, "\n## "); i >= 0 {
		after = after[:i]
	}
	return strings.TrimSpace(after)
}

// epicTableScopes reads the scope column of an epic body's specs table.
func epicTableScopes(body []byte) map[string]string {
	scopes := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		cells := strings.Split(strings.TrimSpace(line), "|")
		if len(cells) < 5 {
			continue
		}
		name := strings.TrimSpace(cells[2])
		if name == "" || name == "Spec" || strings.HasPrefix(name, "-") {
			continue
		}
		scopes[name] = strings.TrimSpace(cells[3])
	}
	return scopes
}

// firstSentence returns text up to and including its first full stop.
func firstSentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if i := strings.Index(text, ". "); i >= 0 {
		return text[:i+1]
	}
	return text
}

func init() {
	epicSplitCmd.Flags().String("from", "", "Read the split description (JSON) from the file at <path> (relative to cwd)")
	epicCmd.AddCommand(epicSplitCmd)
}
