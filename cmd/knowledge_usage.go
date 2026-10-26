package cmd

import (
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/gitexec"
	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// knowledgeUsageCmd reports each entry's usage index record: when it was
// created, when the index started tracking it, and its discovery and retrieval
// history. It is read-only, and it is the way people and tools read the index,
// which the entry commands refuse to address.
var knowledgeUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show each entry's creation date and discovery and retrieval history from the store's usage index",
	RunE:  runKnowledgeUsage,
}

// knowledgeSpec backs the --spec option of always-applied and conventions: the
// spec the caller is working on, so a retrieval made while several workflows
// run in parallel is attributed to the right one. It never changes what the
// command returns.
var knowledgeSpec string

// knowledgeNow is the clock usage is recorded against. Tests replace it.
var knowledgeNow = func() time.Time { return time.Now().UTC() }

// knowledgeLaneKinds are the workflow kinds whose orchestrated runs keep a lane
// of their own, and so can be in progress beside the shared workflow.
var knowledgeLaneKinds = []string{"spec", "plan", "implement"}

// usageLogSchema describes one discovery or retrieval history.
var usageLogSchema = &schemaProp{
	Type: "object",
	Properties: map[string]*schemaProp{
		"count":  {Type: "number"},
		"last":   {Type: "string"},
		"recent": {Type: "array", Items: &schemaProp{Type: "object"}},
	},
}

var knowledgeUsageOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"entries": knowledgeItemSchema(map[string]*schemaProp{
			"path":          {Type: "string"},
			"created":       {Type: "string"},
			"tracked_since": {Type: "string"},
			"tracked":       {Type: "boolean"},
			"discoveries":   usageLogSchema,
			"retrievals":    usageLogSchema,
		}),
	},
}

// knowledgeSpecFlags is the schema always-applied and conventions advertise:
// the shared narrowing options plus --spec, which only those two take.
var knowledgeSpecFlags = func() map[string]*schemaProp {
	flags := make(map[string]*schemaProp, len(knowledgeNarrowingFlags)+1)
	for name, prop := range knowledgeNarrowingFlags {
		flags[name] = prop
	}
	flags["spec"] = &schemaProp{Type: "string"}
	return flags
}()

func runKnowledgeUsage(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeUsageOutputSchema, Flags: knowledgeNarrowingFlags}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	entries, err := set.Usage(knowledgeSelector())
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []knowledge.EntryUsage{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"entries": entries})
}

// recordUsage runs a usage-recording call after a command has its result. A
// failure never fails the command: the result is complete without it, so the
// failure is reported beside the result as usage_warning instead.
func recordUsage(result map[string]any, record func() error) {
	if err := record(); err != nil {
		result["usage_warning"] = "usage not recorded: " + err.Error() +
			"; the result is complete, and the next knowledge command retries the index update"
	}
}

// gitCreatedDater dates an entry the usage index has not seen before from the
// commit that first added it, when its store sits in a git work tree. Without
// git, outside a work tree, or for a file never committed, it has no date, and
// the knowledge layer falls back to the file's modification time.
func gitCreatedDater(location, entryPath string) (time.Time, bool) {
	out, err := gitexec.Run(location, nil, "log", "--diff-filter=A", "--follow", "--format=%aI", "--", entryPath)
	if err != nil {
		return time.Time{}, false
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	first := strings.TrimSpace(lines[len(lines)-1])
	if first == "" {
		return time.Time{}, false
	}
	created, err := time.Parse(time.RFC3339, first)
	if err != nil {
		return time.Time{}, false
	}
	return created.UTC(), true
}

// activeWorkflow names the workflow a retrieval is made under. When the caller
// names its spec and that spec has a workflow in progress (in a lane of its
// own, or in the shared state), that workflow is the answer. Otherwise a
// workflow is named only when exactly one is in progress: with several running
// side by side the CLI cannot tell which one made the call, and a wrong
// attribution is worse than none. It never fails; anything it cannot read
// counts as no workflow.
func activeWorkflow(spec string) *knowledge.WorkflowRef {
	dir, err := dataDir()
	if err != nil {
		return nil
	}
	type candidate struct {
		ref  knowledge.WorkflowRef
		lane bool
	}
	var candidates []candidate
	seen := make(map[knowledge.WorkflowRef]bool)
	add := func(ref knowledge.WorkflowRef, lane bool) {
		if ref.Kind == "" || seen[ref] {
			return
		}
		seen[ref] = true
		candidates = append(candidates, candidate{ref: ref, lane: lane})
	}
	for _, kind := range knowledgeLaneKinds {
		for _, name := range workflow.LaneNames(dir, kind) {
			if lane, err := workflow.ReadLane(dir, kind, name); err == nil && lane != nil && lane.InProgress() {
				add(knowledge.WorkflowRef{Kind: kind, Spec: name}, true)
			}
		}
	}
	if shared, err := readState(stateFilePath(dir)); err == nil && shared != nil && shared.InProgress() {
		name, _ := shared.Data["name"].(string)
		add(knowledge.WorkflowRef{Kind: shared.Kind, Spec: name}, false)
	}

	if spec != "" {
		var matched, lanes []knowledge.WorkflowRef
		for _, c := range candidates {
			if c.ref.Spec != spec {
				continue
			}
			matched = append(matched, c.ref)
			if c.lane {
				lanes = append(lanes, c.ref)
			}
		}
		switch {
		case len(matched) == 1:
			return &matched[0]
		case len(lanes) == 1:
			return &lanes[0]
		}
	}
	if len(candidates) == 1 {
		return &candidates[0].ref
	}
	return nil
}
