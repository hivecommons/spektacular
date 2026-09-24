package knowledge

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hivecommons/spektacular/internal/store"
)

// CategoryTier declares how a category's entries are retrieved. A category's
// retrieval tier is stated once, here in the registry, and every behaviour that
// depends on it — project scaffolding, search exclusion, and the always-applied
// reader — reads it from here rather than restating a category name. Re-tiering
// a category is therefore a single-field change.
//
// This is a different axis from the addressing Tier in address.go, which says
// which knowledge a store holds rather than when its entries are loaded. The
// two never appear on the same object, so Category keeps its "tier" JSON key
// and the unqualified Go name goes to the addressing concept.
type CategoryTier string

const (
	// CategoryTierAlwaysApplied marks a category whose entries are loaded in
	// full on every task and are deliberately excluded from search, so the same
	// content is never surfaced twice. Keep these categories small: their whole
	// content is paid for on every task.
	CategoryTierAlwaysApplied CategoryTier = "always-applied"
	// CategoryTierLookedUp marks a category whose entries are retrieved only
	// when a query matches them. This is the larger reference body of the
	// knowledge base.
	CategoryTierLookedUp CategoryTier = "looked-up"
)

// Category is the registry record describing one knowledge category. It is the
// category's contract with both contributors and the assistant: what the
// category is for, what looks similar but belongs elsewhere, how its entries
// are retrieved, and the shape an entry should take. The registry is the single
// source of truth for the category model — project init scaffolds directories
// and READMEs from it, the knowledge Set derives tier behaviour from it, and the
// `knowledge categories` command projects it to JSON for the contribution flow.
type Category struct {
	// Name is the directory name and path prefix for the category, e.g. "glossary".
	Name string `json:"name"`
	// Purpose states what the category is for.
	Purpose string `json:"purpose"`
	// Boundary states what looks similar but belongs in another category, so a
	// contributor can tell categories apart at the edges.
	Boundary string `json:"boundary"`
	// Tier declares how the category's entries are retrieved.
	Tier CategoryTier `json:"tier"`
	// EntryShape describes the shape an entry should take, e.g. "a term and a
	// short gloss".
	EntryShape string `json:"entryShape"`
}

// Categories is the canonical, ordered list of knowledge categories. It is the
// single declaration of the category model: adding, removing, or re-tiering a
// category is a change to this list alone.
var Categories = []Category{
	{
		Name:       "conventions",
		Purpose:    "The rules a team always wants honoured — coding standards, naming schemes, formatting, required patterns, and the house style that every change must follow.",
		Boundary:   "Not the reasoning behind a rule (that is a decision) and not a one-off lesson learned in passing (that is a learning). A convention is a standing rule, stated as an instruction to follow.",
		Tier:       CategoryTierAlwaysApplied,
		EntryShape: "An imperative rule with, where useful, a brief note on its scope — short enough to apply without re-reading.",
	},
	{
		Name:       "glossary",
		Purpose:    "The shared vocabulary of the project — the domain and project-specific terms a contributor must understand to read the rest of the knowledge base and the code.",
		Boundary:   "Not an explanation of how a thing works (that is architecture) and not the rationale for a choice (that is a decision). A glossary entry defines what a term means, nothing more.",
		Tier:       CategoryTierAlwaysApplied,
		EntryShape: "A term and a short gloss — one or two sentences. Anything longer belongs in architecture, decisions, or learnings.",
	},
	{
		Name:       "architecture",
		Purpose:    "How the system is built and how new work must be built into it — components and their responsibilities, the boundaries between them, data and control flow, and the structures a change has to fit. An architecture entry is binding on the work being planned, not a survey of the code as it stands today.",
		Boundary:   "Not why a structure was chosen over the alternatives (that is a decision) and not a defined term (that is a glossary entry). Architecture states the structure to build to; where an entry and the current code disagree, the entry states the target and the code is what has yet to meet it.",
		Tier:       CategoryTierLookedUp,
		EntryShape: "A focused description of one component, boundary, or flow, written so a reader can place it in the larger system.",
	},
	{
		Name:       "gotchas",
		Purpose:    "Sharp edges and non-obvious traps — surprising behaviours, easy-to-make mistakes, and the things that bite a contributor who does not already know about them.",
		Boundary:   "Not a standing rule (that is a convention) and not the structure of the system (that is architecture). A gotcha is a warning about a specific trap and how to avoid it.",
		Tier:       CategoryTierLookedUp,
		EntryShape: "A short warning naming the trap, why it surprises, and what to do instead.",
	},
	{
		Name:       "learnings",
		Purpose:    "Empirical knowledge gained from doing the work — what was tried, what worked, what did not, and the practical findings that save the next contributor from repeating the effort.",
		Boundary:   "Not a recorded decision with its rationale (that is a decision) and not a standing rule the team must follow (that is a convention). A learning is an observation from experience.",
		Tier:       CategoryTierLookedUp,
		EntryShape: "A finding stated plainly, with enough context to know when it applies.",
	},
	{
		Name:       "decisions",
		Purpose:    "The reasoning behind choices the project has made — the options considered, the trade-offs weighed, and why one path was taken over the others (ADR-style).",
		Boundary:   "Not a description of the resulting structure (that is architecture) and not a rule to follow (that is a convention). A decision records the why, not the what or the how.",
		Tier:       CategoryTierLookedUp,
		EntryShape: "A record of the decision, the alternatives considered, and the rationale for the choice made.",
	},
}

