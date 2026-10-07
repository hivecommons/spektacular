package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// workflowSlot is where a workflow's state and working notes live. A
// standalone workflow uses the shared slot — state.json and
// working-context.md — exactly as before lanes existed. An orchestrated plan
// or implement workflow is a lane: a state file and notes file of its own
// under .spektacular/workflows/, keyed by kind and spec name.
//
// The workflow engine is unchanged; only the path it is handed differs. This
// follows the guided repo add's separate repo-state.json, generalised from one
// extra fixed file to one file per orchestrated spec.
type workflowSlot struct {
	Kind         string
	Name         string // spec name; "" for a goto that named none
	StatePath    string
	NotesPath    string
	Orchestrated bool
}

func sharedSlot(dataDir, kind, name string) workflowSlot {
	return workflowSlot{
		Kind:      kind,
		Name:      name,
		StatePath: stateFilePath(dataDir),
		NotesPath: filepath.Join(dataDir, "working-context.md"),
	}
}

func laneSlot(dataDir, kind, name string) workflowSlot {
	return workflowSlot{
		Kind:         kind,
		Name:         name,
		StatePath:    workflow.LaneStatePath(dataDir, kind, name),
		NotesPath:    workflow.LaneNotesPath(dataDir, kind, name),
		Orchestrated: true,
	}
}

// resolveGotoSlot finds the workflow a goto drives. With no name it is the
// shared slot, so instructions rendered before goto carried a name, and
// hand-typed commands, keep working. With a name it is that name's lane when
// one exists, and otherwise the shared slot — provided the shared slot holds
// that name. A shared workflow of another kind is left for guardKind to
// report, as it always has been.
func resolveGotoSlot(dataDir, command, kind, name string) (workflowSlot, error) {
	if name == "" {
		return sharedSlot(dataDir, kind, ""), nil
	}
	if !nameRegexp.MatchString(name) || len(name) > 64 {
		return workflowSlot{}, fmt.Errorf("name must match ^[a-z0-9_-]+$ and be at most 64 characters")
	}

	lane, err := workflow.ReadLane(dataDir, kind, name)
	if err != nil {
		return workflowSlot{}, err
	}
	if lane != nil {
		return laneSlot(dataDir, kind, name), nil
	}

	shared := sharedSlot(dataDir, kind, name)
	state, err := readState(shared.StatePath)
	if err != nil {
		return workflowSlot{}, err
	}
	if state != nil {
		sharedName, _ := state.Data["name"].(string)
		if state.Kind != "" && state.Kind != kind {
			return shared, nil
		}
		if sharedName == name {
			return shared, nil
		}
	}
	return workflowSlot{}, workflowNotFound(dataDir, command, kind, name, state)
}

// workflowNotFound refuses a goto naming a spec that has no kind workflow:
// no lane for it, and the shared record holds nothing or another spec. The
// message lists the workflows that are in progress, so the agent can see
// which name it meant.
func workflowNotFound(dataDir, command, kind, name string, shared *workflow.State) error {
	var inProgress []string
	if shared != nil && shared.InProgress() {
		sharedName, _ := shared.Data["name"].(string)
		inProgress = append(inProgress, fmt.Sprintf("%q (standalone, at step %q)", sharedName, shared.CurrentStep))
	}
	for _, laneName := range workflow.LaneNames(dataDir, kind) {
		if lane, err := workflow.ReadLane(dataDir, kind, laneName); err == nil && lane != nil && lane.InProgress() {
			inProgress = append(inProgress, fmt.Sprintf("%q (orchestrated, at step %q)", laneName, lane.CurrentStep))
		}
	}

	message := fmt.Sprintf("no %s workflow for %q is in progress", kind, name)
	if len(inProgress) > 0 {
		message += fmt.Sprintf("; %s workflows in progress: %s", kind, strings.Join(inProgress, ", "))
	}
	return output.NewError("workflow_not_found", message).
		WithResource(name).
		WithNextAction(fmt.Sprintf(
			`check the spec name; to drive one of the workflows in progress, re-run the goto with its name in "name"; to start a %s workflow for %q, run: %s %s new --data '{"name":%q}'`,
			kind, name, command, kind, name))
}

