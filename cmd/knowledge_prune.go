package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/spf13/cobra"
)

// knowledgePruneCmd runs prune's two passes. The first runs here, in the
// command: entries old enough, tracked long enough and not retrieved within
// the project's prune age are removed. The second walks the agent through
// every remaining entry, one instruction per call, to correct it, delete it or
// keep it; the agent answers each through `knowledge prune verdict`, which
// checks the claim against the store before handing out the next entry.
//
// The loop's state is a small run file the command owns, outside every
// knowledge store, removed when the final report is delivered. It mirrors how
// workflows drive agents (one instruction per call, resume on re-entry)
// without a state machine of its own.
var knowledgePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old, unread entries, then check every remaining entry against the project; --dry-run previews without changing anything",
	RunE:  runKnowledgePrune,
}

var knowledgePruneVerdictCmd = &cobra.Command{
	Use:   "verdict",
	Short: "Record the verdict and evidence for the entry prune is checking, and get the next entry or the final report",
	RunE:  runKnowledgePruneVerdict,
}

// pruneRunFile is the run file's name under the project's .spektacular
// directory: outside every knowledge store, so a dry run leaves the knowledge
// base byte-identical, and outside the workflow lane folder.
const pruneRunFile = "knowledge-prune.json"

// pruneEntryTemplate is the instruction handed to the agent for each entry.
const pruneEntryTemplate = "knowledge/prune-entry.md"

// Verdicts an agent may report, and what the report calls each one.
const (
	pruneVerdictCurrent   = "current"
	pruneVerdictCorrected = "corrected"
	pruneVerdictDeleted   = "deleted"
	pruneVerdictRemoved   = "removed"
)

// pruneNow is prune's clock. Tests replace it.
var pruneNow = func() time.Time { return time.Now().UTC() }

var (
	pruneDryRun bool
	pruneForce  bool
)

// pruneRun is the run file: the prune's settings, what the first pass
// removed, the entries still to check (head first) and the verdicts so far.
type pruneRun struct {
	StartedAt  time.Time                `json:"started_at"`
	DryRun     bool                     `json:"dry_run"`
	MaxAgeDays int                      `json:"max_age_days"`
	Tier       string                   `json:"tier,omitempty"`
	Filter     []string                 `json:"filter,omitempty"`
	Removed    []knowledge.PruneRemoval `json:"removed"`
	Queue      []pruneItem              `json:"queue"`
	Verdicts   []pruneVerdict           `json:"verdicts"`
}

// pruneItem is one entry still to check, with its checksum when queued, which
// is what a verdict's claim is checked against.
type pruneItem struct {
	Tier     knowledge.Tier `json:"tier"`
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Checksum string         `json:"checksum"`
}

// pruneVerdict is one accepted verdict.
type pruneVerdict struct {
	Tier     knowledge.Tier `json:"tier"`
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Verdict  string         `json:"verdict"`
	Evidence string         `json:"evidence"`
}

// pruneVerdictInput is the --data payload of `knowledge prune verdict`.
type pruneVerdictInput struct {
	Tier     knowledge.Tier `json:"tier"`
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Verdict  string         `json:"verdict"`
	Evidence string         `json:"evidence"`
}

// pruneReportEntry is one row of the final report.
type pruneReportEntry struct {
	Tier     knowledge.Tier `json:"tier"`
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Verdict  string         `json:"verdict"`
	Evidence string         `json:"evidence"`
}

var pruneEntryProp = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"tier": {Type: "string"}, "name": {Type: "string"}, "path": {Type: "string"},
}}

var pruneReportProps = map[string]*schemaProp{
	"dry_run":      {Type: "boolean"},
	"max_age_days": {Type: "number"},
	"entries": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
		"tier": {Type: "string"}, "name": {Type: "string"}, "path": {Type: "string"},
		"verdict":  {Type: "string", Enum: []string{"removed", "corrected", "deleted", "current", "would_remove", "would_correct", "would_delete"}},
		"evidence": {Type: "string"},
	}}},
	"counts":   {Type: "object"},
	"complete": {Type: "boolean"},
}

var knowledgePruneOutputSchema = &schemaObj{
	Type: "object",
	Properties: func() map[string]*schemaProp {
		props := map[string]*schemaProp{
			"removed":     {Type: "array", Items: &schemaProp{Type: "object"}},
			"remaining":   {Type: "number"},
			"entry":       pruneEntryProp,
			"instruction": {Type: "string"},
			"resumed":     {Type: "boolean"},
		}
		for k, v := range pruneReportProps {
			props[k] = v
		}
		return props
	}(),
}

