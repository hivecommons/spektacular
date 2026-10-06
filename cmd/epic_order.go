package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/depgraph"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// `epic order` is the only automatic writer of an epic's dependency graph.
// Two planned specs whose plans name the same file, and that nothing orders
// yet, would collide when implemented side by side, so the later-listed spec
// is made to depend on the earlier one. Overlap is read from the files each
// task's section of the plan's context document names. Only the epic is
// written: no spec changes, so no plan goes stale and nothing is re-planned.

var epicOrderCmd = &cobra.Command{
	Use:   "order <epic>",
	Short: "Order an epic's planned specs whose plans change the same files, or undo one added dependency",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicOrder,
}

type epicOrderEdge struct {
	Spec      string `json:"spec"`
	DependsOn string `json:"depends_on"`
}

type epicOrderInput struct {
	Unorder *epicOrderEdge `json:"unorder"`
}

// epicOrderAdded is one dependency `epic order` added, with the files the two
// plans share.
type epicOrderAdded struct {
	Spec      string             `json:"spec"`
	DependsOn string             `json:"depends_on"`
	Files     []plantask.FileRef `json:"files"`
}

var epicOrderEdgeSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"spec":       {Type: "string", Description: "the later spec"},
	"depends_on": {Type: "string", Description: "the earlier spec it depends on"},
}}

var epicOrderInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"unorder": {Type: "object", Properties: epicOrderEdgeSchema.Properties, Description: "remove this dependency and let the two specs run side by side; omit to order the epic"},
	},
}

var fileRefSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"repo": {Type: "string"},
	"path": {Type: "string"},
}}

var epicOrderOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"epic": {Type: "string"},
		"added": {Type: "array", Description: "dependencies added for shared files (ordering only)", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"spec":       {Type: "string"},
			"depends_on": {Type: "string"},
			"files":      {Type: "array", Items: fileRefSchema},
		}}},
		"unplanned": {Type: "array", Items: &schemaProp{Type: "string"}, Description: "members with no plan, left out of the comparison (ordering only)"},
		"removed":   {Type: "object", Properties: epicOrderEdgeSchema.Properties, Description: "the dependency removed (unorder only)"},
	},
}

func runEpicOrder(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: epicOrderInputSchema, Output: epicOrderOutputSchema}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	var input epicOrderInput
	if dataStr, _ := cmd.Flags().GetString("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return output.NewError("bad_input", fmt.Sprintf("parsing --data: %v", err)).
				WithNextAction(fmt.Sprintf(`reissue without --data to order the epic, or with '{"unorder":{"spec":"<later>","depends_on":"<earlier>"}}' to remove one dependency: %s epic order %s`, cfg.Command, name))
		}
	}
	current, _, err := readEpic(cfg, st, name)
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	if input.Unorder != nil {
		removed, err := unorderEpic(st, cfg, name, current, *input.Unorder)
		if err != nil {
			return err
		}
		return out.WriteResult(map[string]any{"epic": name, "removed": removed})
	}
	added, unplanned, err := orderEpic(st, cfg, name, current)
	if err != nil {
		return err
	}
	if added == nil {
		added = []epicOrderAdded{}
	}
	return out.WriteResult(map[string]any{"epic": name, "added": added, "unplanned": nonNil(unplanned)})
}

// orderEpic adds a later-depends-on-earlier edge for every pair of planned
// members, in list order, whose plans share a file and that nothing orders
// yet, directly or through other specs, unless the user let the pair run side
// by side. It writes the epic and the summary's ordering log in one
// transaction, and writes nothing when no edge is added.
func orderEpic(st store.Store, cfg config.Config, name string, current epic.Epic) ([]epicOrderAdded, []string, error) {
	repos := make([]string, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repos = append(repos, r.Name)
	}
	files := make(map[string][]plantask.FileRef, len(current.Specs))
	var unplanned []string
	for _, s := range current.Specs {
		refs, planned, err := planFiles(st, cfg, s.Name, repos)
		if err != nil {
			return nil, nil, err
		}
		if !planned {
			unplanned = append(unplanned, s.Name)
			continue
		}
		files[s.Name] = refs
	}

	next := current
	next.Specs = make([]epic.EpicSpec, len(current.Specs))
	deps := make(map[string][]string, len(current.Specs))
	for i, s := range current.Specs {
		s.DependsOn = slices.Clone(s.DependsOn)
		if s.DependsOn == nil {
			s.DependsOn = []string{}
		}
		next.Specs[i] = s
		deps[s.Name] = s.DependsOn
	}

	var added []epicOrderAdded
	for j := range next.Specs {
		later := &next.Specs[j]
		for i := range j {
			earlier := next.Specs[i]
			shared := sharedFiles(files[earlier.Name], files[later.Name])
			if len(shared) == 0 ||
				depgraph.Reaches(deps, later.Name, earlier.Name) ||
				depgraph.Reaches(deps, earlier.Name, later.Name) ||
				slices.Contains(later.ParallelWith, earlier.Name) ||
				slices.Contains(earlier.ParallelWith, later.Name) {
				continue
			}
			later.DependsOn = append(later.DependsOn, earlier.Name)
			deps[later.Name] = later.DependsOn
			added = append(added, epicOrderAdded{Spec: later.Name, DependsOn: earlier.Name, Files: shared})
		}
	}
	if len(added) == 0 {
		return nil, unplanned, nil
	}

	lines := make([]string, len(added))
	for i, a := range added {
		quoted := make([]string, len(a.Files))
		for k, f := range a.Files {
			quoted[k] = "`" + fileRefLabel(f) + "`"
		}
		lines[i] = fmt.Sprintf("- Added: `%s` now depends on `%s`. Both change %s.", a.Spec, a.DependsOn, strings.Join(quoted, ", "))
	}
	if err := writeOrderedEpic(st, cfg, name, current, next, lines); err != nil {
		return nil, nil, err
	}
	return added, unplanned, nil
}

