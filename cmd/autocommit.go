package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/autocommit"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/steps/implement"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// autoCommitGit is the Git the automatic-commit path runs through; a
// variable so tests can swap in a fake, mirroring repoGit.
var autoCommitGit autocommit.Git = autocommit.NewGit()

// gotoWithAutoCommit runs a workflow transition, making an automatic git
// commit when the transition is a commit point.
//
// Off a commit point — which includes every transition when auto_commit is
// off, and every dry run — it behaves exactly as the handlers did before
// automatic commits existed: build the workflow, transition, write whatever
// the step produced.
//
// At a commit point it wraps that in a small sequence, because the commit
// has to happen after the state file is written and yet must not leave the
// workflow advanced if git refuses:
//
//  1. read and validate the staged message, and remove it so it never lands
//     in the commit itself;
//  2. snapshot the state file;
//  3. buffer the step's output rather than writing it straight out;
//  4. transition;
//  5. commit every dirty target;
//  6. on success flush the buffer; on failure restore the snapshot, drop the
//     buffer, and report which repo failed and why.
//
// Committing after the state save matters: state.json and
// working-context.md are tracked in git, so committing from inside a step
// callback — which runs before the state is saved — would leave the tree
// dirty after every commit, and the next workflow would always report
// uncommitted changes.
func gotoWithAutoCommit(
	cmd *cobra.Command,
	cfg config.Config,
	root, statePath, kind string,
	steps []workflow.StepConfig,
	wfCfg workflow.Config,
	input map[string]any,
	stepVal string,
	missingNameErr string,
) error {
	// Pull the message path out before the SetData loop below, so it is
	// never copied into persisted workflow data.
	msgPath, _ := input["commit_message_from"].(string)
	delete(input, "commit_message_from")

	// Output is buffered on every path so the commit path can discard it on
	// failure. On the ordinary path it is flushed immediately, which writes
	// the same bytes the writer would have written directly.
	var buf bytes.Buffer
	out := output.New(&buf, globalFields)
	wf := workflow.New(steps, statePath, wfCfg, store.NewSourceStore(root, "project"), out)

	point := autocommit.PointFor(wfCfg.AutoCommit, kind, wf.Current(), stepVal)
	if wfCfg.DryRun {
		point = autocommit.PointNone
	}

	for k, v := range input {
		if k != "step" {
			wf.SetData(k, v)
		}
	}
	// Plan and implement refuse a goto with no workflow in progress; spec
	// passes an empty message and skips the check.
	if missingNameErr != "" {
		if _, ok := wf.GetData("name"); !ok {
			return fmt.Errorf("%s", missingNameErr)
		}
	}

	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	// An orchestrated workflow is a lane, whose files are removed once it
	// finishes: without them, status falls back to the documents, which say
	// the same thing, and finished lanes never pile up in the tree.
	orchestrated := isOrchestrated(wf) && !wfCfg.DryRun
	finishLane := func() {
		if orchestrated && wf.Current() == "finished" {
			removeLane(statePath)
		}
	}

	if point == autocommit.PointNone {
		err := wf.Goto(stepVal)
		if err == nil {
			finishLane()
		}
		flushBuffer(cmd, &buf)
		return err
	}

	specName := workflowName(wf)

	// A milestone point is only a candidate. Most phase wrap-ups land inside
	// a milestone, where nothing is due and the transition must behave
	// exactly as it does with automatic commits off — no message asked for,
	// and any message the agent staged anyway left untouched.
	//
	// An implement completion commit in full mode also covers any milestone
	// its run just finished: a single-task run that ends early can tick the
	// last task of a milestone, and that milestone is recorded here rather
	// than asked for again by a later run.
	var due []int
	if point == autocommit.PointMilestone ||
		(point == autocommit.PointCompletion && kind == "implement" && wfCfg.AutoCommit == config.AutoCommitFull) {
		milestones, err := dueMilestones(cfg, root, wf, specName)
		if err != nil {
			return err
		}
		due = milestones
		if point == autocommit.PointMilestone && len(due) == 0 {
			err := wf.Goto(stepVal)
			if err == nil {
				finishLane()
			}
			flushBuffer(cmd, &buf)
			return err
		}
	}

	message, err := readCommitMessage(root, msgPath, cfg.Command, kind, stepVal, specName)
	if err != nil {
		return err
	}
	if err := autocommit.ValidateMessage(message, specName, due); err != nil {
		return withCommitNextAction(err, cfg.Command, kind, stepVal, specName)
	}
	// Record the milestones this commit covers before transitioning, so the
	// state save carries them and a rollback forgets them along with the
	// step — a retry then asks for the same milestones again.
	if len(due) > 0 {
		wf.SetData(committedMilestonesKey, mergeMilestones(committedMilestones(wf), due))
	}
	// Remove the staged message before transitioning, so it is not itself
	// swept into the commit it describes.
	if abs, ok := commitMessagePath(root, msgPath); ok {
		_ = os.Remove(abs)
	}

	// The snapshot is of this workflow's own state file — a lane's when it
	// is orchestrated — so a failed commit can never roll back another
	// workflow. A lane's notes are snapshotted too, since finishing removes
	// them.
	snapshot, snapshotErr := os.ReadFile(statePath)
	notesPath := laneNotesFor(statePath)
	notesSnapshot, notesErr := os.ReadFile(notesPath)

	if err := wf.Goto(stepVal); err != nil {
		flushBuffer(cmd, &buf)
		return err
	}
	// Removed before the commit, so a plan lane's deletion is part of it.
	finishLane()

	restore := func() {
		if snapshotErr == nil {
			_ = os.WriteFile(statePath, snapshot, 0o644)
		}
		if orchestrated && notesErr == nil {
			_ = os.WriteFile(notesPath, notesSnapshot, 0o644)
		}
	}

	// A plan lane runs in the project's working copy beside other plan
	// lanes, so it commits only its own files, under the project commit
	// lock. Every other workflow commits its whole tree as before.
	if orchestrated && kind == "plan" {
		if err := commitPlanLane(cfg, root, statePath, specName, message); err != nil {
			restore()
			return output.NewError("auto_commit_failed", err.Error()).
				WithResource("project").
				WithNextAction(commitRetryAction(cfg.Command, kind, stepVal, specName))
		}
		flushBuffer(cmd, &buf)
		return nil
	}

	targets, err := autocommit.Targets(cfg, root, autoCommitGit)
	if err != nil {
		restore()
		return output.NewError("auto_commit_failed", err.Error()).
			WithNextAction(commitRetryAction(cfg.Command, kind, stepVal, specName))
	}
	if _, err := autocommit.CommitDirty(targets, message, autoCommitGit); err != nil {
		restore()
		er := output.NewError("auto_commit_failed", err.Error()).
			WithNextAction(commitRetryAction(cfg.Command, kind, stepVal, specName))
		var commitErr *autocommit.CommitError
		if errors.As(err, &commitErr) {
			er = er.WithResource(strings.Join(commitErr.Target.Repos, ", "))
		}
		return er
	}

	flushBuffer(cmd, &buf)
	return nil
}