var knowledgePruneFlags = func() map[string]*schemaProp {
	flags := make(map[string]*schemaProp, len(knowledgeNarrowingFlags)+2)
	for name, prop := range knowledgeNarrowingFlags {
		flags[name] = prop
	}
	flags["dry-run"] = &schemaProp{Type: "boolean"}
	flags["force"] = &schemaProp{Type: "boolean"}
	return flags
}()

var knowledgePruneVerdictInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"tier":     {Type: "string", Enum: []string{string(knowledge.TierProject), string(knowledge.TierRepo)}},
		"name":     {Type: "string"},
		"path":     {Type: "string"},
		"verdict":  {Type: "string", Enum: []string{pruneVerdictCurrent, pruneVerdictCorrected, pruneVerdictDeleted}},
		"evidence": {Type: "string"},
	},
	Required: []string{"tier", "name", "path", "verdict"},
}

var knowledgePruneVerdictOutputSchema = &schemaObj{
	Type: "object",
	Properties: func() map[string]*schemaProp {
		props := map[string]*schemaProp{
			"remaining":   {Type: "number"},
			"entry":       pruneEntryProp,
			"instruction": {Type: "string"},
		}
		for k, v := range pruneReportProps {
			props[k] = v
		}
		return props
	}(),
}

func runKnowledgePrune(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgePruneOutputSchema, Flags: knowledgePruneFlags}, "")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	runPath, err := pruneRunPath()
	if err != nil {
		return err
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)

	if !pruneForce {
		run, err := readPruneRun(runPath)
		if err != nil {
			return err
		}
		if run != nil {
			if run.DryRun != pruneDryRun {
				// Resuming a real prune when a preview was asked for, or the
				// reverse, would hand out instructions of the wrong kind.
				mode, asked, resume := "real", "a dry run", cfg.Command+" knowledge prune"
				if run.DryRun {
					mode, asked, resume = "dry-run", "a real prune", cfg.Command+" knowledge prune --dry-run"
				}
				return output.NewError("knowledge_prune_in_progress",
					fmt.Sprintf("an unfinished %s prune started %s is in progress, so %s cannot start", mode, run.StartedAt.Format(time.RFC3339), asked)).
					WithResource(runPath).
					WithNextAction(fmt.Sprintf(`resume it with "%s", or discard it and start the one you asked for by adding --force`, resume))
			}
			if len(run.Queue) == 0 {
				return writePruneReport(out, runPath, run)
			}
			result, err := pruneNextResult(set, cfg.Command, run)
			if err != nil {
				return err
			}
			result["resumed"] = true
			result["removed"] = nonNilRemovals(run.Removed)
			return out.WriteResult(result)
		}
	}

	maxAgeDays := cfg.Knowledge.PruneMaxAgeDays()
	sel := knowledgeSelector()
	usage, err := set.Usage(sel)
	if err != nil {
		return err
	}
	now := pruneNow()
	removals := knowledge.PruneCandidates(usage, time.Duration(maxAgeDays)*24*time.Hour, now)
	if !pruneDryRun {
		if err := applyPruneRemovals(set, removals); err != nil {
			return err
		}
	}

	removed := make(map[string]bool, len(removals))
	for _, r := range removals {
		removed[pruneKey(r.Tier, r.Name, r.Path)] = true
	}
	entries, err := set.List(sel)
	if err != nil {
		return err
	}
	var queue []pruneItem
	for _, e := range entries {
		if removed[pruneKey(e.Tier, e.Name, e.Path)] || knowledge.IsCategoryDescription(e.Path) {
			continue
		}
		// An untracked read: prune's own reads are never usage.
		content, err := set.Read(knowledge.Address{Tier: e.Tier, Name: e.Name}, e.Path)
		if err != nil {
			return err
		}
		queue = append(queue, pruneItem{Tier: e.Tier, Name: e.Name, Path: e.Path, Checksum: checksumOf(content)})
	}

	run := &pruneRun{
		StartedAt:  now,
		DryRun:     pruneDryRun,
		MaxAgeDays: maxAgeDays,
		Tier:       knowledgeTier,
		Filter:     knowledgeFilter,
		Removed:    nonNilRemovals(removals),
		Queue:      queue,
		Verdicts:   []pruneVerdict{},
	}
	if len(queue) == 0 {
		return out.WriteResult(pruneReport(run))
	}
	if err := writePruneRun(runPath, run); err != nil {
		return err
	}
	result, err := pruneNextResult(set, cfg.Command, run)
	if err != nil {
		return err
	}
	result["removed"] = run.Removed
	return out.WriteResult(result)
}

