package cmd

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/agent"
	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/stepkit"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/jumppad-labs/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// contractWorkingContextFooter is a hand-maintained copy of the rendered
// templates/partials/working-context-footer.md paragraph (its leading mustache
// comment line renders to nothing). It is the oracle every continuing step's
// instruction must end with.
const contractWorkingContextFooter = "**Before you advance:** refresh `.spektacular/working-context.md` with your cross-cutting working context only — the key decisions and substitutions made, the answers the user gave to your questions, and learnings worth carrying forward. Keep it to learnings and decisions, not a transcript and not a copy of content already captured elsewhere (such as a section's own working file). Use your own file tools. This file is git-tracked, and a resumed session reads it back to pick up where you left off, so keep it current every time before running the `goto` command above.\n"

// contractWorkflows are the workflows whose step templates the instruction
// contract covers.
var contractWorkflows = []string{"spec", "plan", "implement", "repo"}

// stepTemplateRow pairs a step template with the next step its workflow
// passes to stepkit.WriteStepResult.
type stepTemplateRow struct {
	workflow     string
	stepName     string
	templatePath string
	nextStep     string
}

// stepTemplateTable is hand-maintained to mirror the writeStep calls in
// internal/steps/{spec,plan,implement,repo}/steps.go. It is deliberately not
// derived from Steps(): a step whose next step changes must be reflected here
// by hand. Terminal (finished) steps have no next step.
var stepTemplateTable = []stepTemplateRow{
	{"spec", "new", "steps/spec/00-new.md", "interview"},
	{"spec", "interview", "steps/spec/00b-interview.md", "overview"},
	{"spec", "overview", "steps/spec/01-overview.md", "requirements"},
	{"spec", "requirements", "steps/spec/02-requirements.md", "acceptance_criteria"},
	{"spec", "acceptance_criteria", "steps/spec/03-acceptance_criteria.md", "constraints"},
	{"spec", "constraints", "steps/spec/04-constraints.md", "technical_approach"},
	{"spec", "technical_approach", "steps/spec/05-technical_approach.md", "success_metrics"},
	{"spec", "success_metrics", "steps/spec/06-success_metrics.md", "non_goals"},
	{"spec", "non_goals", "steps/spec/07-non_goals.md", "verification"},
	{"spec", "verification", "steps/spec/08-verification.md", "finished"},
	{"spec", "finished", "steps/spec/09-finished.md", ""},

	{"plan", "overview", "steps/plan/01-overview.md", "discovery"},
	{"plan", "discovery", "steps/plan/02-discovery.md", "architecture"},
	{"plan", "architecture", "steps/plan/03-architecture.md", "components"},
	{"plan", "components", "steps/plan/04-components.md", "data_structures"},
	{"plan", "data_structures", "steps/plan/05-data_structures.md", "implementation_detail"},
	{"plan", "implementation_detail", "steps/plan/06-implementation_detail.md", "dependencies"},
	{"plan", "dependencies", "steps/plan/07-dependencies.md", "testing_approach"},
	{"plan", "testing_approach", "steps/plan/08-testing_approach.md", "milestones"},
	{"plan", "milestones", "steps/plan/09-milestones.md", "phases"},
	{"plan", "phases", "steps/plan/10-phases.md", "open_questions"},
	{"plan", "open_questions", "steps/plan/11-open_questions.md", "out_of_scope"},
	{"plan", "out_of_scope", "steps/plan/12-out_of_scope.md", "assemble"},
	{"plan", "assemble", "steps/plan/13-assemble.md", "verification"},
	{"plan", "verification", "steps/plan/14-verification.md", "write_plan"},
	{"plan", "write_plan", "steps/plan/15-write_plan.md", "write_context"},
	{"plan", "write_context", "steps/plan/16-write_context.md", "write_research"},
	{"plan", "write_research", "steps/plan/17-write_research.md", "walkthrough"},
	{"plan", "walkthrough", "steps/plan/18-walkthrough.md", "finished"},
	{"plan", "finished", "steps/plan/19-finished.md", ""},

	{"implement", "read_plan", "steps/implement/01-read_plan.md", "analyze"},
	{"implement", "analyze", "steps/implement/02-analyze.md", "implement"},
	{"implement", "implement", "steps/implement/03-implement.md", "test"},
	{"implement", "test", "steps/implement/04-test.md", "verify"},
	{"implement", "verify", "steps/implement/05-verify.md", "update_plan"},
	{"implement", "update_plan", "steps/implement/06-update_plan.md", "update_changelog"},
	{"implement", "update_changelog", "steps/implement/07-update_changelog.md", "test_plan"},
	{"implement", "test_plan", "steps/implement/09-test_plan.md", "update_feature_changelog"},
	{"implement", "update_feature_changelog", "steps/implement/10-update_feature_changelog.md", "reconcile_spec"},
	{"implement", "reconcile_spec", "steps/implement/11-reconcile_spec.md", "finished"},
	{"implement", "finished", "steps/implement/12-finished.md", ""},

	{"repo", "locate", "steps/repo/01-locate.md", "name"},
	{"repo", "name", "steps/repo/02-name.md", "description"},
	{"repo", "description", "steps/repo/03-description.md", "role"},
	{"repo", "role", "steps/repo/04-role.md", "tags"},
	{"repo", "tags", "steps/repo/05-tags.md", "placement"},
	{"repo", "placement", "steps/repo/06-placement.md", "confirm"},
	{"repo", "confirm", "steps/repo/07-confirm.md", "register"},
	{"repo", "register", "steps/repo/08-register.md", "finished"},
	{"repo", "finished", "steps/repo/09-finished.md", ""},
}

