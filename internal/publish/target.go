package publish

import (
	"fmt"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Target is a work source a publication is filed into. The GitHub target
// lives in internal/publish/github; a Jira or Linear target implements the
// same three methods and the command layer needs no other change.
type Target interface {
	// Name is the target's marker value: github, jira, linear.
	Name() string
	// Find looks the target up for items already filed for artifactID by
	// their markers. It returns nil when none is.
	Find(ctx context.Context, artifactID string) (*Existing, error)
	// Publish files the publication: creating everything when existing is
	// nil, otherwise updating the filed items in place and creating the
	// children that are missing. The target renders final bodies itself via
	// Publication.RFCBody and Child.Render once it knows the references.
	Publish(ctx context.Context, p Publication, existing *Existing, opts PublishOptions) (Result, error)
}

// PublishOptions tunes a Publish call.
type PublishOptions struct {
	// ParentLabel is applied to the RFC when the target has such a label;
	// a target never creates labels.
	ParentLabel string
	// Labels are applied to every filed item when they exist.
	Labels []string
}

// HiveSuppressLabels are the labels a hive hard-suppresses (hive's
// src/pkg/github/labels.go): an item carrying one is never claimed, so
// publish refuses to file with them.
var HiveSuppressLabels = []string{"hold", "needs-human", "needs-decision", "needs-spec", "needs-direction"}

// CheckLabels refuses options that would put a hive suppress label on a
// filed item, naming the label and the setting it came from.
func (o PublishOptions) CheckLabels(section string) error {
	check := func(label, setting string) error {
		for _, s := range HiveSuppressLabels {
			if strings.EqualFold(strings.TrimSpace(label), s) {
				return &Error{
					Code:       ErrTargetUnconfigured,
					Message:    fmt.Sprintf("%s.%s names %q, a label a hive treats as do-not-claim; every filed item would be parked", section, setting, label),
					NextAction: fmt.Sprintf("remove %q from %s.%s in .spektacular/config.yaml", label, section, setting),
				}
			}
		}
		return nil
	}
	if err := check(o.ParentLabel, "parent_label"); err != nil {
		return err
	}
	for _, l := range o.Labels {
		if err := check(l, "labels"); err != nil {
			return err
		}
	}
	return nil
}

// Existing is what Find reports: the RFC and the children already filed.
type Existing struct {
	RFC      *Filed
	Children map[string]Filed
}

// Filed is one item already in the target.
type Filed struct {
	// ID is the target's own numeric identity: GitHub's database id. A
	// target whose identities are strings (a Jira key, a Linear UUID) leaves
	// it zero and sets Key.
	ID     int64
	Key    string
	Number int
	URL    string
	// Ref is how other bodies in the target link to it: "#12" on GitHub,
	// "PROJ-12" on Jira, "ENG-12" on Linear.
	Ref   string
	Title string
	// Body is the filed body as the target stores it. A target that cannot
	// read the body back verbatim (Jira stores a converted document) leaves
	// it empty and sets BodyDigest instead.
	Body string
	// BodyDigest is Digest of the body as filed, when the target records it.
	BodyDigest string
	Closed     bool
	Marker     Marker
	// Parent is the Key of the item this one hangs off as the target has
	// it filed (the RFC's key for a child), "" when it hangs off nothing.
	// BlockedBy lists the Keys of the items filed as blocking this one. A
	// Publish reconciles both against the publication on every run, so a
	// link that failed, a dependency added later or a re-created RFC is
	// repaired by the next run.
	Parent    string
	BlockedBy []string
}

// Hinted is implemented by a target whose search can lag behind its writes
// (Jira's index is eventually consistent). The command passes it the URLs
// the artifact's frontmatter recorded, and Find reads those items directly
// when the search does not return them.
type Hinted interface {
	Hint(urls []string)
}

// SearchToken is a word a target can put in a filed body to find it again
// where the artifact id itself cannot be searched for safely (Jira's text
// search treats /, = and - as operators). It is alphanumeric only, so no
// query grammar needs escaping.
func SearchToken(artifactID string) string {
	sum := sha256.Sum256([]byte("spek:publish " + artifactID))
	return "spekpub" + hex.EncodeToString(sum[:8])
}

// SameAsFiled reports whether body is what the filed item carries, by
// digest when the target recorded one, else by body text.
func (f Filed) SameAsFiled(body string) bool {
	if f.BodyDigest != "" {
		return f.BodyDigest == Digest(body)
	}
	return SameBody(f.Body, body)
}

// Result is what Publish reports.
type Result struct {
	Target    string        `json:"target"`
	Repo      string        `json:"repo"`
	RFCURL    string        `json:"rfc_url"`
	RFCNumber int           `json:"rfc_number,omitempty"`
	RFCKey    string        `json:"rfc_key,omitempty"`
	Children  []ResultChild `json:"children"`
	Created   []string      `json:"created"`
	Updated   []string      `json:"updated"`
	Unchanged []string      `json:"unchanged"`
	// Linked lists the parent and dependency relations this run filed or
	// repaired, as "<item> under <rfc>" and "<item> blocked by <blocker>".
	Linked   []string `json:"linked,omitempty"`
	Warnings []string `json:"warnings"`
}

// ResultChild is one filed child.
type ResultChild struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Number int    `json:"number,omitempty"`
	// Key is the target's own key for the item when it has one: a Jira
	// issue key, a Linear identifier.
	Key string `json:"key,omitempty"`
}

