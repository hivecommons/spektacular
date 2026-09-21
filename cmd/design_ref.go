package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/design"
	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// The reference verbs live in the command layer rather than in
// internal/design, deliberately. That package owns design sources; a spec
// lives in the project's spec store, which is a different concern. Keeping the
// recorder here means internal/design never has to know where specs are kept,
// and matches how cmd/knowledge.go composes config, repo and knowledge rather
// than pushing that wiring down into a domain package.

var designRefCmd = &cobra.Command{
	Use:   "ref",
	Short: "Record, remove and resolve the design references a spec carries",
	RunE:  runUnknownSubcommand,
}

var designRefAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Record a design reference on a spec",
	RunE:  runDesignRefAdd,
}

var designRefRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a design reference from a spec",
	RunE:  runDesignRefRemove,
}

var designRefListCmd = &cobra.Command{
	Use:   "list",
	Short: "Report every design reference a spec carries, and whether each resolves",
	RunE:  runDesignRefList,
}

// designRefInput is the --data payload for the reference verbs. add and remove
// need all three fields; list needs only the spec.
type designRefInput struct {
	Spec   string `json:"spec"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

var designRefInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec":   {Type: "string"},
		"source": {Type: "string"},
		"path":   {Type: "string"},
	},
	Required: []string{"spec", "source", "path"},
}

var designRefListInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec": {Type: "string"},
	},
	Required: []string{"spec"},
}

var designRefWriteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec": {Type: "string"},
		"designs": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"source": {Type: "string"},
			"path":   {Type: "string"},
		}}},
	},
}

var designRefListOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec": {Type: "string"},
		"refs": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"source":   {Type: "string"},
			"path":     {Type: "string"},
			"resolved": {Type: "boolean"},
			"location": {Type: "string"},
		}}},
		"unresolved":  {Type: "integer"},
		"next_action": {Type: "string"},
	},
}

// designRefData parses the --data flag for the reference verbs. requireDoc is
// false for list, which addresses only a spec.
func designRefData(cmd *cobra.Command, requireDoc bool) (designRefInput, error) {
	example := `--data '{"spec":"000054_example"}'`
	if requireDoc {
		example = `--data '{"spec":"000054_example","source":"api","path":"payments/v2.md"}'`
	}
	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == "" {
		return designRefInput{}, output.NewError(
			"design_data_required",
			"--data is required",
		).WithNextAction(fmt.Sprintf("reissue with %s; run 'spec file list' for spec names and 'design sources' for the declared source names", example))
	}
	var input designRefInput
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return designRefInput{}, fmt.Errorf("parsing --data: %w", err)
	}
	if input.Spec == "" {
		return designRefInput{}, output.NewError(
			"design_ref_spec_required",
			`--data must include a non-empty "spec"`,
		).WithNextAction("reissue naming the spec the reference belongs to; run 'spec file list' to see the stored specs")
	}
	if requireDoc && (input.Source == "" || input.Path == "") {
		return designRefInput{}, output.NewError(
			"design_address_incomplete",
			`--data must include both a "source" and a "path"`,
		).WithNextAction(fmt.Sprintf("reissue with %s; run 'design sources' to see the declared source names", example))
	}
	return input, nil
}

// specStore resolves the spec store and the path a spec name addresses within
// it, appending the .md extension when the caller gives a bare name.
func specStore(name string) (store.Store, string, error) {
	st, dir, err := storeFileStore(func(c config.Config) string { return c.Spec.Config.Directory })
	if err != nil {
		return nil, "", err
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	specPath := filepath.Join(dir, name)
	if !st.Exists(specPath) {
		return nil, "", output.NewError(
			"design_ref_spec_not_found",
			fmt.Sprintf("no spec named %q is stored in this project", name),
		).WithResource(specPath).WithNextAction("run 'spec file list' to see the stored specs, and reissue with one of those names")
	}
	return st, specPath, nil
}

// refsOf reads a spec's currently recorded references.
func refsOf(st store.Store, specPath string) ([]metadata.DesignRef, []byte, error) {
	raw, err := st.Read(specPath)
	if err != nil {
		return nil, nil, err
	}
	fm, _, err := metadata.Split(raw)
	if err != nil {
		return nil, nil, err
	}
	if fm == nil {
		return nil, raw, nil
	}
	return fm.Designs, raw, nil
}

// writeRefs stores a new reference list on a spec, rewriting only its
// frontmatter. The body is carried through untouched: a reference is recorded
// in the spec's record, never in its prose, which is what keeps the design's
// own content out of the spec.
func writeRefs(st store.Store, specPath string, raw []byte, refs []metadata.DesignRef) error {
	_, body, err := metadata.Split(raw)
	if err != nil {
		return err
	}
	next := refs
	if next == nil {
		next = []metadata.DesignRef{}
	}
	merged, err := metadata.Merge(raw, body, metadata.UpdateOptions{Designs: &next})
	if err != nil {
		return err
	}
	return st.Write(specPath, merged)
}

// bareSpecName normalises a spec name to the form stored in a design's
// back-link list. specStore accepts a bare name or one suffixed with .md and
// appends the extension itself, so both spellings address the same spec; the
// bare form is what `spec file list` reports, so storing that keeps a specs:
// list stable however the caller happened to spell it.
func bareSpecName(name string) string {
	return strings.TrimSuffix(name, ".md")
}

// nextBackLinks computes a design's new back-link list, returning false when
// nothing would change. Add is idempotent and remove of an absent entry is a
// no-op, matching how the reference verbs already treat the spec side.
func nextBackLinks(current []string, spec string, add bool) ([]string, bool) {
	if add {
		for _, s := range current {
			if s == spec {
				return nil, false
			}
		}
		return append(append([]string{}, current...), spec), true
	}
	kept := make([]string, 0, len(current))
	for _, s := range current {
		if s != spec {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(current) {
		return nil, false
	}
	return kept, true
}

// writeBackLink brings a design document's own record into agreement with a
// reference that has just been recorded on, or removed from, a spec.
//
// Two cases write nothing and are not failures. A reference may be recorded
// before its document exists, which the reference verbs allow on purpose, and
// a design the project already had carries no lifecycle record at all, which
// is the guarantee that referencing a team's own file leaves it untouched.
func writeBackLink(set *design.Set, doc design.Document, spec string, add bool) error {
	exists, err := set.Exists(doc)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	raw, err := set.Read(doc)
	if err != nil {
		return err
	}
	fm := authoredMetadata(raw)
	if fm == nil {
		return nil
	}
	next, changed := nextBackLinks(fm.Specs, spec, add)
	if !changed {
		return nil
	}
	_, body, err := metadata.Split(raw)
	if err != nil {
		return err
	}
	merged, err := metadata.Merge(raw, body, metadata.UpdateOptions{Specs: &next})
	if err != nil {
		return err
	}
	return set.Write(doc, merged)
}

// writeBackLinkFn indirects the back-link write so a test can force it to
// fail, which is the only way to exercise the compensating rollback below.
// The compensating-write-failed branch in particular cannot be provoked from
// the filesystem alone: it needs the spec to become unwritable *between* the
// two writes, and the only thing running between them is this call. The
// precedent for a package-level seam of this shape is `var sourceFS fs.FS =
// templates.FS` in internal/agent/skills.go, which exists for the same reason.
// A test substituting it must restore it in t.Cleanup, since -shuffle=on makes
// a leaked substitution a cross-test failure.
var writeBackLinkFn = writeBackLink

// applyRef performs the two-document write that keeps a spec and a design in
// agreement. It is shared by add and remove because the two differ only in how
// the new lists are computed; the ordering, and the failure behaviour layered
// on top of it, are identical and must not be allowed to drift apart.
//
// The order is fixed: the spec first, the design's back-link second. Writing
// the back-link first would make the spec trivially untouched if the second
// write failed, but it would leave a design listing a spec that does not
// reference it, which is the one thing a back-link must never do.
//
// There is no transaction available, so the two writes are made to fail as a
// unit by compensation: the spec's original bytes are written back when the
// back-link write fails. That compensation can itself fail, and that third
// outcome is the one most easily left unhandled, so it is named and reported
// rather than swallowed. The two failures carry different codes deliberately:
// one says retry, the other says repair two named files by hand, and an agent
// branching on the code must not treat them as the same thing.
func applyRef(st store.Store, specPath string, raw []byte, next []metadata.DesignRef, set *design.Set, doc design.Document, spec string, add bool) error {
	if err := writeRefs(st, specPath, raw, next); err != nil {
		return err
	}
	backLinkErr := writeBackLinkFn(set, doc, spec, add)
	if backLinkErr == nil {
		return nil
	}

	designPath, resolveErr := set.Resolve(doc)
	if resolveErr != nil {
		designPath = doc.Path
	}
	specAbs := filepath.Join(st.Root(), specPath)

	// refsOf already handed back the spec's original bytes, so the
	// compensation payload cost nothing to obtain.
	if rollbackErr := st.Write(specPath, raw); rollbackErr != nil {
		return output.NewError(
			"design_ref_backlink_rollback_failed",
			fmt.Sprintf("could not update %s (%s), and restoring %s afterwards also failed (%s); the spec now records a reference the design does not",
				designPath, backLinkErr.Error(), specAbs, rollbackErr.Error()),
		).WithResource(specAbs).WithNextAction(fmt.Sprintf(
			"reconcile the two by hand: %s and %s disagree, so either remove the reference from the spec or bring the design's specs list into agreement with it", specAbs, designPath))
	}
	return output.NewError(
		"design_ref_backlink_failed",
		fmt.Sprintf("could not update the back-link on %s (%s); the spec was left exactly as it was and nothing was recorded",
			designPath, backLinkErr.Error()),
	).WithResource(designPath).WithNextAction(
		"make the design document writable and reissue the same command; no manual repair is needed")
}

func refItems(refs []metadata.DesignRef) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		out = append(out, map[string]any{"source": r.Source, "path": r.Path})
	}
	return out
}