// renderedInstruction is one step template rendered the way a workflow emits
// it at runtime.
type renderedInstruction struct {
	workflow     string
	stepName     string
	templatePath string
	nextStep     string
	body         string
}

// contractData is a minimal workflow.Data backed by a map.
type contractData map[string]any

func (d contractData) Get(key string) (any, bool) { v, ok := d[key]; return v, ok }
func (d contractData) Set(key string, value any)  { d[key] = value }

// contractCapture records the last result written.
type contractCapture struct{ result any }

func (c *contractCapture) WriteResult(v any) error { c.result = v; return nil }

// contractStrategy supplies realistic path and name variables for every
// workflow at once, so any step template renders with its placeholders
// filled.
type contractStrategy struct{}

func (contractStrategy) PathVars(instanceName, _ string) map[string]any {
	planDir := "/proj/.spektacular/plans/" + instanceName
	return map[string]any{
		"name":           instanceName,
		"plan_name":      instanceName,
		"spec_name":      instanceName,
		"repo_name":      instanceName,
		"plan_dir":       planDir,
		"plan_path":      planDir + "/plan.md",
		"context_path":   planDir + "/context.md",
		"research_path":  planDir + "/research.md",
		"spec_path":      "/proj/.spektacular/specs/" + instanceName + ".md",
		"changelog_path": "/proj/.spektacular/changelog/" + instanceName + ".md",
		"repo_path":      "/proj/repos/" + instanceName,
	}
}

func (contractStrategy) PrimaryPathField() string { return "plan_path" }

var _ stepkit.PathStrategy = contractStrategy{}

// renderAllStepInstructions renders every step template in stepTemplateTable
// through stepkit.WriteStepResult with the given command prefix. It first
// requires that the table covers every step template on disk in the embedded
// templates FS, so a newly added step cannot slip past the contract.
func renderAllStepInstructions(t *testing.T, command string) []renderedInstruction {
	t.Helper()
	requireStepTableComplete(t)

	out := make([]renderedInstruction, 0, len(stepTemplateTable))
	for _, row := range stepTemplateTable {
		capture := &contractCapture{}
		err := stepkit.WriteStepResult(
			stepkit.StepRequest{
				StepName:     row.stepName,
				NextStep:     row.nextStep,
				TemplatePath: row.templatePath,
				Strategy:     contractStrategy{},
			},
			contractData{"name": "demo-feature"},
			capture, nil, workflow.Config{Command: command, Kind: row.workflow},
			func(_, _, _, instruction string) any { return instruction },
		)
		require.NoErrorf(t, err, "rendering %s", row.templatePath)
		body, ok := capture.result.(string)
		require.Truef(t, ok, "%s: builder result must be the instruction string", row.templatePath)

		out = append(out, renderedInstruction{
			workflow:     row.workflow,
			stepName:     row.stepName,
			templatePath: row.templatePath,
			nextStep:     row.nextStep,
			body:         body,
		})
	}
	return out
}

