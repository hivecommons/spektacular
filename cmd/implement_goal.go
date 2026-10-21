package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/gitexec"
	"github.com/hivecommons/spektacular/internal/goal"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/steps/implement"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// goalNow is the clock every recorded check and decision is stamped with; a
// test may pin it.
var goalNow = time.Now

var implementGoalCmd = &cobra.Command{
	Use:   "goal",
	Short: "Record, decide and show the implement workflow's goal checks",
	Long: `The goal check is the implement step, check_goal, in which a separate
read-only reviewer judges each acceptance criterion of the current task (or
legacy phase) as met, not_met or impossible, with evidence. These commands
record the reviewer's verdicts and the user's decisions, and read the outcome
back. The decision of what happens next is taken here, not by the agent.`,
}

var implementGoalRecordInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the plan whose item was checked"},
		"item": {Type: "string", Description: "the checked item: a task id, or a legacy phase's \"Phase N.M\" label"},
	},
	Required: []string{"name", "item"},
}

var implementGoalDecideInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":     {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the plan whose item stopped"},
		"item":     {Type: "string", Description: "the item: a task id, or a legacy phase's \"Phase N.M\" label"},
		"decision": {Type: "string", Enum: goal.UserDecisions, Description: "the user's answer to the stopped check"},
		"reason":   {Type: "string", Description: "optional: why the user decided so"},
	},
	Required: []string{"name", "item", "decision"},
}

var implementGoalResultSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"plan":           {Type: "string"},
		"item":           {Type: "string"},
		"round":          {Type: "integer"},
		"attempt":        {Type: "integer"},
		"decision":       {Type: "string"},
		"state":          {Type: "string"},
		"retries_left":   {Type: "integer"},
		"met":            {Type: "array", Items: &schemaProp{Type: "integer"}},
		"not_met":        {Type: "array", Items: &schemaProp{Type: "integer"}},
		"impossible":     {Type: "array", Items: &schemaProp{Type: "integer"}},
		"tree_unchanged": {Type: "boolean"},
		"warning":        {Type: "string"},
		"next_step":      {Type: "string"},
		"next_action":    {Type: "string"},
	},
}

var implementGoalRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record one goal check from the reviewer's staged reply",
	Long: `Records the reviewer's verdicts for the current item from the reply staged
with --from, exactly as the reviewer returned it. The reply must hold exactly
one verdict (met, not_met or impossible) with non-empty evidence for every
criterion. Answers with the decision taken and the exact next command.`,
	Args: cobra.NoArgs,
	RunE: runImplementGoalRecord,
}

var implementGoalDecideCmd = &cobra.Command{
	Use:   "decide",
	Short: "Record the user's decision after a goal check stopped the run",
	Args:  cobra.NoArgs,
	RunE:  runImplementGoalDecide,
}

var implementGoalShowCmd = &cobra.Command{
	Use:   "show <plan>",
	Short: "Show a plan's recorded goals, verdicts, evidence and attempt counts",
	Args:  cobra.ExactArgs(1),
	RunE:  runImplementGoalShow,
}

// goalResult is what record and decide answer with.
type goalResult struct {
	Plan          string `json:"plan"`
	Item          string `json:"item"`
	Round         int    `json:"round"`
	Attempt       int    `json:"attempt"`
	Decision      string `json:"decision"`
	State         string `json:"state"`
	RetriesLeft   int    `json:"retries_left"`
	Met           []int  `json:"met"`
	NotMet        []int  `json:"not_met"`
	Impossible    []int  `json:"impossible"`
	TreeUnchanged *bool  `json:"tree_unchanged,omitempty"`
	Warning       string `json:"warning,omitempty"`
	NextStep      string `json:"next_step"`
	NextAction    string `json:"next_action"`
}

// goalProject is what every goal verb reads before it acts.
type goalProject struct {
	cfg   config.Config
	root  string
	store store.Store
}

func loadGoalProject() (goalProject, error) {
	cfg, err := loadConfig()
	if err != nil {
		return goalProject{}, err
	}
	root, err := projectRoot()
	if err != nil {
		return goalProject{}, err
	}
	return goalProject{cfg: cfg, root: root, store: store.NewSourceStore(root, "project")}, nil
}

func (p goalProject) refuseDisabled() error {
	if p.cfg.Implement.GoalCheck.Enabled {
		return nil
	}
	return output.NewError("goal_check_disabled", "the goal check is switched off for this project (implement.goal_check.enabled is false)").
		WithResource("implement.goal_check.enabled").
		WithNextAction("set implement.goal_check.enabled: true in .spektacular/config.yaml to switch the goal check on")
}