// isOrchestrated reports whether the workflow was started by an epic
// orchestrator, and so is a lane.
func isOrchestrated(wf *workflow.Workflow) bool {
	v, _ := wf.GetData("orchestrated")
	b, _ := v.(bool)
	return b
}

// laneNotesFor is the notes file beside a lane's state file.
func laneNotesFor(statePath string) string {
	return strings.TrimSuffix(statePath, ".json") + ".md"
}

// removeLane deletes a finished lane's state and notes files.
func removeLane(statePath string) {
	_ = os.Remove(statePath)
	_ = os.Remove(laneNotesFor(statePath))
}

// commitPlanLane commits a finished plan lane's own files and nothing else:
// the plan's store directory, its per-section working files, its scratch
// folder and its lane files. Everything another plan running beside it has
// changed stays uncommitted for that plan's own commit. The commit is taken
// under the project commit lock, so lanes finishing together queue rather
// than contend for git's index.
func commitPlanLane(cfg config.Config, root, statePath, name, message string) error {
	release, err := autocommit.AcquireLock(filepath.Join(root, ".spektacular"))
	if err != nil {
		return err
	}
	defer release()

	top, ok, err := autoCommitGit.TopLevel(root)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	paths := []string{
		filepath.Join(root, filepath.Dir(implement.PlanFilePath(cfg.Plan.Config.Directory, name))),
		filepath.Join(root, ".spektacular", "work", name),
		filepath.Join(root, ".spektacular", "tmp", name),
		statePath,
		laneNotesFor(statePath),
	}
	return autoCommitGit.CommitPaths(top, paths, message)
}