// requireStepTableComplete walks steps/<workflow>/ for each contract workflow
// and requires every markdown template to appear in stepTemplateTable.
func requireStepTableComplete(t *testing.T) {
	t.Helper()
	inTable := make(map[string]bool, len(stepTemplateTable))
	for _, row := range stepTemplateTable {
		inTable[row.templatePath] = true
	}
	for _, wf := range contractWorkflows {
		err := fs.WalkDir(templates.FS, "steps/"+wf, func(p string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() || path.Ext(p) != ".md" {
				return nil
			}
			require.Truef(t, inTable[p],
				"%s is a step template missing from stepTemplateTable; add it with its next step", p)
			return nil
		})
		require.NoErrorf(t, err, "walking steps/%s", wf)
	}
}

func TestContinuingStepsEndWithIdenticalFooter(t *testing.T) {
	for _, ri := range renderAllStepInstructions(t, "spektacular") {
		if ri.nextStep == "" {
			require.NotContainsf(t, ri.body, "**Before you advance:**",
				"%s is terminal and must carry no working-context footer", ri.templatePath)
			continue
		}
		require.Equalf(t, 1, strings.Count(ri.body, contractWorkingContextFooter),
			"%s must carry exactly one working-context footer", ri.templatePath)
		require.Truef(t, strings.HasSuffix(ri.body, "\n\n---\n\n"+contractWorkingContextFooter),
			"%s must end with a rule followed by the working-context footer", ri.templatePath)
	}
}

func TestNoTemplateFileCarriesFooter(t *testing.T) {
	err := fs.WalkDir(templates.FS, "steps", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(templates.FS, p)
		require.NoError(t, err)
		require.NotContainsf(t, string(body), "**Before you advance:**",
			"%s carries the working-context footer; it is appended at render time", p)
		return nil
	})
	require.NoError(t, err)
}

// gotoInvocation matches a workflow goto invocation; the text before it on
// the same line must be the configured command prefix.
var gotoInvocation = regexp.MustCompile(`\b(spec|plan|implement|repo) goto --data`)

// instructionSource is one piece of agent-facing text, labelled by where it
// came from so a failure names its origin.
type instructionSource struct {
	label string
	body  string
}

// installClaudeInto runs the production claude install into a fresh temp dir
// with the given command prefix and returns that dir. The install writes the
// workflow skills under .claude/skills/ and the managed sections into
// AGENTS.md.
func installClaudeInto(t *testing.T, command string) string {
	t.Helper()
	a, err := agent.Lookup("claude")
	require.NoError(t, err)
	cfg := config.NewDefault()
	cfg.Command = command
	installDir := t.TempDir()
	require.NoError(t, a.Install(installDir, cfg, io.Discard))
	return installDir
}

// workflowInstructionCorpus gathers the text the workflows emit with the given
// command prefix: every rendered step instruction, the resume and mismatch
// instructions for each workflow kind, and every workflow skill the claude
// install writes into installDir.
func workflowInstructionCorpus(t *testing.T, command, installDir string) []instructionSource {
	t.Helper()
	var corpus []instructionSource

	for _, ri := range renderAllStepInstructions(t, command) {
		corpus = append(corpus, instructionSource{ri.templatePath, ri.body})
	}

	for _, kind := range contractWorkflows {
		body, err := resumeInstruction(command, kind, "demo-feature", "some_step")
		require.NoError(t, err)
		corpus = append(corpus, instructionSource{"steps/resume.md (" + kind + ")", body})

		other := "spec"
		if kind == "spec" {
			other = "plan"
		}
		body, err = mismatchInstruction(command, kind, other, "demo-feature", "some_step")
		require.NoError(t, err)
		corpus = append(corpus, instructionSource{"steps/resume_mismatch.md (" + kind + ")", body})
	}

	skillsRoot := filepath.Join(installDir, ".claude", "skills")
	skills := 0
	err := filepath.WalkDir(skillsRoot, func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		rel, _ := filepath.Rel(installDir, p)
		corpus = append(corpus, instructionSource{rel, string(b)})
		skills++
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, skills, "the claude install must produce workflow skills to check")

	return corpus
}

