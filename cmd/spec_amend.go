package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/design"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// specAmendInput is spec amend's --data payload.
type specAmendInput struct {
	Name   string              `json:"name"`
	Reason string              `json:"reason"`
	Run    string              `json:"run"`
	Design *metadata.DesignRef `json:"design,omitempty"`
}

var specAmendInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":   {Type: "string", Pattern: `^[a-z0-9_-]+$`, Description: "the planned spec to amend"},
		"reason": {Type: "string", Description: "why the spec or design was wrong, and what the user approved"},
		"run":    {Type: "string", Description: "the implement run the amendment came from, e.g. \"epic 000068_payments implement run, task 2.1\""},
		"design": {Type: "object", Description: "a design the spec references whose revision this records; omit when only the spec changed", Properties: map[string]*schemaProp{
			"source": {Type: "string"},
			"path":   {Type: "string"},
		}},
	},
	Required: []string{"name", "reason", "run"},
}

var specAmendOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec":             {Type: "string"},
		"amended_sections": {Type: "array", Items: &schemaProp{Type: "string"}},
		"design":           {Type: "object", Properties: map[string]*schemaProp{"source": {Type: "string"}, "path": {Type: "string"}}},
		"recorded_at":      {Type: "string", Description: "RFC3339 UTC"},
		"hash":             {Type: "string", Description: "sha256:<hex> of the amended body, checkbox marks normalised"},
		"dry_run":          {Type: "boolean"},
	},
}

var specAmendCmd = &cobra.Command{
	Use:   "amend",
	Short: "Amend a planned spec during an implement run, and record why",
	Long: `Amend a planned spec's Requirements, Acceptance Criteria, Constraints or
Success Metrics during an implement run, or record a revision already made to
a design the spec references. The command works out which sections changed,
refuses any other change, appends a dated entry to the spec's ## Amendments
section and records the amendment in its metadata. A recorded amendment does
not make the spec's plan stale.

Pass the full amended spec with --from, and/or the revised design in --data.`,
	Args: cobra.NoArgs,
	RunE: runSpecAmend,
}