func runKnowledgePruneVerdict(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: knowledgePruneVerdictInputSchema, Output: knowledgePruneVerdictOutputSchema}, "")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	runPath, err := pruneRunPath()
	if err != nil {
		return err
	}
	run, err := readPruneRun(runPath)
	if err != nil {
		return err
	}
	if run == nil || len(run.Queue) == 0 {
		return output.NewError("knowledge_prune_not_running", "no knowledge prune is in progress, so there is no entry to give a verdict for").
			WithNextAction(fmt.Sprintf(`start one with "%s knowledge prune" (add --dry-run to preview it), then answer each entry it hands you`, cfg.Command))
	}

	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == "" {
		return output.NewError("knowledge_data_required", "--data is required").
			WithNextAction(pruneVerdictCommand(cfg.Command, run.Queue[0]))
	}
	var input pruneVerdictInput
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return output.NewError("knowledge_prune_verdict_invalid", fmt.Sprintf("parsing --data: %v", err)).
			WithNextAction("reissue with valid JSON (keep single quotes out of the evidence): " + pruneVerdictCommand(cfg.Command, run.Queue[0]))
	}
	head := run.Queue[0]
	if input.Tier != head.Tier || input.Name != head.Name || input.Path != head.Path {
		return output.NewError("knowledge_prune_wrong_entry",
			fmt.Sprintf("prune is checking %s/%s %q, not %s/%s %q", head.Tier, head.Name, head.Path, input.Tier, input.Name, input.Path)).
			WithResource(input.Path).
			WithNextAction(fmt.Sprintf(`give the verdict for the entry being checked: %s; run "%s knowledge prune" to see its instruction again`,
				pruneVerdictCommand(cfg.Command, head), cfg.Command))
	}
	switch input.Verdict {
	case pruneVerdictCurrent, pruneVerdictCorrected, pruneVerdictDeleted:
	default:
		return output.NewError("knowledge_prune_verdict_invalid",
			fmt.Sprintf("verdict %q is not one of %q, %q or %q", input.Verdict, pruneVerdictCurrent, pruneVerdictCorrected, pruneVerdictDeleted)).
			WithNextAction(pruneVerdictCommand(cfg.Command, head))
	}
	input.Evidence = strings.TrimSpace(input.Evidence)
	if input.Verdict != pruneVerdictCurrent && input.Evidence == "" {
		return output.NewError("knowledge_prune_evidence_required",
			fmt.Sprintf("a %s verdict needs evidence: the file, command, flag or behaviour you checked and what you found", input.Verdict)).
			WithNextAction(fmt.Sprintf(`reissue with "evidence" filled in, or, if you cannot back the change with evidence, give the entry "current": %s`,
				pruneVerdictCommand(cfg.Command, head)))
	}

	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	if err := checkPruneClaim(set, cfg.Command, run.DryRun, head, input.Verdict); err != nil {
		return err
	}

	run.Verdicts = append(run.Verdicts, pruneVerdict{
		Tier: head.Tier, Name: head.Name, Path: head.Path, Verdict: input.Verdict, Evidence: input.Evidence,
	})
	run.Queue = run.Queue[1:]
	out := output.New(cmd.OutOrStdout(), globalFields)
	if len(run.Queue) == 0 {
		return writePruneReport(out, runPath, run)
	}
	if err := writePruneRun(runPath, run); err != nil {
		return err
	}
	result, err := pruneNextResult(set, cfg.Command, run)
	if err != nil {
		return err
	}
	return out.WriteResult(result)
}