func (p goalProject) repoNames() []string {
	return repoNames(p.cfg)
}

// planDocs reads the plan and its context document. A plan without a context
// document is read with none.
func (p goalProject) planDocs(name string) (plan, context []byte, err error) {
	dir := p.cfg.Plan.Config.Directory
	plan, err = p.store.Read(implement.PlanFilePath(dir, name))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, output.NewError("plan_not_found", fmt.Sprintf("there is no plan named %q", name)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("check the name with `%s plan file list`", p.cfg.Command))
	}
	if err != nil {
		return nil, nil, err
	}
	context, err = p.store.Read(implement.ContextFilePath(dir, name))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, nil, err
	}
	return plan, context, nil
}

// runSelectedTask is the task an implement run for the plan is scoped to, ""
// for a whole-plan run or when no implement run for the plan is in progress.
func runSelectedTask(root, plan string) string {
	dataDir := filepath.Join(root, ".spektacular")
	if lane, err := workflow.ReadLane(dataDir, "implement", plan); err == nil && lane != nil {
		task, _ := lane.Data["task"].(string)
		return task
	}
	if st, err := readState(stateFilePath(dataDir)); err == nil && st != nil && st.Kind == "implement" {
		if name, _ := st.Data["name"].(string); name == plan {
			task, _ := st.Data["task"].(string)
			return task
		}
	}
	return ""
}

type goalInput struct {
	Name     string `json:"name"`
	Item     string `json:"item"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func parseGoalInput(cmd *cobra.Command, shape string) (goalInput, error) {
	var in goalInput
	dataStr, _ := cmd.Flags().GetString("data")
	if err := json.Unmarshal([]byte(dataStr), &in); err != nil || !nameRegexp.MatchString(in.Name) || len(in.Name) > 64 || strings.TrimSpace(in.Item) == "" {
		msg := "--data must be a JSON object naming the plan and the item"
		if err != nil {
			msg = fmt.Sprintf("parsing --data: %v", err)
		}
		return in, output.NewError("bad_input", msg).WithNextAction("reissue with --data '" + shape + "'")
	}
	return in, nil
}

func goalGoto(command, plan, step string) string {
	return fmt.Sprintf(`%s implement goto --data '{"step":"%s","name":"%s"}'`, command, step, plan)
}

func goalDecideCommand(command, plan, item string) string {
	return fmt.Sprintf(`%s implement goal decide --data '{"name":"%s","item":"%s","decision":"keep_fixing|accept|skip|recheck"}'`, command, plan, item)
}

func runImplementGoalRecord(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  implementGoalRecordInputSchema,
			Output: implementGoalResultSchema,
			Flags:  map[string]*schemaProp{"from": {Type: "string", Description: "the reviewer's reply, staged unchanged under .spektacular/tmp/<plan>/goal-check/"}},
		}, "")
	}
	in, err := parseGoalInput(cmd, `{"name":"<plan>","item":"<task id | Phase N.M>"}`)
	if err != nil {
		return err
	}
	p, err := loadGoalProject()
	if err != nil {
		return err
	}
	if err := p.refuseDisabled(); err != nil {
		return err
	}
	stageDir := goalStageDir(in.Name)
	fromPath, _ := cmd.Flags().GetString("from")
	if fromPath == "" {
		return output.NewError("from_required", "record needs the reviewer's reply, staged in a file").
			WithResource(in.Item).
			WithNextAction(fmt.Sprintf(`write the reviewer's reply unchanged to %s/reply.json and reissue with --from %s/reply.json`, stageDir, stageDir))
	}
	raw, err := os.ReadFile(fromPath)
	if err != nil {
		return output.NewError("from_unreadable", fmt.Sprintf("cannot read the staged reply %s: %v", fromPath, err)).
			WithResource(fromPath).
			WithNextAction(fmt.Sprintf("write the reviewer's reply unchanged to %s/reply.json and pass that path with --from", stageDir))
	}

	planBody, contextBody, err := p.planDocs(in.Name)
	if err != nil {
		return err
	}
	g, err := goal.Build(planBody, contextBody, in.Item, p.repoNames())
	if err != nil {
		return withGoalPlan(err, p.cfg.Command, in.Name)
	}
	planDir := p.cfg.Plan.Config.Directory
	outcome, err := goal.Load(p.store, planDir, in.Name)
	if err != nil {
		return err
	}
	if current, ok := goal.Current(plantask.Parse(planBody), outcome, runSelectedTask(p.root, in.Name)); !ok || current != in.Item {
		msg := fmt.Sprintf("%s is not the run's current item", in.Item)
		if ok {
			msg = fmt.Sprintf("%s is not the run's current item; the current item is %s", in.Item, current)
		}
		return output.NewError("goal_item_not_current", msg).
			WithResource(in.Item).
			WithNextAction(fmt.Sprintf("record the check of the current item, as the check_goal step names it: %s", goalGoto(p.cfg.Command, in.Name, "check_goal")))
	}

	verdicts, err := goal.ValidateReply(g, raw)
	if err != nil {
		var oe *output.ErrorResponse
		if errors.As(err, &oe) && len(outcome.Item(in.Item).Checks) > 0 {
			if last := outcome.Item(in.Item).Checks; len(last[len(last)-1].Goal.Criteria) != len(g.Criteria) {
				oe.NextAction += fmt.Sprintf("; the plan's criteria for %s changed since the last check, so judge the goal the check_goal step renders now", in.Item)
			}
		}
		return err
	}
	check, err := goal.NewCheck(outcome.Item(in.Item), g, verdicts)
	if err != nil {
		return withGoalPlan(err, p.cfg.Command, in.Name)
	}
	unchanged, warning := goalTreeUnchanged(p, in.Name)
	check.TreeUnchanged = unchanged
	it, err := goal.AppendCheck(p.store, planDir, in.Name, check, goalNow())
	if err != nil {
		return err
	}

	res := checkResult(in.Name, in.Item, check)
	res.State = it.State
	res.TreeUnchanged = &unchanged
	res.Warning = warning
	switch check.Decision {
	case goal.CheckAccept, goal.CheckRetry:
		res.NextAction = goalGoto(p.cfg.Command, in.Name, res.NextStep)
	default:
		res.NextAction = fmt.Sprintf("STOP and ask the user whether to keep fixing, accept the shortfall, skip the task or (after correcting a criterion in the plan) recheck; record the answer with %s, then run the goto it returns",
			goalDecideCommand(p.cfg.Command, in.Name, in.Item))
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
}