func TestNextCommandsCarryPrefix(t *testing.T) {
	const command = "spekx"

	corpus := workflowInstructionCorpus(t, command, installClaudeInto(t, command))

	invocations := 0
	for _, src := range corpus {
		require.NotContainsf(t, src.body, "{{", "%s has an unrendered mustache tag", src.label)

		for i, line := range strings.Split(src.body, "\n") {
			for _, loc := range gotoInvocation.FindAllStringIndex(line, -1) {
				invocations++
				require.Truef(t, strings.HasSuffix(line[:loc[0]], command+" "),
					"%s:%d: goto invocation lacks the %q prefix: %s",
					src.label, i+1, command, line)
			}
		}
	}
	require.NotZero(t, invocations, "the corpus must contain goto invocations to check")
}

// oldWorkingContextPath is the working-context file's former location. No
// agent-facing text may still send an agent there.
const oldWorkingContextPath = ".spektacular/context.md"

// agentFacingCorpus extends workflowInstructionCorpus with the rest of the
// text an agent reads: the managed AGENTS.md sections the claude install
// writes and every helper skill served through the skill command. It chdirs
// into a fresh skill project fixture, so callers must not run in parallel.
func agentFacingCorpus(t *testing.T, command string) []instructionSource {
	t.Helper()
	installDir := installClaudeInto(t, command)
	corpus := workflowInstructionCorpus(t, command, installDir)

	agentsMD, err := os.ReadFile(filepath.Join(installDir, "AGENTS.md"))
	require.NoError(t, err, "the claude install must write the managed AGENTS.md sections")
	corpus = append(corpus, instructionSource{"AGENTS.md (managed sections)", string(agentsMD)})

	skillProject(t)
	helperSkills := listSkills()
	require.NotEmpty(t, helperSkills, "the skill library must serve helper skills to check")
	for _, name := range helperSkills {
		corpus = append(corpus, instructionSource{"skill " + name, fetchSkillInstructions(t, name)})
	}
	return corpus
}

func TestNoEmittedInstructionNamesOldWorkingContext(t *testing.T) {
	for _, src := range agentFacingCorpus(t, "spektacular") {
		require.NotContainsf(t, src.body, oldWorkingContextPath,
			"%s still names the old working-context path", src.label)
	}
}

// unqualifiedContextMd reports whether line mentions the plan's context.md
// without saying whose it is. A mention counts only when context.md is not
// glued to a preceding identifier character, so names such as
// phases_context.md, working-context.md or memory-context.md are not
// mentions. A mention is qualified when its line says "plan's" or names the
// file under a plan: the concrete harness plan (demo-feature), or the
// <plan_name>/<name> placeholders the skills use. mentions is the number of
// context.md mentions on the line.
func unqualifiedContextMd(line string) (mentions int, unqualified bool) {
	const target = "context.md"
	for i := 0; ; {
		j := strings.Index(line[i:], target)
		if j < 0 {
			break
		}
		at := i + j
		i = at + len(target)
		if at > 0 {
			c := line[at-1]
			if c == '_' || c == '-' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') {
				continue
			}
		}
		mentions++
	}
	if mentions == 0 {
		return 0, false
	}
	for _, qualifier := range []string{
		"plan's",
		"demo-feature/context.md",
		"<plan_name>/context.md",
		"<name>/context.md",
		"/demo-feature/",
	} {
		if strings.Contains(line, qualifier) {
			return mentions, false
		}
	}
	return mentions, true
}

func TestUnqualifiedContextMdFlagsBareMention(t *testing.T) {
	for line, want := range map[string]bool{
		"stop and move it to context.md.":                         true,
		"record the decision in the plan's `context.md`.":         false,
		"run `spektacular plan file read <plan_name>/context.md`": false,
		"refresh `.spektacular/working-context.md` before goto":   false,
	} {
		_, got := unqualifiedContextMd(line)
		require.Equalf(t, want, got, "unqualified verdict for %q", line)
	}
}