// runDesignRefAdd records a reference, refusing an undeclared source before
// anything is written.
//
// The design document itself need not exist yet. The spec mandates exactly one
// refusal at record time, an undeclared source, and a capture conversation
// naturally records the intent alongside or just before the write. Requiring
// the document to exist would impose an ordering between `design write` and
// `design ref add` that buys nothing, and `design ref list` already reports a
// reference that does not resolve.
func runDesignRefAdd(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: designRefInputSchema, Output: designRefWriteOutputSchema}, "")
	}
	input, err := designRefData(cmd, true)
	if err != nil {
		return err
	}

	// Validate the source against the project's declarations first, so a
	// reference naming a source that does not exist leaves the spec untouched.
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	doc := design.Document{Source: input.Source, Path: input.Path}
	if _, err := set.Resolve(doc); err != nil {
		return err
	}

	st, specPath, err := specStore(input.Spec)
	if err != nil {
		return err
	}
	refs, raw, err := refsOf(st, specPath)
	if err != nil {
		return err
	}

	// A duplicate is a no-op rather than an error: the operation is idempotent
	// by nature, and an agent re-running a capture after an interruption
	// should not have to tell "already recorded" apart from "failed".
	for _, r := range refs {
		if r.Source == input.Source && r.Path == input.Path {
			out := output.New(cmd.OutOrStdout(), globalFields)
			return out.WriteResult(map[string]any{"spec": input.Spec, "designs": refItems(refs)})
		}
	}

	refs = append(refs, metadata.DesignRef{Source: input.Source, Path: input.Path})
	if err := applyRef(st, specPath, raw, refs, set, doc, bareSpecName(input.Spec), true); err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"spec": input.Spec, "designs": refItems(refs)})
}