func checkResult(plan, item string, c goal.Check) goalResult {
	return goalResult{
		Plan:        plan,
		Item:        item,
		Round:       c.Round,
		Attempt:     c.Attempt,
		Decision:    c.Decision,
		RetriesLeft: goal.RetriesLeft(c),
		Met:         goal.CriteriaWith(c, goal.VerdictMet),
		NotMet:      goal.CriteriaWith(c, goal.VerdictNotMet),
		Impossible:  goal.CriteriaWith(c, goal.VerdictImpossible),
		NextStep:    goal.NextStep(c.Decision),
	}
}

// withGoalPlan fills the "<plan>" placeholder the goal package leaves in a
// refusal's next action, and the configured command name.
func withGoalPlan(err error, command, plan string) error {
	var oe *output.ErrorResponse
	if errors.As(err, &oe) {
		oe.NextAction = strings.ReplaceAll(oe.NextAction, "<plan>", plan)
		oe.NextAction = strings.ReplaceAll(oe.NextAction, "spektacular ", command+" ")
	}
	return err
}

func runImplementGoalDecide(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: implementGoalDecideInputSchema, Output: implementGoalResultSchema}, "")
	}
	in, err := parseGoalInput(cmd, `{"name":"<plan>","item":"<task id | Phase N.M>","decision":"keep_fixing|accept|skip|recheck"}`)
	if err != nil {
		return err
	}
	p, err := loadGoalProject()
	if err != nil {
		return err
	}
	if err := p.refuseDisabled(); err != nil {
		return err
	}
	planBody, contextBody, err := p.planDocs(in.Name)
	if err != nil {
		return err
	}
	if _, err := goal.Build(planBody, contextBody, in.Item, p.repoNames()); err != nil {
		return withGoalPlan(err, p.cfg.Command, in.Name)
	}
	it, next, err := goal.AppendDecision(p.store, p.cfg.Plan.Config.Directory, in.Name, in.Item,
		goal.Decision{Decision: in.Decision, Reason: strings.TrimSpace(in.Reason)}, goalNow())
	if err != nil {
		return withGoalPlan(err, p.cfg.Command, in.Name)
	}
	last := it.Checks[len(it.Checks)-1]
	res := checkResult(in.Name, in.Item, last)
	res.Decision = in.Decision
	res.State = it.State
	res.NextStep = next
	res.NextAction = goalGoto(p.cfg.Command, in.Name, next)
	// Round and attempt stay those of the check that stopped; a keep_fixing or
	// recheck answer opens a new round whose first check has MaxRetries
	// retries after it, and accept or skip leave none.
	res.RetriesLeft = 0
	if in.Decision == goal.DecideKeepFixing || in.Decision == goal.DecideRecheck {
		res.RetriesLeft = goal.MaxRetries
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
}

