package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// artifactStatusResult is the per-artifact status shape. Two timestamps are
// deliberately kept apart because they answer different questions:
//
//   - updated_at is workflow activity: the in-progress state's UpdatedAt,
//     present only when that state belongs to this artifact. It is omitted
//     otherwise, so absence means "no live workflow", never a guess.
//   - modified_at is the store's modification time for the artifact. A
//     checkout, a reformat or a stray touch all move it without anything
//     having happened, so a poller must not read it as progress. It is
//     omitted when the backend cannot report one.
type artifactStatusResult struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	ArtifactID     string   `json:"artifact_id"`
	DocumentStatus string   `json:"document_status"`
	CurrentStep    string   `json:"current_step"`
	CompletedSteps []string `json:"completed_steps"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	ModifiedAt     string   `json:"modified_at,omitempty"`
	ClosedAt       string   `json:"closed_at"`
	Spec           string   `json:"spec"`
	Plan           string   `json:"plan"`

	// Progress and Tasks report a task-format plan's work, task by task. They
	// are absent for specs and for plans without task structure.
	Progress *taskProgress `json:"progress,omitempty"`
	Tasks    []taskStatus  `json:"tasks,omitempty"`
}

// taskProgress totals a plan's tasks.
type taskProgress struct {
	TasksCompleted int `json:"tasks_completed"`
	TasksTotal     int `json:"tasks_total"`
}

// taskStatus is one task's progress. Completion (the heading checkbox) and
// acceptance criteria are separate facts: a task can be completed with a
// criterion unmet, and that must stay visible.
type taskStatus struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Milestone          int               `json:"milestone"`
	Completed          bool              `json:"completed"`
	AcceptanceCriteria plantask.Criteria `json:"acceptance_criteria"`
}

var artifactStatusOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"kind":            {Type: "string"},
		"name":            {Type: "string"},
		"artifact_id":     {Type: "string"},
		"document_status": {Type: "string"},
		"current_step":    {Type: "string"},
		"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
		"created_at":      {Type: "string"},
		"updated_at":      {Type: "string"},
		"modified_at":     {Type: "string"},
		"closed_at":       {Type: "string"},
		"spec":            {Type: "string"},
		"plan":            {Type: "string"},
	},
}

// planArtifactStatusOutputSchema is the named plan status schema: the shared
// artifact fields plus the optional per-task progress.
var planArtifactStatusOutputSchema = func() *schemaObj {
	props := make(map[string]*schemaProp, len(artifactStatusOutputSchema.Properties)+2)
	for k, v := range artifactStatusOutputSchema.Properties {
		props[k] = v
	}
	props["progress"] = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
		"tasks_completed": {Type: "integer"},
		"tasks_total":     {Type: "integer"},
	}}
	props["tasks"] = &schemaProp{Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
		"id":        {Type: "string"},
		"title":     {Type: "string"},
		"milestone": {Type: "integer"},
		"completed": {Type: "boolean"},
		"acceptance_criteria": {Type: "object", Properties: map[string]*schemaProp{
			"met":   {Type: "integer"},
			"total": {Type: "integer"},
		}},
	}}}
	return &schemaObj{Type: "object", Properties: props}
}()

type artifactStatusHook func(*metadata.Metadata) metadata.DocumentStatus

// artifactBodyHook adds kind-specific facts read from an artifact's body to
// its status. Only plans set one.
type artifactBodyHook func(body []byte, r *artifactStatusResult)

// planTaskProgress fills a task-format plan's progress and per-task list.
func planTaskProgress(body []byte, r *artifactStatusResult) {
	p := plantask.Parse(body)
	if p.Format != plantask.FormatTasks {
		return
	}
	r.Progress = &taskProgress{TasksTotal: len(p.Tasks)}
	r.Tasks = make([]taskStatus, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		if t.Completed {
			r.Progress.TasksCompleted++
		}
		r.Tasks = append(r.Tasks, taskStatus{
			ID:                 t.ID,
			Title:              t.Title,
			Milestone:          t.Milestone,
			Completed:          t.Completed,
			AcceptanceCriteria: t.Criteria,
		})
	}
}

// resolveDocumentStatus is an artifact's reported document status: the
// stored one, as adjusted by hook when the kind has one. Every command that
// reports a document status goes through it, so they always agree.
func resolveDocumentStatus(fm *metadata.Metadata, hook artifactStatusHook) metadata.DocumentStatus {
	if fm == nil {
		return ""
	}
	if hook != nil {
		return hook(fm)
	}
	return fm.DocumentStatus
}

func runArtifactStatus(cmd *cobra.Command, kind, name, storePath, statePath, command string, steps []workflow.StepConfig, st store.Store, statusHook artifactStatusHook, bodyHook artifactBodyHook) error {
	raw, err := st.Read(storePath)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return output.NewError("artifact_not_found", fmt.Sprintf("%s artifact %q was not found", kind, name)).
				WithResource(name).
				WithNextAction(fmt.Sprintf("run `%s %s file list` to see available %ss", command, kind, kind))
		}
		return err
	}

	fm, body, err := metadata.Split(raw)
	if err != nil {
		return output.NewError("metadata_read_failed", fmt.Sprintf("could not read metadata for %s artifact %q: %v", kind, name, err)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("repair the artifact frontmatter, then re-run `%s %s status %s`", command, kind, name))
	}

	result := artifactStatusResult{
		Kind:           kind,
		Name:           name,
		ArtifactID:     name,
		CompletedSteps: []string{},
	}
	if fm != nil {
		result.DocumentStatus = string(resolveDocumentStatus(fm, statusHook))
		result.CreatedAt = dateAsRFC3339(fm.CreatedDate)
		result.ClosedAt = dateAsRFC3339(fm.ClosedDate)
		result.Spec = fm.Spec
		result.Plan = fm.Plan
	}

	if bodyHook != nil {
		bodyHook(body, &result)
	}

	// modified_at comes from the store regardless of workflow state: it is a
	// fact about the stored bytes, not about progress.
	result.ModifiedAt = timestampAsRFC3339(artifactModTime(st, storePath))

	wf := workflow.New(steps, statePath, workflow.Config{}, nil, nil)
	state := wf.State()
	if state.InProgress() && state.Kind == kind && fmt.Sprintf("%v", state.Data["name"]) == name {
		result.CurrentStep = state.CurrentStep
		result.CompletedSteps = append([]string(nil), state.CompletedSteps...)
		result.UpdatedAt = timestampAsRFC3339(state.UpdatedAt)
	} else if result.DocumentStatus == string(metadata.StatusStale) {
		result.CurrentStep = "stale"
	} else if fm != nil && isClosedDocumentStatus(metadata.DocumentStatus(result.DocumentStatus)) {
		result.CurrentStep = "finished"
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(result)
}

func dateAsRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
}

func timestampAsRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func isClosedDocumentStatus(s metadata.DocumentStatus) bool {
	return s == metadata.StatusFinal || s == metadata.StatusSuperseded || s == metadata.StatusArchived
}

// artifactModTime asks the store when the artifact last changed. A backend
// that cannot report a timestamp leaves it zero, and so does a Stat failure:
// the artifact was already read successfully, so a failing Stat is a
// transient race rather than a reason to fail the status call.
func artifactModTime(st store.Store, storePath string) time.Time {
	info, err := st.Stat(storePath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime
}