// Change is one difference between what is filed and what would be.
type Change struct {
	// Item is "rfc" or a child ID.
	Item string `json:"item"`
	// Kind is "create", "title" or "body".
	Kind string `json:"kind"`
}

// Diff compares the publication against what is filed and lists what a
// Publish would change. The RFC body is compared with the existing children's
// refs substituted, the child bodies with the RFC's URL, so an unchanged
// publication diffs clean. A child filed but no longer in the publication is
// left alone and not reported.
func Diff(p Publication, existing *Existing) []Change {
	if existing == nil {
		return nil
	}
	refs := map[string]Ref{}
	for id, f := range existing.Children {
		refs[id] = Ref{Ref: f.Ref, URL: f.URL}
	}
	var changes []Change
	rfcURL := ""
	if existing.RFC == nil {
		changes = append(changes, Change{Item: "rfc", Kind: "create"})
	} else {
		rfcURL = existing.RFC.URL
		if existing.RFC.Title != p.RFC.Title {
			changes = append(changes, Change{Item: "rfc", Kind: "title"})
		}
		if !existing.RFC.SameAsFiled(p.RFCBody(refs)) {
			changes = append(changes, Change{Item: "rfc", Kind: "body"})
		}
	}
	for _, c := range p.Children {
		f, ok := existing.Children[c.ID]
		if !ok {
			changes = append(changes, Change{Item: c.ID, Kind: "create"})
			continue
		}
		if f.Title != c.Title {
			changes = append(changes, Change{Item: c.ID, Kind: "title"})
		}
		if !f.SameAsFiled(c.Render(p, rfcURL, refs)) {
			changes = append(changes, Change{Item: c.ID, Kind: "body"})
		}
	}
	return changes
}

// SameBody compares bodies as a target stores them: line endings and
// trailing whitespace normalised.
func SameBody(a, b string) bool {
	return normaliseBody(a) == normaliseBody(b)
}

// Digest is a stable fingerprint of a body as a target stores it, for
// targets that cannot read the body back verbatim.
func Digest(body string) string {
	sum := sha256.Sum256([]byte(normaliseBody(body)))
	return hex.EncodeToString(sum[:])
}

func normaliseBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// DescribeChanges spells a diff out for a refusal message.
func DescribeChanges(changes []Change) string {
	parts := make([]string, 0, len(changes))
	for _, c := range changes {
		parts = append(parts, c.Item+" ("+c.Kind+")")
	}
	return strings.Join(parts, ", ")
}

// SortedIDs returns the keys of m in order.
func SortedIDs(m map[string]Filed) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