func runImplementGoalShow(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"plan":  {Type: "string"},
				"items": {Type: "array", Items: &schemaProp{Type: "object"}},
			}},
			Flags: map[string]*schemaProp{
				"item":   {Type: "string", Description: "show one item only: a task id or \"Phase N.M\""},
				"format": {Type: "string", Enum: []string{statusFormatPretty, statusFormatJSON}},
			},
		}, "")
	}
	name := args[0]
	format, _ := cmd.Flags().GetString("format")
	if format != statusFormatPretty && format != statusFormatJSON {
		return output.NewError("goal_format_unsupported",
			fmt.Sprintf("format %q is not supported; supported formats: %s, %s", format, statusFormatPretty, statusFormatJSON)).
			WithResource(format).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", statusFormatPretty, statusFormatJSON))
	}
	if !nameRegexp.MatchString(name) || len(name) > 64 {
		return output.NewError("bad_input", "a plan name matching ^[a-z0-9_-]+$ is required").
			WithNextAction("reissue as: spektacular implement goal show <plan>")
	}
	p, err := loadGoalProject()
	if err != nil {
		return err
	}
	if _, _, err := p.planDocs(name); err != nil {
		return err
	}
	outcome, err := goal.Load(p.store, p.cfg.Plan.Config.Directory, name)
	if err != nil {
		return err
	}
	if item, _ := cmd.Flags().GetString("item"); item != "" {
		outcome.Items = []goal.ItemOutcome{outcome.Item(item)}
	}
	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(outcome)
	}
	return renderGoalOutcome(cmd.OutOrStdout(), outcome)
}