func runSpecAmend(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  specAmendInputSchema,
			Output: specAmendOutputSchema,
			Flags:  map[string]*schemaProp{"from": {Type: "string", Description: "the full amended spec, staged under .spektacular/tmp/"}},
		}, "")
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	fromPath, _ := cmd.Flags().GetString("from")

	var input specAmendInput
	dataStr, _ := cmd.Flags().GetString("data")
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil || !nameRegexp.MatchString(input.Name) {
		msg := "--data must be a JSON object naming the spec"
		if err != nil {
			msg = fmt.Sprintf("parsing --data: %v", err)
		}
		return output.NewError("bad_input", msg).
			WithNextAction(`reissue with --data '{"name":"<spec>","reason":"<why>","run":"<implement run, task>"}'`)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	retry := fmt.Sprintf("reissue `%s spec amend` with it added to --data", cfg.Command)
	if strings.TrimSpace(input.Reason) == "" {
		return output.NewError("spec_amend_reason_required", "an amendment needs a reason: what was wrong and what the user approved").
			WithResource(input.Name).WithNextAction(`add "reason" and ` + retry)
	}
	if strings.TrimSpace(input.Run) == "" {
		return output.NewError("spec_amend_run_required", "an amendment needs the implement run it came from").
			WithResource(input.Name).WithNextAction(`add "run" (e.g. "interactive implement run, task 2.1") and ` + retry)
	}
	if fromPath == "" && input.Design == nil {
		return output.NewError("spec_amend_nothing_to_record", "there is nothing to record: pass the amended spec with --from, a revised design in --data, or both").
			WithResource(input.Name).
			WithNextAction(fmt.Sprintf("stage the full amended spec under .spektacular/tmp/%s/ and reissue with --from <that file>, or add \"design\":{\"source\":\"…\",\"path\":\"…\"} for a design already revised", input.Name))
	}

	root, err := projectRoot()
	if err != nil {
		return err
	}
	st := store.NewSourceStore(root, "project")
	specFile := artifact.Address{Kind: artifact.KindSpec, Feature: input.Name}.StorePath(cfg.Spec.Config.Directory)
	raw, err := st.Read(specFile)
	if errors.Is(err, store.ErrNotFound) {
		return output.NewError("spec_not_found", fmt.Sprintf("no spec named %q is stored in this project", input.Name)).
			WithResource(input.Name).
			WithNextAction(fmt.Sprintf("run `%s spec file list` to see the stored specs, and reissue with one of those names", cfg.Command))
	}
	if err != nil {
		return err
	}
	planFile := artifact.Address{Kind: artifact.KindPlan, Feature: input.Name, Document: "plan"}.StorePath(cfg.Plan.Config.Directory)
	if !st.Exists(planFile) {
		return output.NewError("spec_amend_no_plan", fmt.Sprintf("%q has no plan; spec amend is for correcting a planned spec during its implement run", input.Name)).
			WithResource(input.Name).
			WithNextAction(fmt.Sprintf("edit a spec that has not been planned with `%s spec file write %s --from <file>`", cfg.Command, input.Name))
	}
	fm, body, err := metadata.Split(raw)
	if err != nil {
		return output.NewError("metadata_merge_failed", err.Error()).WithResource(input.Name)
	}
	if fm == nil {
		fm = &metadata.Metadata{}
	}

	var sections []string
	if fromPath != "" {
		content, err := os.ReadFile(fromPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", fromPath, err)
		}
		next := stripLeadingFrontmatterBlocks(content)
		if sections, err = amendedSections(cfg.Command, input.Name, body, next); err != nil {
			return err
		}
		body = next
	}
	if input.Design != nil {
		if err := checkAmendedDesign(cfg.Command, root, input, fm.Designs); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	body = appendAmendmentEntry(body, amendmentEntry(now, sections, input))
	amendment := metadata.Amendment{
		At:       now.Format(time.RFC3339),
		Sections: sections,
		Design:   input.Design,
		Hash:     metadata.BodyHash(body),
	}
	amendments := append(append([]metadata.Amendment{}, fm.Amendments...), amendment)
	merged, err := metadata.Merge(raw, body, metadata.UpdateOptions{Amendments: &amendments})
	if err != nil {
		return output.NewError("metadata_merge_failed", err.Error()).WithResource(input.Name)
	}
	if !dryRun {
		if err := st.Write(specFile, merged); err != nil {
			return err
		}
	}

	result := map[string]any{
		"spec":             input.Name,
		"amended_sections": nonNilStrings(sections),
		"recorded_at":      amendment.At,
		"hash":             amendment.Hash,
		"dry_run":          dryRun,
	}
	if input.Design != nil {
		result["design"] = input.Design
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(result)
}

// amendedSections works out which sections the staged body changes and
// refuses an amendment that changes nothing, changes a section an implement
// run may not amend, or rewrites the amendment history.
func amendedSections(command, name string, prev, next []byte) ([]string, error) {
	changed := changedSections(prev, next)
	if len(changed) == 0 {
		return nil, output.NewError("spec_amend_no_change", "the staged spec changes nothing but checkbox marks").
			WithResource(name).
			WithNextAction(fmt.Sprintf("edit the amended section in the staged file and reissue, or record only a design revision by dropping --from; read the current text with `%s spec file read %s`", command, name))
	}
	var refused []string
	for _, h := range changed {
		switch {
		case h == amendmentsSection:
			refused = append(refused, "Amendments (its history is append-only and written by spec amend)")
		case h == "":
			refused = append(refused, preambleSection)
		case !isAmendable(h):
			refused = append(refused, h)
		}
	}
	if len(refused) > 0 {
		return nil, output.NewError("spec_amend_section_not_amendable",
			fmt.Sprintf("only %s can be amended during an implement run; the staged spec also changes: %s",
				strings.Join(amendableSections, ", "), strings.Join(refused, "; "))).
			WithResource(name).
			WithNextAction(fmt.Sprintf("restore those sections in the staged file from `%s spec file read %s` and reissue; a change beyond them means stopping the run and re-planning", command, name))
	}
	return changed, nil
}

// checkAmendedDesign refuses a design the spec does not reference, or one that
// does not resolve to a stored document.
func checkAmendedDesign(command, root string, input specAmendInput, refs []metadata.DesignRef) error {
	d := *input.Design
	label := d.Source + ":" + d.Path
	referenced := false
	for _, r := range refs {
		if r == d {
			referenced = true
			break
		}
	}
	if !referenced {
		return output.NewError("spec_amend_design_not_referenced", fmt.Sprintf("%s does not reference the design %s", input.Name, label)).
			WithResource(label).
			WithNextAction(fmt.Sprintf("run `%s design ref list --data '{\"spec\":\"%s\"}'` to see the designs it references; if this one belongs to it, record the reference with `%s design ref add` first", command, input.Name, command))
	}
	unresolved := func(why string) error {
		return output.NewError("spec_amend_design_unresolved", fmt.Sprintf("the design %s does not resolve: %s", label, why)).
			WithResource(label).
			WithNextAction(fmt.Sprintf("check the address with `%s design sources` and `%s design list`, revise the design with `%s design author` (or `%s design write` for one the user supplied), then reissue", command, command, command, command))
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	set, err := design.NewSet(cfg, root)
	if err != nil {
		return unresolved(err.Error())
	}
	exists, err := set.Exists(design.Document{Source: d.Source, Path: d.Path})
	if err != nil {
		return unresolved(err.Error())
	}
	if !exists {
		return unresolved("no document is stored at that path")
	}
	return nil
}

// amendmentEntry renders one `## Amendments` list item: the date, what was
// amended, the run, and the reason on an indented line of its own.
func amendmentEntry(at time.Time, sections []string, input specAmendInput) string {
	var what []string
	if len(sections) > 0 {
		what = append(what, strings.Join(sections, ", "))
	}
	if input.Design != nil {
		what = append(what, "design "+input.Design.Source+":"+input.Design.Path)
	}
	reason := strings.Join(strings.Fields(input.Reason), " ")
	run := strings.Join(strings.Fields(input.Run), " ")
	return fmt.Sprintf("- **%s: %s** (%s)\n  %s", at.Format(metadata.DateFormat), strings.Join(what, "; "), run, reason)
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func init() {
	specAmendCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"000070_billing","reason":"…","run":"interactive implement run, task 2.1"}')`)
	specAmendCmd.Flags().String("from", "", "Read the full amended spec from the file at <path> (relative to cwd)")
	specCmd.AddCommand(specAmendCmd)
}