// checkPruneClaim checks a verdict against what the store holds, so the final
// report can never disagree with it: deleted means the entry is gone,
// corrected means its text changed, current means it is untouched, and in a
// dry run nothing may have changed at all. The read is untracked.
func checkPruneClaim(set *knowledge.Set, command string, dryRun bool, head pruneItem, verdict string) error {
	content, err := set.Read(knowledge.Address{Tier: head.Tier, Name: head.Name}, head.Path)
	present := true
	if err != nil {
		var refusal *output.ErrorResponse
		if !errors.As(err, &refusal) || refusal.Code != knowledge.ErrCodeEntryNotFound {
			return err
		}
		present = false
	}
	changed := !present || checksumOf(content) != head.Checksum
	address := fmt.Sprintf(`{"tier":%q,"name":%q,"path":%q}`, head.Tier, head.Name, head.Path)

	if dryRun {
		if !changed {
			return nil
		}
		return output.NewError("knowledge_prune_dry_run_changed",
			fmt.Sprintf("this is a dry run, but %s/%s %q has changed since prune queued it; a dry run changes nothing", head.Tier, head.Name, head.Path)).
			WithResource(head.Path).
			WithNextAction(fmt.Sprintf("restore the entry from version history so it is unchanged, then give the verdict you would give in a real run: %s",
				pruneVerdictCommand(command, head)))
	}

	switch verdict {
	case pruneVerdictDeleted:
		if present {
			return output.NewError("knowledge_prune_claim_mismatch",
				fmt.Sprintf("the verdict says %s/%s %q was deleted, but the store still holds it", head.Tier, head.Name, head.Path)).
				WithResource(head.Path).
				WithNextAction(fmt.Sprintf("delete it with %s knowledge delete --data '%s' and give the verdict again, or give the verdict that matches the store", command, address))
		}
	case pruneVerdictCorrected:
		if !present {
			return output.NewError("knowledge_prune_claim_mismatch",
				fmt.Sprintf("the verdict says %s/%s %q was corrected, but the store no longer holds it", head.Tier, head.Name, head.Path)).
				WithResource(head.Path).
				WithNextAction(`give the verdict "deleted" with your evidence, or restore the entry and write the corrected text at its address`)
		}
		if !changed {
			return output.NewError("knowledge_prune_claim_mismatch",
				fmt.Sprintf("the verdict says %s/%s %q was corrected, but its text is unchanged", head.Tier, head.Name, head.Path)).
				WithResource(head.Path).
				WithNextAction(fmt.Sprintf("write the corrected text with %s knowledge write --data '%s' --file <staged file> and give the verdict again, or give the entry \"current\"", command, address))
		}
	case pruneVerdictCurrent:
		if changed {
			what := "its text has changed"
			if !present {
				what = "the store no longer holds it"
			}
			return output.NewError("knowledge_prune_claim_mismatch",
				fmt.Sprintf("the verdict says %s/%s %q is current, but %s", head.Tier, head.Name, head.Path, what)).
				WithResource(head.Path).
				WithNextAction(`give the verdict that matches the change you made ("corrected" or "deleted", with evidence), or restore the entry from version history and give "current"`)
		}
	}
	return nil
}

// applyPruneRemovals deletes the first pass's removals and drops their usage
// records, with one index update per store.
func applyPruneRemovals(set *knowledge.Set, removals []knowledge.PruneRemoval) error {
	byStore := make(map[knowledge.Address][]string)
	var order []knowledge.Address
	for _, r := range removals {
		addr := knowledge.Address{Tier: r.Tier, Name: r.Name}
		if err := set.Delete(addr, r.Path); err != nil {
			return err
		}
		if _, ok := byStore[addr]; !ok {
			order = append(order, addr)
		}
		byStore[addr] = append(byStore[addr], r.Path)
	}
	for _, addr := range order {
		if err := set.ForgetRecords(addr, byStore[addr]); err != nil {
			return err
		}
	}
	return nil
}