// renderGoalOutcome writes the readable outcome: per item, its goal, every
// check's verdicts with evidence, every decision and the attempt count.
func renderGoalOutcome(w io.Writer, o goal.Outcome) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  goal checks\n", o.Plan)
	if len(o.Items) == 0 {
		b.WriteString("\n  no goal checks recorded\n")
	}
	for _, it := range o.Items {
		title := it.Title
		if title == "" {
			title = "(not checked)"
		}
		fmt.Fprintf(&b, "\n%s  %s\n", it.Item, title)
		fmt.Fprintf(&b, "  state: %s, %d attempt(s)\n", strings.ReplaceAll(it.State, "_", " "), it.Attempts)
		if len(it.Checks) > 0 {
			g := it.Checks[len(it.Checks)-1].Goal
			b.WriteString("  goal:\n")
			for i, c := range g.Criteria {
				fmt.Fprintf(&b, "    %d. %s\n", i+1, c)
			}
			fmt.Fprintf(&b, "  scope: repo %s; %s\n", orNone(g.Scope.Repo), g.Scope.Rule)
			for _, f := range g.Scope.Files {
				fmt.Fprintf(&b, "    - %s: %s\n", f.Repo, f.Path)
			}
		}
		decisions := map[int][]goal.Decision{}
		for _, d := range it.Decisions {
			decisions[d.Round] = append(decisions[d.Round], d)
		}
		for i, c := range it.Checks {
			tree := ""
			if !c.TreeUnchanged {
				tree = ", work changed while the reviewer ran or was not staged"
			}
			fmt.Fprintf(&b, "  round %d, attempt %d (%s): %s%s\n", c.Round, c.Attempt, c.At, c.Decision, tree)
			for _, v := range c.Verdicts {
				fmt.Fprintf(&b, "    %d. %s: %s\n", v.Criterion, v.Verdict, v.Evidence)
			}
			lastOfRound := i == len(it.Checks)-1 || it.Checks[i+1].Round != c.Round
			if lastOfRound {
				for _, d := range decisions[c.Round] {
					line := fmt.Sprintf("  decision (%s): %s", d.At, d.Decision)
					if d.Reason != "" {
						line += ": " + d.Reason
					}
					b.WriteString(line + "\n")
				}
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// goalStageDir is the project-relative folder the check_goal step stages the
// work's diff, its status and the reviewer's reply in.
func goalStageDir(plan string) string {
	return ".spektacular/tmp/" + plan + "/goal-check"
}

// goalTreeUnchanged compares each code root's live diff and status with the
// <repo>.diff and <repo>.status the agent staged just before launching the
// reviewer, so a reviewer that changed any file is caught whatever agent ran
// it. Spektacular's own .spektacular folders are left out on both sides: the
// agent and the CLI write there during the check. A mismatch, or a missing
// staged file, gives false and a warning, never a refusal, so the evidence of
// a reviewer that changed the work is kept.
func goalTreeUnchanged(p goalProject, plan string) (bool, string) {
	roots, err := goalCodeRoots(p, plan)
	if err != nil {
		return false, fmt.Sprintf("the code roots could not be listed (%v), so the work could not be compared", err)
	}
	stage := filepath.Join(p.root, filepath.FromSlash(goalStageDir(plan)))
	var problems []string
	for _, r := range roots {
		for _, probe := range []struct {
			ext  string
			args []string
			norm func(string) string
		}{
			{"diff", []string{"diff", "HEAD"}, withoutSpektacularDiff},
			{"status", []string{"status", "--porcelain", "--untracked-files=all"}, withoutSpektacularStatus},
		} {
			staged, err := os.ReadFile(filepath.Join(stage, r.Repo+"."+probe.ext))
			if err != nil {
				problems = append(problems, fmt.Sprintf("no staged %s for %s at %s/%s.%s", probe.ext, r.Repo, goalStageDir(plan), r.Repo, probe.ext))
				continue
			}
			live, err := gitexec.Run(r.Root, nil, probe.args...)
			if err != nil {
				problems = append(problems, fmt.Sprintf("git %s failed in %s (%v)", probe.ext, r.Root, err))
				continue
			}
			if probe.norm(string(staged)) != probe.norm(live) {
				problems = append(problems, fmt.Sprintf("%s's %s changed between staging and recording", r.Repo, probe.ext))
			}
		}
	}
	if len(problems) == 0 {
		return true, ""
	}
	return false, "the work is not known to be unchanged across the reviewer's run: " + strings.Join(problems, "; ")
}

// goalCodeRoots are where each repo's code lives for the plan's run: the
// spec's worktree roots when it is built in its own worktrees, otherwise every
// registered repo whose code is on disk.
func goalCodeRoots(p goalProject, plan string) ([]workflow.CodeRoot, error) {
	roots, err := codeRootsFor(p.root, p.cfg, plan)
	if err != nil || len(roots) > 0 {
		return roots, err
	}
	set, err := repo.New(p.cfg, p.root, nil)
	if err != nil {
		return nil, err
	}
	for _, e := range set.Entries() {
		if dir, ok := set.LocalSource(e.Name); ok {
			roots = append(roots, workflow.CodeRoot{Repo: e.Name, Root: dir})
		}
	}
	return roots, nil
}

// isSpektacularPath reports a path inside a .spektacular folder.
func isSpektacularPath(path string) bool {
	path = strings.Trim(path, `"`)
	return strings.HasPrefix(path, ".spektacular/") || strings.Contains(path, "/.spektacular/")
}

// withoutSpektacularDiff drops every file section of a diff whose path is
// inside a .spektacular folder.
func withoutSpektacularDiff(diff string) string {
	var out []string
	keep := true
	for _, line := range strings.Split(strings.TrimSpace(diff), "\n") {
		if rest, ok := strings.CutPrefix(line, "diff --git a/"); ok {
			path, _, _ := strings.Cut(rest, " b/")
			keep = !isSpektacularPath(path)
		}
		if keep {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// withoutSpektacularStatus drops every porcelain status line whose path is
// inside a .spektacular folder.
func withoutSpektacularStatus(status string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if len(line) > 3 {
			path := line[3:]
			if _, to, ok := strings.Cut(path, " -> "); ok {
				path = to
			}
			if isSpektacularPath(path) {
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func init() {
	implementGoalRecordCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature","item":"<task id>"}')`)
	implementGoalRecordCmd.Flags().String("from", "", "the reviewer's reply, staged unchanged in a file")
	implementGoalDecideCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature","item":"<task id>","decision":"accept"}')`)
	implementGoalShowCmd.Flags().String("item", "", "show one item only: a task id or \"Phase N.M\"")
	implementGoalShowCmd.Flags().String("format", statusFormatPretty, "Output format: pretty or json")
	implementGoalCmd.AddCommand(implementGoalRecordCmd, implementGoalDecideCmd, implementGoalShowCmd)
	implementCmd.AddCommand(implementGoalCmd)
}