// refuseLaneInProgress stops a `new` in the shared slot for a spec whose lane
// is in progress: two workflows for one spec would overwrite each other's
// plan documents.
func refuseLaneInProgress(dataDir, command, kind, name string) error {
	lane, err := workflow.ReadLane(dataDir, kind, name)
	if err != nil || lane == nil || !lane.InProgress() {
		return err
	}
	if orchestrated, _ := lane.Data["orchestrated"].(bool); !orchestrated {
		return output.NewError("workflow_in_progress",
			fmt.Sprintf("a %s workflow for %q is in progress at step %q in its own lane (%s)",
				kind, name, lane.CurrentStep, workflow.LaneStateRel(kind, name))).
			WithResource(name).
			WithState(lane.CurrentStep, nil).
			WithNextAction(fmt.Sprintf(
				`it was started with implement.worktrees on; continue it with: %s %s goto --data '{"step":%q,"name":%q}'`,
				command, kind, lane.CurrentStep, name))
	}
	return output.NewError("workflow_in_progress",
		fmt.Sprintf("an orchestrated %s workflow for %q is in progress at step %q in its own lane (%s)",
			kind, name, lane.CurrentStep, workflow.LaneStateRel(kind, name))).
		WithResource(name).
		WithState(lane.CurrentStep, nil).
		WithNextAction(fmt.Sprintf(
			`an epic orchestrator started this workflow; continue it with: %s %s goto --data '{"step":%q,"name":%q}', or discard it and start it again with: %s %s new --force --data '{"name":%q,"orchestrated":true}'`,
			command, kind, lane.CurrentStep, name, command, kind, name))
}

// implementLaneStart decides whether an `implement new` keeps its workflow
// in a lane of its own. An orchestrated start always does. A start the user
// makes with worktrees on does too, so runs on different specs can be in
// progress side by side; it stays interactive, since lane is not
// orchestrated. A dry run, a start with no valid name, and a start with
// worktrees off use the shared slot as before.
func implementLaneStart(cfg config.Config, dataStr string, dryRun bool) (name string, lane, orchestrated bool, err error) {
	name, orchestrated, err = orchestratedStart(dataStr)
	if err != nil || orchestrated {
		return name, orchestrated, orchestrated, err
	}
	if dryRun || !cfg.Implement.Worktrees || dataStr == "" {
		return "", false, false, nil
	}
	var input struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(dataStr), &input) != nil || input.Name == "" || !nameRegexp.MatchString(input.Name) || len(input.Name) > 64 {
		return "", false, false, nil
	}
	return input.Name, true, false, nil
}

// inProgressLanes lists the in-progress lanes of kind, as resume commands, for
// a refusal that has no name to go on.
func inProgressLanes(dataDir, command, kind string) []string {
	var out []string
	for _, name := range workflow.LaneNames(dataDir, kind) {
		if lane, err := workflow.ReadLane(dataDir, kind, name); err == nil && lane != nil && lane.InProgress() {
			out = append(out, fmt.Sprintf(`%q at step %q (resume with: %s %s goto --data '{"step":%q,"name":%q}')`,
				name, lane.CurrentStep, command, kind, lane.CurrentStep, name))
		}
	}
	return out
}

// orchestratedStart reads a `new` command's --data for an orchestrated start.
// An orchestrated start probes only its own lane for a resume, so — unlike a
// standalone start, which can offer a resume before any name is given — it
// needs the spec name up front. Malformed --data reports not orchestrated and
// is left for the command's own parse to refuse.
func orchestratedStart(dataStr string) (name string, orchestrated bool, err error) {
	if dataStr == "" {
		return "", false, nil
	}
	var input struct {
		Name         string `json:"name"`
		Orchestrated bool   `json:"orchestrated"`
	}
	if json.Unmarshal([]byte(dataStr), &input) != nil || !input.Orchestrated {
		return "", false, nil
	}
	if input.Name == "" || !nameRegexp.MatchString(input.Name) || len(input.Name) > 64 {
		return "", false, fmt.Errorf("an orchestrated start needs a name matching ^[a-z0-9_-]+$ of at most 64 characters")
	}
	return input.Name, true, nil
}