// startGate is the shared uncommitted-changes check the three `new` commands
// run before anything is written — including before the stale state file is
// removed, so a refusal really does leave the project untouched.
//
// It is silent when automatic commits are off, on a dry run, or when every
// registered repo is clean. Otherwise the user's own uncommitted work is
// about to be swept into the workflow's commits, so the agent is sent an
// uncommitted_changes report to put to the user, and re-runs `new` carrying
// their answer as commit_existing: true commits that work first, under a
// message saying it is theirs and predates the workflow; false goes ahead and
// lets the workflow's own commits include it.
//
// dataStr is the raw --data JSON, used both to read the answer and to echo
// the caller's own arguments back in the re-run commands.
func startGate(cfg config.Config, root, kind, specName, dataStr string, dryRun bool) error {
	if dryRun || cfg.AutoCommitMode() == config.AutoCommitOff {
		return nil
	}

	targets, err := autocommit.Targets(cfg, root, autoCommitGit)
	if err != nil {
		return err
	}
	dirty, err := autocommit.DirtyTargets(targets, autoCommitGit)
	if err != nil {
		return err
	}
	if len(dirty) == 0 {
		return nil
	}

	choice, answered := commitExistingAnswer(dataStr)
	if !answered {
		return output.NewError("uncommitted_changes",
			"uncommitted changes in registered repos: "+describeTargets(dirty)).
			WithResource(targetNames(dirty)).
			WithNextAction(fmt.Sprintf(
				"ask the user whether to git commit these changes before the %s workflow starts; then run: %s to commit them first, or %s to continue without committing",
				kind,
				newCommandWith(cfg.Command, kind, dataStr, true),
				newCommandWith(cfg.Command, kind, dataStr, false)))
	}
	if !choice {
		return nil
	}

	if _, err := autocommit.CommitDirty(dirty, autocommit.PreWorkflowMessage(kind, specName), autoCommitGit); err != nil {
		er := output.NewError("auto_commit_failed", err.Error()).
			WithNextAction(fmt.Sprintf(
				"fix the cause git reported above (a failing hook, for example), then re-run: %s",
				newCommandWith(cfg.Command, kind, dataStr, true)))
		var commitErr *autocommit.CommitError
		if errors.As(err, &commitErr) {
			er = er.WithResource(strings.Join(commitErr.Target.Repos, ", "))
		}
		return er
	}
	return nil
}

// commitExistingAnswer reads the user's answer to the uncommitted-changes
// question out of the raw --data. The second return is false when the key is
// absent, which is what distinguishes "not asked yet" from an explicit no.
func commitExistingAnswer(dataStr string) (choice, answered bool) {
	if dataStr == "" {
		return false, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(dataStr), &m); err != nil {
		return false, false
	}
	v, ok := m["commit_existing"]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// newCommandWith renders the `new` invocation the caller just made, with the
// uncommitted-changes answer added, so the agent can re-run it verbatim.
func newCommandWith(command, kind, dataStr string, choice bool) string {
	m := map[string]any{}
	if dataStr != "" {
		_ = json.Unmarshal([]byte(dataStr), &m)
	}
	m["commit_existing"] = choice
	// Marshal sorts map keys, so the rendered command is stable.
	encoded, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("%s %s new", command, kind)
	}
	return fmt.Sprintf("%s %s new --data '%s'", command, kind, encoded)
}

// describeTargets renders each work tree as "name (dir)", naming every repo
// registered against a shared tree.
func describeTargets(targets []autocommit.Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, fmt.Sprintf("%s (%s)", strings.Join(t.Repos, ", "), t.Dir))
	}
	return strings.Join(parts, ", ")
}

// targetNames renders just the registered repo names.
func targetNames(targets []autocommit.Target) string {
	var names []string
	for _, t := range targets {
		names = append(names, t.Repos...)
	}
	return strings.Join(names, ", ")
}

// committedMilestonesKey is the workflow-data key recording which plan
// milestones have already been committed, so none is committed twice.
const committedMilestonesKey = "committed_milestones"

// dueMilestones reports the milestones whose work items are now all ticked and
// that have not been committed yet. It reads the plan through the store, the
// same way the implement steps reach it.
func dueMilestones(cfg config.Config, root string, wf *workflow.Workflow, name string) ([]int, error) {
	st := store.NewSourceStore(root, "project")
	body, err := st.Read(implement.PlanFilePath(cfg.Plan.Config.Directory, name))
	if err != nil {
		return nil, err
	}
	return autocommit.DueMilestones(
		autocommit.CompletedMilestones(string(body)),
		committedMilestones(wf),
	), nil
}