// unorderEpic removes one dependency and records the pair in the later
// spec's parallel_with, so a later `epic order` never adds it back.
func unorderEpic(st store.Store, cfg config.Config, name string, current epic.Epic, edge epicOrderEdge) (epicOrderEdge, error) {
	next := current
	next.Specs = slices.Clone(current.Specs)
	idx := slices.IndexFunc(next.Specs, func(s epic.EpicSpec) bool { return s.Name == edge.Spec })
	if idx < 0 || !slices.Contains(next.Specs[idx].DependsOn, edge.DependsOn) {
		return epicOrderEdge{}, output.NewError("epic_dependency_not_found",
			fmt.Sprintf("epic %q records no dependency of %q on %q", name, edge.Spec, edge.DependsOn)).
			WithResource(edge.Spec).
			WithNextAction(fmt.Sprintf("run `%s status %s --format json` to see the epic's current dependencies, then reissue naming one of them", cfg.Command, name))
	}
	entry := next.Specs[idx]
	entry.DependsOn = slices.DeleteFunc(slices.Clone(entry.DependsOn), func(d string) bool { return d == edge.DependsOn })
	if !slices.Contains(entry.ParallelWith, edge.DependsOn) {
		entry.ParallelWith = append(slices.Clone(entry.ParallelWith), edge.DependsOn)
	}
	next.Specs[idx] = entry
	line := fmt.Sprintf("- Removed at review: `%s` no longer depends on `%s`; they may be implemented side by side.", edge.Spec, edge.DependsOn)
	if err := writeOrderedEpic(st, cfg, name, current, next, []string{line}); err != nil {
		return epicOrderEdge{}, err
	}
	return edge, nil
}

// writeOrderedEpic validates and stamps next, then writes it with its body
// untouched, and appends lines to the summary's ordering log, all in one
// transaction. Membership does not change, so no spec is written.
func writeOrderedEpic(st store.Store, cfg config.Config, name string, current, next epic.Epic, lines []string) error {
	if err := epic.Validate(next.Specs); err != nil {
		return err
	}
	next, err := epic.Stamp(&current, next, nil, time.Now().UTC())
	if err != nil {
		return err
	}
	rendered, err := next.Render()
	if err != nil {
		return err
	}
	t := newDocTxn(st)
	if err := t.write(epicPath(cfg, name), rendered); err != nil {
		return t.fail(err)
	}
	if err := appendOrdering(t, cfg, name, next.SpecNames(), lines); err != nil {
		return t.fail(err)
	}
	return nil
}

// planFiles returns the files a spec's plan says its tasks change, in task
// order, and whether the spec has a plan at all.
func planFiles(st store.Store, cfg config.Config, spec string, repos []string) ([]plantask.FileRef, bool, error) {
	read := func(doc string) ([]byte, error) {
		raw, err := st.Read(artifact.Address{Kind: artifact.KindPlan, Feature: spec, Document: doc}.StorePath(cfg.Plan.Config.Directory))
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return raw, err
	}
	plan, err := read("plan")
	if err != nil || plan == nil {
		return nil, false, err
	}
	context, err := read("context")
	if err != nil {
		return nil, false, err
	}
	byTask := plantask.TaskFiles(plan, context, repos)
	var refs []plantask.FileRef
	for _, task := range plantask.Parse(plan).Tasks {
		for _, f := range byTask[task.Title] {
			if !slices.Contains(refs, f) {
				refs = append(refs, f)
			}
		}
	}
	return refs, true, nil
}

// sharedFiles lists the files in both a and b, in a's order.
func sharedFiles(a, b []plantask.FileRef) []plantask.FileRef {
	var shared []plantask.FileRef
	for _, f := range a {
		if slices.Contains(b, f) {
			shared = append(shared, f)
		}
	}
	return shared
}

// fileRefLabel writes a file as <repo>:<path>, or the bare path when it has
// no repo.
func fileRefLabel(f plantask.FileRef) string {
	if f.Repo == "" {
		return f.Path
	}
	return f.Repo + ":" + f.Path
}

func init() {
	epicOrderCmd.Flags().StringP("data", "d", "", `JSON input; omit to order the epic, or '{"unorder":{"spec":"<later>","depends_on":"<earlier>"}}' to remove one dependency`)
	epicCmd.AddCommand(epicOrderCmd)
}