// CategoryDescriptionFile is the filename a category's own generated
// description is written to, inside that category's directory. The name is
// stated once, here, because three behaviours depend on it — the retrieval
// exclusion, the refusal to delete a descriptor, and the drift comparison —
// and restating it at each of them is the drift this registry exists to
// prevent.
const CategoryDescriptionFile = "README.md"

// IsCategoryDescription reports whether a store-relative path addresses a
// category's own generated description rather than a knowledge entry. A
// description is rendered from the registry by README() and written by project
// init and repo-footprint scaffolding; it documents the category rather than
// contributing to it, which is why it is kept out of retrieval and cannot be
// deleted.
//
// True only for "<registry category>/README.md" exactly, which is the only
// shape either write site produces. A README deeper inside a category is a
// contributor's own file and is treated as an ordinary entry — the exclusion
// has no claim over content someone wrote themselves.
func IsCategoryDescription(path string) bool {
	segments := strings.Split(strings.TrimPrefix(filepath.ToSlash(path), "./"), "/")
	if len(segments) != 2 || segments[1] != CategoryDescriptionFile {
		return false
	}
	_, ok := CategoryByName(segments[0])
	return ok
}

// TagReachability reports the labels an entry declares that no search will
// ever reach, together with what to do about it. It travels on a successful
// write's result rather than as a refusal: the entry is still written, exactly
// as it would have been, and the caller is told so it finds out at the moment
// of writing rather than never.
type TagReachability struct {
	Category    string   `json:"category"`
	Unreachable []string `json:"unreachable_tags"`
	NextAction  string   `json:"next_action"`
}

// UnreachableTags reports which of an entry's declared labels a search can
// never reach, or nil when every one of them is reachable.
//
// Labels on an entry in an always-applied category are inert by construction:
// those categories are deliberately excluded from search and from the label
// vocabulary, because their entries are loaded in full on every task. That
// exclusion is correct and is not weakened here — what changes is that such
// labels stop being accepted in silence.
//
// It is a package function rather than a method on Set because it consults no
// store: keeping it callable without one is what lets the command layer ask
// before or after a write without an ordering constraint. It lives beside the
// registry because its next action has to name the destination's retrieval tier
// and the categories a search does reach, which only the registry knows.
func UnreachableTags(path string, content []byte) *TagReachability {
	category := categoryOf(path)
	if category == "" {
		return nil
	}
	if !slices.Contains(AlwaysApplied(), category) {
		return nil
	}
	tags, _, _ := store.ParseEntry(content)
	if len(tags) == 0 {
		return nil
	}
	var lookedUp []string
	for _, c := range Categories {
		if c.Tier == CategoryTierLookedUp {
			lookedUp = append(lookedUp, c.Name)
		}
	}
	return &TagReachability{
		Category:    category,
		Unreachable: tags,
		NextAction: fmt.Sprintf(
			"the %q category is always-applied: its entries are loaded in full on every task and are deliberately excluded from search and from the label vocabulary, so these labels can never be matched. Either move the entry to a looked-up category, where labels are searchable (%s), or drop the labels — the entry itself is written either way",
			category, strings.Join(lookedUp, ", "),
		),
	}
}

// README renders the category's self-documenting README content from its
// registry definition. Project init and repo-footprint creation both write
// this file, so the rendering lives with the registry it projects.
func (c Category) README() string {
	title := strings.Title(c.Name) //nolint:staticcheck // simple capitalisation
	return fmt.Sprintf(
		"# %s\n\n**Tier:** %s\n\n**Purpose:** %s\n\n**Belongs elsewhere:** %s\n\n**Entry shape:** %s\n",
		title, c.Tier, c.Purpose, c.Boundary, c.EntryShape,
	)
}

// AlwaysApplied returns the names of the categories in the always-applied tier,
// in registry order. Both the search-exclusion in the knowledge Set and the
// always-applied reader consult this single derivation, so the two behaviours
// can never drift apart.
func AlwaysApplied() []string {
	var names []string
	for _, c := range Categories {
		if c.Tier == CategoryTierAlwaysApplied {
			names = append(names, c.Name)
		}
	}
	return names
}

// CategoryByName returns the category with the given name and true, or a zero
// Category and false if no category in the registry has that name.
func CategoryByName(name string) (Category, bool) {
	for _, c := range Categories {
		if c.Name == name {
			return c, true
		}
	}
	return Category{}, false
}