// committedMilestones reads the recorded milestone numbers out of workflow
// data. They come back from JSON as float64, so they are normalised here.
func committedMilestones(wf *workflow.Workflow) []int {
	v, ok := wf.GetData(committedMilestonesKey)
	if !ok {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		switch n := item.(type) {
		case float64:
			out = append(out, int(n))
		case int:
			out = append(out, n)
		}
	}
	return out
}

// mergeMilestones unions two milestone sets, ascending.
func mergeMilestones(existing, added []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, n := range append(append([]int{}, existing...), added...) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// flushBuffer copies the buffered step output to the command's stdout.
func flushBuffer(cmd *cobra.Command, buf *bytes.Buffer) {
	if buf.Len() > 0 {
		_, _ = cmd.OutOrStdout().Write(buf.Bytes())
	}
}

// workflowName returns the workflow's instance name — the spec name for a
// spec run, and the plan name, which is the same name, for plan and
// implement runs.
func workflowName(wf *workflow.Workflow) string {
	v, ok := wf.GetData("name")
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// commitMessagePath resolves a project-relative message path to an absolute
// one, refusing anything that escapes the project root.
func commitMessagePath(root, rel string) (string, bool) {
	if rel == "" {
		return "", false
	}
	abs := rel
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, rel)
	}
	abs = filepath.Clean(abs)
	within, err := filepath.Rel(root, abs)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

// readCommitMessage reads the staged commit message, refusing with
// commit_message_required when no usable path was given or the file is
// missing or empty.
func readCommitMessage(root, rel, command, kind, step, name string) (string, error) {
	missing := func(reason string) error {
		return output.NewError("commit_message_required", reason).
			WithResource(step).
			WithNextAction(commitMessageAction(command, kind, step, name))
	}
	if rel == "" {
		return "", missing("advancing to this step makes a git commit, and no commit message was supplied")
	}
	abs, ok := commitMessagePath(root, rel)
	if !ok {
		return "", missing(fmt.Sprintf("the commit message path %q is outside the project", rel))
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return "", missing(fmt.Sprintf("the commit message file %q could not be read: %v", rel, err))
	}
	if strings.TrimSpace(string(body)) == "" {
		return "", missing(fmt.Sprintf("the commit message file %q is empty", rel))
	}
	return string(body), nil
}

// commitMessageAction is the remediation for a missing or invalid message:
// stage it, then re-run the same goto carrying it.
func commitMessageAction(command, kind, step, name string) string {
	tmp := autocommit.MessageTmpPath(name)
	return fmt.Sprintf("write the git commit message to %s, then run: %s %s goto --data '%s'",
		tmp, command, kind, commitGotoData(kind, step, name, tmp))
}

// commitRetryAction is the remediation for a commit git refused: fix what it
// reported, re-stage the message, and re-run the identical command. The
// workflow is back on its previous step, so the retry repeats the transition.
func commitRetryAction(command, kind, step, name string) string {
	tmp := autocommit.MessageTmpPath(name)
	return fmt.Sprintf(
		"fix the cause git reported above (a failing hook, for example), re-stage the message at %s, then re-run: %s %s goto --data '%s'",
		tmp, command, kind, commitGotoData(kind, step, name, tmp))
}

// commitGotoData renders the --data of a goto carrying a commit message. Plan
// and implement gotos carry the spec name, which routes them to the right
// workflow; a spec goto takes none.
func commitGotoData(kind, step, name, tmp string) string {
	if name != "" && (kind == "plan" || kind == "implement") {
		return fmt.Sprintf(`{"step":%q,"name":%q,"commit_message_from":%q}`, step, name, tmp)
	}
	return fmt.Sprintf(`{"step":%q,"commit_message_from":%q}`, step, tmp)
}

// withCommitNextAction attaches the stage-and-retry remediation to a message
// validation failure from the engine, which carries only a code and message
// because only the command layer knows the goto to re-run.
func withCommitNextAction(err error, command, kind, step, name string) error {
	var er *output.ErrorResponse
	if !errors.As(err, &er) {
		return err
	}
	return er.WithResource(step).WithNextAction(commitMessageAction(command, kind, step, name))
}