// runDesignRefRemove drops a reference. Removing one the spec does not carry
// succeeds and changes nothing, mirroring add's idempotence.
func runDesignRefRemove(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: designRefInputSchema, Output: designRefWriteOutputSchema}, "")
	}
	input, err := designRefData(cmd, true)
	if err != nil {
		return err
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	st, specPath, err := specStore(input.Spec)
	if err != nil {
		return err
	}
	refs, raw, err := refsOf(st, specPath)
	if err != nil {
		return err
	}

	kept := make([]metadata.DesignRef, 0, len(refs))
	removed := false
	for _, r := range refs {
		if r.Source == input.Source && r.Path == input.Path {
			removed = true
			continue
		}
		kept = append(kept, r)
	}
	// Both documents are left alone when the spec did not carry the reference:
	// the back-link removal follows the same condition as the spec write, so a
	// no-op remove does not rewrite the design.
	if removed {
		doc := design.Document{Source: input.Source, Path: input.Path}
		if err := applyRef(st, specPath, raw, kept, set, doc, bareSpecName(input.Spec), false); err != nil {
			return err
		}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"spec": input.Spec, "designs": refItems(kept)})
}

// runDesignRefList reports every reference a spec carries and whether each one
// resolves to a document that is actually there.
//
// It always returns a success envelope, even when nothing resolves. The plan
// workflow needs the whole picture in one call so it can report every broken
// reference at once, and output.ErrorResponse has fixed fields that cannot
// carry a list. Making the *read* the hard failure is what stops planning
// proceeding on a broken reference; this command's job is to show which ones
// they are.
func runDesignRefList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: designRefListInputSchema, Output: designRefListOutputSchema}, "")
	}
	input, err := designRefData(cmd, false)
	if err != nil {
		return err
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	st, specPath, err := specStore(input.Spec)
	if err != nil {
		return err
	}
	refs, _, err := refsOf(st, specPath)
	if err != nil {
		return err
	}

	items := make([]map[string]any, 0, len(refs))
	unresolved := 0
	for _, r := range refs {
		doc := design.Document{Source: r.Source, Path: r.Path}
		item := map[string]any{"source": r.Source, "path": r.Path, "resolved": false, "location": ""}
		location, resolveErr := set.Resolve(doc)
		if resolveErr != nil {
			// The source itself is no longer declared. The reference cannot be
			// resolved and there is no location to report, but it is still
			// listed: a reference the project can no longer address is exactly
			// what this command exists to surface.
			unresolved++
			items = append(items, item)
			continue
		}
		item["location"] = location
		exists, existsErr := set.Exists(doc)
		if existsErr != nil || !exists {
			unresolved++
			items = append(items, item)
			continue
		}
		item["resolved"] = true
		items = append(items, item)
	}

	result := map[string]any{
		"spec":       input.Spec,
		"refs":       items,
		"unresolved": unresolved,
	}
	if unresolved > 0 {
		result["next_action"] = fmt.Sprintf(
			"%d of %d design references do not resolve; run 'design list' to see what each declared source holds, write the missing document with 'design write', or drop the reference with 'design ref remove'",
			unresolved, len(items))
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(result)
}

func init() {
	designRefAddCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"spec":"000054_example","source":"api","path":"payments/v2.md"}')`)
	designRefRemoveCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"spec":"000054_example","source":"api","path":"payments/v2.md"}')`)
	designRefListCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"spec":"000054_example"}')`)

	designRefCmd.AddCommand(designRefAddCmd, designRefRemoveCmd, designRefListCmd)
	designCmd.AddCommand(designRefCmd)
}