// Two different files are called context.md in this project's history, so
// every agent-facing mention of the plan's context.md must say whose it is.
func TestContextMdAlwaysQualified(t *testing.T) {
	qualified := 0
	for _, src := range agentFacingCorpus(t, "spektacular") {
		for i, line := range strings.Split(src.body, "\n") {
			mentions, unqualified := unqualifiedContextMd(line)
			require.Falsef(t, unqualified,
				"%s:%d: context.md is not qualified as the plan's: %s", src.label, i+1, line)
			qualified += mentions
		}
	}
	require.NotZero(t, qualified, "the corpus must contain context.md mentions to check")
}

// newWorkingContextPath is the working-context file's current location,
// hand-maintained rather than read from workingcontext.RelPath.
const newWorkingContextPath = ".spektacular/working-context.md"

func TestEachWorkflowNamesWorkingContext(t *testing.T) {
	const command = "spektacular"

	// workflowSkillFor maps each contract workflow to the skill that drives it.
	workflowSkillFor := map[string]string{
		"spec":      "spek-new",
		"plan":      "spek-plan",
		"implement": "spek-implement",
		"repo":      "spek-manage-repos",
	}
	require.Len(t, workflowSkillFor, len(contractWorkflows))

	instructions := renderAllStepInstructions(t, command)
	installDir := installClaudeInto(t, command)

	for _, wf := range contractWorkflows {
		named := false
		for _, ri := range instructions {
			if ri.workflow == wf && strings.Contains(ri.body, newWorkingContextPath) {
				named = true
				break
			}
		}
		require.Truef(t, named, "no rendered %s step instruction names %s", wf, newWorkingContextPath)

		skill := workflowSkillFor[wf]
		require.NotEmptyf(t, skill, "%s has no workflow skill mapped", wf)
		body, err := os.ReadFile(filepath.Join(installDir, ".claude", "skills", skill, "SKILL.md"))
		require.NoErrorf(t, err, "reading installed %s skill", skill)
		require.Containsf(t, string(body), newWorkingContextPath,
			"the installed %s skill must name %s", skill, newWorkingContextPath)
	}
}

// normalizeIndent strips leading whitespace from every line of s, so a block
// included at an indent compares equal to the same block at column 0.
func normalizeIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func TestImplementStartListsPlanDocuments(t *testing.T) {
	const command = "spekx"

	skill, err := os.ReadFile(filepath.Join(installClaudeInto(t, command), ".claude", "skills", "spek-implement", "SKILL.md"))
	require.NoError(t, err, "reading installed spek-implement skill")

	var readPlan string
	for _, ri := range renderAllStepInstructions(t, command) {
		if ri.workflow == "implement" && ri.stepName == "read_plan" {
			readPlan = ri.body
			break
		}
	}
	require.NotEmpty(t, readPlan, "the harness must render the implement read_plan instruction")

	// The partial is rendered only to locate the shared block in each surface;
	// the hand-written anchors below are the oracle for what it says.
	block, err := stepkit.RenderTemplate("partials/implement-plan-documents.md", map[string]any{"command": command})
	require.NoError(t, err)
	block = normalizeIndent(strings.TrimSpace(block))
	require.NotEmpty(t, block)

	surfaces := []instructionSource{
		{"spek-implement/SKILL.md", string(skill)},
		{"steps/implement/01-read_plan.md", readPlan},
	}
	for _, src := range surfaces {
		for _, anchor := range []string{
			command + " plan file read <plan_name>/plan.md",
			command + " plan file read <plan_name>/context.md",
			command + " plan file read <plan_name>/research.md",
			"the per-phase technical detail",
			"the decision log",
			".spektacular/working-context.md",
		} {
			require.Containsf(t, src.body, anchor, "%s must carry %q", src.label, anchor)
		}
		require.Containsf(t, normalizeIndent(src.body), block,
			"%s must include the shared plan-documents block", src.label)
	}
}
