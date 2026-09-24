package knowledge

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// Tier says which knowledge a store holds: the project's own shared knowledge,
// or a registered repo's knowledge about its own code. It is a named string
// type so an arbitrary string cannot be passed where a tier is expected.
//
// This is a different axis from CategoryTier in category.go, which says when a
// category's entries are retrieved. The two never appear on the same object.
type Tier string

const (
	// TierProject addresses the stores the project itself declares, holding
	// knowledge that belongs to no single repo.
	TierProject Tier = "project"
	// TierRepo addresses the stores the registered repos declare, each holding
	// what is true of that repo's own code.
	TierRepo Tier = "repo"
	// TierAll addresses both tiers at once. It names a set rather than one
	// store, so it is valid for fan-out only and is refused by Read and Write.
	TierAll Tier = "all"
)

// Address names exactly one store: a tier, and the name the store is known by
// within that tier. Read and Write travel on it.
type Address struct {
	Tier Tier   `json:"tier"`
	Name string `json:"name"`
}

// Selector names a set of stores: a tier, and an optional flat list of store
// names narrowing it. Search, List, Conventions and AlwaysAppliedEntries travel
// on it. An empty Filter covers every store the tier reaches; a Filter naming a
// store the tier does not reach is an error, not a silent empty result.
//
// The value carries two narrowing axes that are applied at different points.
// Tier and Filter decide which stores are queried at all; Tags narrows the
// entries within them, restricting results to those carrying every listed tag.
// Keeping both on one value is what stops narrowing a request becoming two
// separate ideas threaded through every layer.
type Selector struct {
	Tier   Tier     `json:"tier"`
	Filter []string `json:"filter"`
	Tags   []string `json:"tags"`
}

// knowledge error codes. Each refusal built with one of these carries a next
// action naming the stores available in the tier concerned, so a caller can
// reissue the request without going and reading configuration.
const (
	// ErrCodeTierRequired is returned when a request states no tier at all.
	ErrCodeTierRequired = "knowledge_tier_required"
	// ErrCodeTierInvalid is returned for a tier that is not one of the three
	// values, and for "all" where a single store is required.
	ErrCodeTierInvalid = "knowledge_tier_invalid"
	// ErrCodeNameRequired is returned when a single-store request states no
	// store name.
	ErrCodeNameRequired = "knowledge_name_required"
	// ErrCodeStoreUnknown is returned when a named store does not exist in the
	// tier concerned.
	ErrCodeStoreUnknown = "knowledge_store_unknown"
	// ErrCodeEntryNotFound is returned when a correctly addressed store holds
	// no entry at the requested path.
	ErrCodeEntryNotFound = "knowledge_entry_not_found"
	// ErrCodeTagUnknown is returned when a search is narrowed to a tag no
	// entry in scope carries, so a caller is never left reading an empty
	// result as "no knowledge on this subject".
	ErrCodeTagUnknown = "knowledge_tag_unknown"
)

// validTier reports whether t is one of the three declared tiers.
func validTier(t Tier) bool {
	return t == TierProject || t == TierRepo || t == TierAll
}

// Validate checks the address names exactly one store. An empty tier, an
// unrecognised tier, TierAll, and an empty name are each refused. It does not
// check the store exists; Set.resolve does that, since only the set knows which
// stores are configured.
func (a Address) Validate() error {
	switch {
	case a.Tier == "":
		return output.NewError(ErrCodeTierRequired, `a knowledge address must state a "tier"`).
			WithNextAction(`reissue with "tier" set to "project" or "repo", e.g. --data '{"tier":"repo","name":"<name>","path":"<path>"}'`)
	case !validTier(a.Tier):
		return output.NewError(ErrCodeTierInvalid, fmt.Sprintf("unknown knowledge tier %q", a.Tier)).
			WithNextAction(`reissue with "tier" set to "project" or "repo"`)
	case a.Tier == TierAll:
		return output.NewError(ErrCodeTierInvalid, `tier "all" addresses both tiers and cannot name a single store`).
			WithNextAction(`reissue with "tier" set to "project" or "repo"`)
	case a.Name == "":
		return output.NewError(ErrCodeNameRequired, `a knowledge address must state a store "name"`).
			WithNextAction(`reissue with "name" set to one of the stores in that tier`)
	}
	return nil
}

// Validate checks the selector names a reachable set of stores. An empty or
// unrecognised tier is refused; TierAll is valid, and an empty Filter is valid
// and means every store the tier reaches. Filter names are checked against the
// configured stores by Set.validateSelector, which is the only place that
// knows them.
func (sel Selector) Validate() error {
	switch {
	case sel.Tier == "":
		return output.NewError(ErrCodeTierRequired, `a knowledge selector must state a "tier"`).
			WithNextAction(`reissue with --tier set to "project", "repo" or "all"`)
	case !validTier(sel.Tier):
		return output.NewError(ErrCodeTierInvalid, fmt.Sprintf("unknown knowledge tier %q", sel.Tier)).
			WithNextAction(`reissue with --tier set to "project", "repo" or "all"`)
	}
	return nil
}

// covers reports whether a store falls inside the set the selector names. The
// rule is applied to every store without exception: the store's tier must fall
// under the selector's tier, and when a filter is present the store's name must
// appear in it. No store is included because it was hard to attribute, and none
// is excluded for any other reason.
// covers deliberately knows nothing about Selector.Tags: it answers "is this
// store in scope", and tags narrow entries within a store rather than the set of
// stores. Teaching it about tags would exclude a whole store because one entry
// lacked a tag.
func (sel Selector) covers(src scopedStore) bool {
	if sel.Tier != TierAll && src.tier != sel.Tier {
		return false
	}
	if len(sel.Filter) == 0 {
		return true
	}
	for _, name := range sel.Filter {
		if name == src.name {
			return true
		}
	}
	return false
}

// availableNames renders the store names in a tier for an error's next action,
// or a plain statement when the tier holds no stores at all. TierAll spans both
// tiers, so it is rendered without naming one.
func availableNames(tier Tier, names []string) string {
	where := fmt.Sprintf(" in the %q tier", tier)
	if tier == TierAll {
		where = ""
	}
	if len(names) == 0 {
		return fmt.Sprintf("no stores are configured%s", where)
	}
	return fmt.Sprintf("stores available%s: %s", where, strings.Join(names, ", "))
}