// pruneNextResult renders the instruction for the entry at the head of the
// queue, with the entry's text inlined so the agent never needs a tracked read.
func pruneNextResult(set *knowledge.Set, command string, run *pruneRun) (map[string]any, error) {
	head := run.Queue[0]
	content, err := set.Read(knowledge.Address{Tier: head.Tier, Name: head.Name}, head.Path)
	if err != nil {
		var refusal *output.ErrorResponse
		if !errors.As(err, &refusal) || refusal.Code != knowledge.ErrCodeEntryNotFound {
			return nil, err
		}
		content = nil
	}
	instruction, err := stepkit.RenderTemplate(pruneEntryTemplate, map[string]any{
		"command":   command,
		"tier":      string(head.Tier),
		"name":      head.Name,
		"path":      head.Path,
		"body":      strings.TrimRight(string(content), "\n"),
		"dry_run":   run.DryRun,
		"remaining": len(run.Queue),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"dry_run":      run.DryRun,
		"max_age_days": run.MaxAgeDays,
		"remaining":    len(run.Queue),
		"entry":        map[string]any{"tier": head.Tier, "name": head.Name, "path": head.Path},
		"instruction":  instruction,
	}, nil
}

// writePruneReport returns the final report and removes the run file: the
// prune is complete.
func writePruneReport(out *output.Writer, runPath string, run *pruneRun) error {
	if err := os.Remove(runPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return out.WriteResult(pruneReport(run))
}

// pruneReport lists every entry prune accounted for: the first pass's
// removals with their age and usage evidence, then every verdict. In a dry run
// the changes are reported as what a real run would do.
func pruneReport(run *pruneRun) map[string]any {
	entries := make([]pruneReportEntry, 0, len(run.Removed)+len(run.Verdicts))
	counts := map[string]int{}
	dryLabels := map[string]string{
		pruneVerdictRemoved:   "would_remove",
		pruneVerdictCorrected: "would_correct",
		pruneVerdictDeleted:   "would_delete",
	}
	label := func(verdict string) string {
		if l, ok := dryLabels[verdict]; ok && run.DryRun {
			return l
		}
		return verdict
	}
	for _, r := range run.Removed {
		last := "never"
		if r.LastRetrieval != nil {
			last = r.LastRetrieval.UTC().Format(time.DateOnly)
		}
		v := label(pruneVerdictRemoved)
		counts[v]++
		entries = append(entries, pruneReportEntry{
			Tier: r.Tier, Name: r.Name, Path: r.Path, Verdict: v,
			Evidence: fmt.Sprintf("created %s, tracked since %s, last retrieved %s, %d discoveries; not retrieved within %d days",
				r.Created.UTC().Format(time.DateOnly), r.TrackedSince.UTC().Format(time.DateOnly), last, r.Discoveries, run.MaxAgeDays),
		})
	}
	for _, v := range run.Verdicts {
		l := label(v.Verdict)
		counts[l]++
		entries = append(entries, pruneReportEntry{Tier: v.Tier, Name: v.Name, Path: v.Path, Verdict: l, Evidence: v.Evidence})
	}
	return map[string]any{
		"dry_run":      run.DryRun,
		"max_age_days": run.MaxAgeDays,
		"entries":      entries,
		"counts":       counts,
		"complete":     true,
	}
}

// pruneVerdictCommand is the exact verdict command for an entry, with the
// verdict and evidence left for the agent to fill in.
func pruneVerdictCommand(command string, item pruneItem) string {
	return fmt.Sprintf(`%s knowledge prune verdict --data '{"tier":%q,"name":%q,"path":%q,"verdict":"<current|corrected|deleted>","evidence":"<what you checked and what you found>"}'`,
		command, item.Tier, item.Name, item.Path)
}

func pruneRunPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, pruneRunFile), nil
}

// readPruneRun loads the run file, or nil when no prune is in progress.
func readPruneRun(path string) (*pruneRun, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var run pruneRun
	if err := json.Unmarshal(raw, &run); err != nil {
		return nil, output.NewError("knowledge_prune_run_unreadable", fmt.Sprintf("the prune run file %s cannot be read: %v", path, err)).
			WithResource(path).
			WithNextAction(`discard it and start again with "knowledge prune --force"`)
	}
	return &run, nil
}

// writePruneRun saves the run file through a temp file and rename.
func writePruneRun(path string, run *pruneRun) error {
	raw, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), pruneRunFile+".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

func checksumOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func pruneKey(tier knowledge.Tier, name, path string) string {
	return string(tier) + "\x00" + name + "\x00" + path
}

func nonNilRemovals(r []knowledge.PruneRemoval) []knowledge.PruneRemoval {
	if r == nil {
		return []knowledge.PruneRemoval{}
	}
	return r
}

func init() {
	knowledgePruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Preview: report what prune would remove, correct and delete, changing no entry and no usage record")
	knowledgePruneCmd.Flags().BoolVar(&pruneForce, "force", false, "Discard an unfinished prune and start again")
	knowledgePruneCmd.Flags().StringVar(&knowledgeTier, "tier", string(knowledge.TierAll), `Which knowledge to cover: "project", "repo", or "all"`)
	knowledgePruneCmd.Flags().StringArrayVar(&knowledgeFilter, "filter", nil, "Narrow to the named store(s) within the tier (repeatable); omit to cover every store the tier reaches")
	knowledgePruneVerdictCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"tier":"repo","name":"docs","path":"learnings/x.md","verdict":"current","evidence":"..."}')`)
	knowledgePruneCmd.AddCommand(knowledgePruneVerdictCmd)
	knowledgeCmd.AddCommand(knowledgePruneCmd)
}
