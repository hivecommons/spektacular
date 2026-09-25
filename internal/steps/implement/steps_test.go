package implement

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// today returns time.Now().UTC() truncated to day, matching the value the
// metadata package stamps when UpdateOptions.Today is unset. Used by the
// Phase 1.5 finish-step tests to assert closed_date without pinning wall time.
func today() time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour)
}

type testData struct {
	values map[string]any
}

func (d *testData) Get(key string) (any, bool) {
	v, ok := d.values[key]
	return v, ok
}

func (d *testData) Set(key string, value any) {
	d.values[key] = value
}

type captureWriter struct {
	result Result
}

func (c *captureWriter) WriteResult(v any) error {
	c.result = v.(Result)
	return nil
}

func renderStep(t *testing.T, cb workflow.StepCallback) string {
	t.Helper()
	return renderStepWithData(t, cb, map[string]any{"name": "test"})
}

// renderStepWithData drives a step callback the way renderStep does but with
// caller-supplied workflow data, for steps whose templates render values the
// command layer injects into the workflow (e.g. the repo roster).
func renderStepWithData(t *testing.T, cb workflow.StepCallback, values map[string]any) string {
	t.Helper()
	data := &testData{values: values}
	writer := &captureWriter{}
	st := store.NewFileStore(t.TempDir(), "project")
	_, err := cb(data, writer, st, workflow.Config{Command: "spektacular"})
	require.NoError(t, err)
	return writer.result.Instruction
}

// renderFinishedStep seeds the required project-level changelog artifact in
// the store before invoking finished(), so tests that only care about the
// template's rendered instruction don't trip the hard-fail-on-missing-
// changelog guard added to finished().
func renderFinishedStep(t *testing.T) string {
	t.Helper()
	data := &testData{values: map[string]any{"name": "test"}}
	writer := &captureWriter{}
	st := store.NewFileStore(t.TempDir(), "project")

	cfg := workflow.Config{Command: "spektacular"}
	seed, err := metadata.Render(metadata.Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
	}, []byte("# body\n"))
	require.NoError(t, err)
	require.NoError(t, st.Write(ChangelogFilePath(cfg.ChangelogDir, "test"), seed))

	_, err = finished()(data, writer, st, cfg)
	require.NoError(t, err)
	return writer.result.Instruction
}

func TestStepsOrderMatchesExpected(t *testing.T) {
	expected := []string{
		"new",
		"read_plan",
		"analyze",
		"implement",
		"test",
		"verify",
		"update_plan",
		"update_changelog",
		"test_plan",
		"update_feature_changelog",
		"reconcile_spec",
		"finished",
	}
	got := Steps()
	require.Len(t, got, len(expected))
	for i, step := range got {
		require.Equal(t, expected[i], step.Name, "step %d name mismatch", i)
	}
}

func TestAnalyzeStepHasMultiSourceTransition(t *testing.T) {
	for _, s := range Steps() {
		if s.Name == "analyze" {
			require.ElementsMatch(t, []string{"read_plan", "update_changelog"}, s.Src,
				"analyze must be reachable from both read_plan and update_changelog")
			return
		}
	}
	t.Fatal("analyze step not found")
}

func TestReconcileSpecStepWiring(t *testing.T) {
	steps := Steps()

	var reconcileSpecStep, finishedStep *workflow.StepConfig
	for i := range steps {
		switch steps[i].Name {
		case "reconcile_spec":
			reconcileSpecStep = &steps[i]
		case "finished":
			finishedStep = &steps[i]
		}
	}

	require.NotNil(t, reconcileSpecStep, "reconcile_spec step not found")
	require.NotNil(t, finishedStep, "finished step not found")

	require.Equal(t, []string{"update_feature_changelog"}, reconcileSpecStep.Src,
		"reconcile_spec must only be reachable from update_feature_changelog")
	require.Equal(t, []string{"reconcile_spec", "update_changelog"}, finishedStep.Src,
		"finished is reachable from reconcile_spec, and from update_changelog for a single-task run that leaves tasks open")
}

func TestFSMWalkFromNewToFinished(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	st := store.NewFileStore(tmp, "project")
	writer := &captureWriter{}

	wf := workflow.New(Steps(), statePath, workflow.Config{Command: "spektacular", DryRun: true}, st, writer)
	wf.SetData("name", "test")

	require.Equal(t, "start", wf.Current())

	// new auto-advances to read_plan, so the first Next lands on read_plan.
	// Walk forward with Next() up through update_changelog. Then use explicit
	// Goto() to disambiguate the multi-source exit (update_changelog has two
	// legal successors — analyze via the loop, and test_plan — so Next()
	// cannot pick one deterministically).
	linear := []string{
		"read_plan",
		"analyze",
		"implement",
		"test",
		"verify",
		"update_plan",
		"update_changelog",
	}
	for _, want := range linear {
		require.NoError(t, wf.Next(), "transition to %s failed", want)
		require.Equal(t, want, wf.Current(), "expected state %s after transition", want)
	}

	require.NoError(t, wf.Goto("test_plan"))
	require.Equal(t, "test_plan", wf.Current())
	require.NoError(t, wf.Goto("update_feature_changelog"))
	require.Equal(t, "update_feature_changelog", wf.Current())
	require.NoError(t, wf.Goto("reconcile_spec"))
	require.Equal(t, "reconcile_spec", wf.Current())
	require.NoError(t, wf.Goto("finished"))
	require.Equal(t, "finished", wf.Current())
}

// Phase 2.1 criterion 1: the implement workflow has no root-changelog step,
// and update_changelog's non-loop exit lands directly on test_plan; the
// removed step name is not a legal state at all.
func TestFSMHasNoUpdateRepoChangelogStep(t *testing.T) {
	for _, step := range Steps() {
		require.NotEqual(t, "update_repo_changelog", step.Name)
		for _, src := range step.Src {
			require.NotEqual(t, "update_repo_changelog", src, "%s must not be reachable from the removed step", step.Name)
		}
	}
	tmp := t.TempDir()
	wf := workflow.New(Steps(), filepath.Join(tmp, "state.json"), workflow.Config{Command: "spektacular", DryRun: true}, store.NewFileStore(tmp, "project"), &captureWriter{})
	wf.SetData("name", "test")
	for range []string{"read_plan", "analyze", "implement", "test", "verify", "update_plan", "update_changelog"} {
		require.NoError(t, wf.Next())
	}
	require.Error(t, wf.Goto("update_repo_changelog"))
	require.NoError(t, wf.Goto("test_plan"))
	require.Equal(t, "test_plan", wf.Current())
}

func TestFSMLoopFromUpdateChangelogBackToAnalyze(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	st := store.NewFileStore(tmp, "project")
	writer := &captureWriter{}

	wf := workflow.New(Steps(), statePath, workflow.Config{Command: "spektacular", DryRun: true}, st, writer)
	wf.SetData("name", "test")

	// Walk through to update_changelog the first time.
	for _, want := range []string{"read_plan", "analyze", "implement", "test", "verify", "update_plan", "update_changelog"} {
		require.NoError(t, wf.Next())
		require.Equal(t, want, wf.Current())
	}

	// Loop back via the multi-source edge: update_changelog → analyze.
	require.NoError(t, wf.Goto("analyze"))
	require.Equal(t, "analyze", wf.Current())

	// Walk forward again to update_changelog.
	for _, want := range []string{"implement", "test", "verify", "update_plan", "update_changelog"} {
		require.NoError(t, wf.Next())
		require.Equal(t, want, wf.Current())
	}

	// Second exit: update_changelog → test_plan → update_feature_changelog → reconcile_spec → finished.
	require.NoError(t, wf.Goto("test_plan"))
	require.Equal(t, "test_plan", wf.Current())
	require.NoError(t, wf.Goto("update_feature_changelog"))
	require.Equal(t, "update_feature_changelog", wf.Current())
	require.NoError(t, wf.Goto("reconcile_spec"))
	require.Equal(t, "reconcile_spec", wf.Current())
	require.NoError(t, wf.Goto("finished"))
	require.Equal(t, "finished", wf.Current())
}

// --- Per-template content assertions ---

func TestReadPlanStepContainsFullReadDirective(t *testing.T) {
	out := renderStep(t, readPlan())
	lower := strings.ToLower(out)
	require.Contains(t, lower, "in full", "read_plan must direct a full read of the plan documents")
	// Plan documents are read through the CLI, never the built-in Read tool.
	require.Contains(t, out, "plan file read", "read_plan must read the plan documents via `plan file read`")
	// The shared plan-documents partial renders {{command}} from the step's
	// config, so each read command carries the configured prefix.
	require.Contains(t, out, "spektacular plan file read <plan_name> plan")
	require.Contains(t, out, "spektacular plan file read <plan_name> context")
	require.Contains(t, out, "spektacular plan file read <plan_name> research")
}

func TestReadPlanStepMentionsChangelog(t *testing.T) {
	out := renderStep(t, readPlan())
	require.Contains(t, out, "## Changelog")
	require.Contains(t, strings.ToLower(out), "first-task")
	require.Contains(t, strings.ToLower(out), "subsequent-task")
}

func TestReadPlanTemplateDirectsStructuralValidation(t *testing.T) {
	out := renderStep(t, readPlan())
	// Every required scaffold section must be mentioned.
	for _, section := range []string{
		"## Overview",
		"## Architecture & Design Decisions",
		"## Component Breakdown",
		"## Data Structures & Interfaces",
		"## Implementation Detail",
		"## Dependencies",
		"## Testing Approach",
		"## Milestones & Phases",
		"## Open Questions",
		"## Out of Scope",
	} {
		require.Contains(t, out, section, "read_plan must require section %q", section)
	}
	require.Contains(t, out, "#### - [ ] Phase")
	require.Contains(t, out, "*Technical detail:*")
}

func TestReadPlanTemplateDirectsDriftCheck(t *testing.T) {
	out := renderStep(t, readPlan())
	lower := strings.ToLower(out)
	require.Contains(t, lower, "drift")
	require.Contains(t, lower, "drift check against each repo's source", "drift check must name the target")
	require.Contains(t, lower, "stop")
	// The three-option prompt (fix / proceed / abandon).
	require.Contains(t, lower, "fix the plan first")
	require.Contains(t, lower, "proceed with")
	require.Contains(t, lower, "abandon")
}

func TestReadPlanTemplateDirectsSpecCoverageCheck(t *testing.T) {
	out := renderStep(t, readPlan())
	lower := strings.ToLower(out)
	require.Contains(t, out, "spec file read test", "spec coverage check must read the spec via `spec file read`")
	require.Contains(t, out, "## Requirements")
	require.Contains(t, out, "## Acceptance Criteria")
	require.Contains(t, lower, "stop")
	// The two-option prompt (fix / accept as descoped).
	require.Contains(t, lower, "fix the plan first")
	require.Contains(t, lower, "accept the gap as descoped")
}

func TestReadPlanTemplateDirectsDescopedMarkerMechanics(t *testing.T) {
	out := renderStep(t, readPlan())
	require.Contains(t, out, "**Descoped requirements**:", "descoped gaps must use the documented marker format")
	require.Contains(t, strings.ToLower(out), "already recorded as accepted", "spec coverage check must skip gaps already recorded as accepted")
	require.Contains(t, out, "plan file write test plan --from .spektacular/tmp/plan_update.md", "descoped marker must be committed via `plan file write`")
}

func TestAnalyzeStepReferencesSpawnImplementationAgents(t *testing.T) {
	out := renderStep(t, analyze())
	require.Contains(t, out, "skill spawn-implementation-agents")
}

func TestImplementStepForbidsInlineTests(t *testing.T) {
	out := renderStep(t, implementStep())
	lower := strings.ToLower(out)
	require.Contains(t, lower, "test")
	require.Contains(t, lower, "next step", "implement step must defer tests to the next step")
}

func TestTestStepReferencesFollowTestPatterns(t *testing.T) {
	out := renderStep(t, testStep())
	require.Contains(t, out, "skill follow-test-patterns")
	require.Contains(t, strings.ToLower(out), "sub-agent")
}

func TestVerifyStepReferencesVerifyImplementation(t *testing.T) {
	out := renderStep(t, verify())
	require.Contains(t, out, "skill verify-implementation")
	require.Contains(t, strings.ToLower(out), "pass/fail")
}

// The implement, test and verify steps each load the current phase from the
// plan store themselves rather than trusting an earlier step's output to
// still be in the agent's context (it may have been compacted or resumed).
func TestPhaseStepsReadPhaseDetail(t *testing.T) {
	for name, cb := range map[string]workflow.StepCallback{
		"implement": implementStep(),
		"test":      testStep(),
		"verify":    verify(),
	} {
		t.Run(name, func(t *testing.T) {
			out := renderStep(t, cb)
			require.Contains(t, out, "spektacular plan file read test plan",
				"%s must read the plan to find the current phase", name)
			require.Contains(t, out, "spektacular plan file read test context",
				"%s must read the phase's technical detail from the plan's context.md", name)
			for _, stale := range []string{"analysis summaries", "from the previous step", "already available"} {
				require.NotContains(t, out, stale,
					"%s must not assume an earlier step's output is still in context", name)
			}
		})
	}
}

func TestUpdatePlanStepDirectsCheckboxMarking(t *testing.T) {
	out := renderStep(t, updatePlan())
	require.Contains(t, out, "[x]")
	require.Contains(t, out, "- [ ]")
	require.Contains(t, out, "#### - [ ] Phase")
}

func TestUpdateChangelogStepSpecifiesEntryFields(t *testing.T) {
	out := renderStep(t, updateChangelog())
	for _, field := range []string{
		"What was done",
		"Deviations",
		"Files changed",
		"Discoveries",
	} {
		require.Contains(t, out, field, "update_changelog must specify field %q", field)
	}
}

func TestUpdateChangelogStepCreatesSectionOnFirstInvocation(t *testing.T) {
	out := renderStep(t, updateChangelog())
	require.Contains(t, out, "## Changelog")
	require.Contains(t, strings.ToLower(out), "first")
	require.Contains(t, out, "## Out of Scope", "update_changelog must anchor the new section relative to Out of Scope")
}

func TestUpdateChangelogStepBranchesOnUncheckedPhases(t *testing.T) {
	out := renderStep(t, updateChangelog())
	// Both legal exits must be present.
	require.Contains(t, out, `"step":"analyze"`)
	require.Contains(t, out, `"step":"test_plan"`)
	require.NotContains(t, out, "update_repo_changelog")
	require.Contains(t, strings.ToLower(out), "ask the user")
}

// TestUpdateChangelogStepOffersKnowledgeCaptureForDurableDiscoveries asserts
// the update_changelog step directs the agent to assess the phase's
// Discoveries entry for durable knowledge and offer capture via the
// spek-knowledge skill, gated on explicit acceptance with decline being final
// (Phase 1.1 assessment-and-offer beat).
func TestUpdateChangelogStepOffersKnowledgeCaptureForDurableDiscoveries(t *testing.T) {
	out := renderStep(t, updateChangelog())
	lower := strings.ToLower(out)

	// Durability assessment: each discovery is weighed for value beyond the
	// current change.
	require.Contains(t, lower, "durable", "update_changelog must direct a durability assessment of discoveries")
	require.Contains(t, lower, "beyond this one change", "update_changelog must frame durability as holding beyond the current change")

	// The offer: name what would be captured and why it is worth keeping.
	require.Contains(t, lower, "offer", "update_changelog must direct offering knowledge capture, never writing unprompted")
	require.Contains(t, lower, "name what you would capture and why it is worth keeping", "update_changelog must require the offer to name what and why")

	// Selectivity bar: change-local discoveries produce no offer.
	require.Contains(t, lower, "most tasks produce none", "update_changelog must state that most tasks produce no qualifying discovery")

	// Confirm gate: capture only on explicit acceptance.
	require.Contains(t, lower, "explicit acceptance", "update_changelog must gate capture on the user's explicit acceptance")

	// Decline finality: a declined item is not offered again.
	require.Contains(t, lower, "not offered again", "update_changelog must make a decline final for that discovery")

	// The hand-off goes to the spek-knowledge skill.
	require.Contains(t, out, "spek-knowledge", "update_changelog must hand accepted items to the spek-knowledge skill")

	// Negative guard: there is no `skill spek-knowledge` CLI subcommand — the
	// template must not direct the unreachable invocation
	// `spektacular skill spek-knowledge` (the legitimate
	// `skill update-changelog` reference must not trip this).
	require.NotContains(t, out, "skill spek-knowledge", "update_changelog must not direct the nonexistent `skill spek-knowledge` CLI invocation")
}

func TestStopOnMismatchDirectivePresentInEveryNonTerminalTemplate(t *testing.T) {
	nonTerminal := map[string]workflow.StepCallback{
		"read_plan":                readPlan(),
		"analyze":                  analyze(),
		"implement":                implementStep(),
		"test":                     testStep(),
		"verify":                   verify(),
		"update_plan":              updatePlan(),
		"update_changelog":         updateChangelog(),
		"test_plan":                testPlan(),
		"update_feature_changelog": updateFeatureChangelog(),
		"reconcile_spec":           reconcileSpec(),
	}
	for name, cb := range nonTerminal {
		out := renderStep(t, cb)
		require.Contains(t, strings.ToUpper(out), "STOP", "%s template must contain a STOP directive", name)
	}
}

// TestPlanFilePaths_UseConfiguredDirectory asserts the implement path helpers
// root plan, context and research files under the given directory argument
// (Phase 2.2).
func TestPlanFilePaths_UseConfiguredDirectory(t *testing.T) {
	require.Equal(t, "my-plans/x/plan.md", PlanFilePath("my-plans", "x"))
	require.Equal(t, "my-plans/x/context.md", ContextFilePath("my-plans", "x"))
	require.Equal(t, "my-plans/x/research.md", ResearchFilePath("my-plans", "x"))
}

func TestFinishedStepEmitsNoGoto(t *testing.T) {
	out := renderFinishedStep(t)
	require.NotContains(t, out, "implement goto", "finished template must not emit a goto command")
	require.Contains(t, strings.ToLower(out), "terminal")
}

func TestFinishedStepMentionsChangelogRecordByAddress(t *testing.T) {
	out := renderFinishedStep(t)
	require.Contains(t, out, "The project changelog record, named `test` (read it with `spektacular changelog file read test`).",
		"finished template must name the project changelog record by address and the command that reads it")
}

func TestFinishedStepReportsSpecCompletionStatus(t *testing.T) {
	out := renderFinishedStep(t)
	require.Contains(t, out, "spec file read test", "finished template must direct reading the spec via `spec file read`")
	require.Contains(t, out, "Requirements/Acceptance-Criteria", "finished template must report Requirements/Acceptance-Criteria status")
	require.Contains(t, out, "reconcile_spec", "finished template must reference reconcile_spec as the source of unchecked-item reasons")
	require.Contains(t, out, "deferred, descoped, or not attempted", "finished template must enumerate the possible reasons for unchecked items")
}

// Phase 2.2 criterion 2: the finished step reports where the project
// record and each per-repo record were written, and no longer mentions a
// repo-level CHANGELOG.md.
func TestFinishedStepReportsProjectAndPerRepoRecordLocations(t *testing.T) {
	out := renderFinishedStep(t)
	require.Contains(t, out, "project changelog record", "finished must name the project record")
	require.Contains(t, out, "one derived record per affected repo", "finished must name the per-repo records")
	require.Contains(t, out, "changelog file read test --repo <repo-name>", "finished must direct reading each per-repo record through the CLI")
	require.Contains(t, out, "user-facing summary", "finished must describe the per-repo record as the repo's release note")
	require.NotContains(t, out, "CHANGELOG.md")
}

func TestUpdateFeatureChangelogStepMentionsSourcesAndCommitCommand(t *testing.T) {
	out := renderStep(t, updateFeatureChangelog())
	require.Contains(t, out, "spec file read test", "update_feature_changelog must read the feature's spec via `spec file read`")
	require.Contains(t, out, "plan file read test plan", "update_feature_changelog must read the plan's implementation history via `plan file read`")
	require.Contains(t, out, ".spektacular/tmp/changelog_project.md", "update_feature_changelog must stage the project-level record at a project-scoped scratch path")
	require.Contains(t, out, "changelog file write test --from .spektacular/tmp/changelog_project.md", "update_feature_changelog must commit the project-level record via `changelog file write`")
	// The rendered advance target must match the FSM, which only allows
	// update_feature_changelog → reconcile_spec (finished is two steps away).
	require.Contains(t, out, `"step":"reconcile_spec"`, "update_feature_changelog must advance to reconcile_spec, not skip it")
	require.NotContains(t, out, `"step":"finished"`, "update_feature_changelog must not direct a goto to finished — the FSM rejects that transition")
}

// Criterion 3: the update_feature_changelog instruction writes one repo-level
// record per affected repo, including the colocated repo (no "central covers
// it" carve-out). Every affected repo gets its own record via `changelog file
// write --repo`, discovered from `repo list` and the Files-changed prefixes,
// carrying the readable reference line. Unaffected repos get no record.
func TestUpdateFeatureChangelogStepDerivesOneEntryPerAffectedRepo(t *testing.T) {
	out := renderStep(t, updateFeatureChangelog())

	require.Contains(t, out, "repo-level record per affected repo",
		"update_feature_changelog must direct writing one repo-level record per affected repo")
	require.Contains(t, out, "spektacular repo list",
		"update_feature_changelog must discover repos via `repo list`")
	require.Contains(t, out, "changelog file write test --repo <repo-name> --from .spektacular/tmp/changelog_<repo>.md",
		"per-repo records must be committed through `changelog file write --repo`")
	require.Contains(t, out, "> Derived from project",
		"each repo record must carry a human-readable reference line")
	require.Contains(t, out, "spec/plan test.",
		"the reference line must name the resolved spec/plan identifier")
	require.Contains(t, out, "only that repo's changes",
		"each repo record must be scoped to that repo's own changes")
	require.Contains(t, out, "including the colocated one",
		"every affected repo — including the colocated one — must get its own repo-level record; there must be no carve-out")
	require.NotContains(t, out, "other than the project's own colocated",
		"the old 'central record already covers the colocated repo' carve-out must be gone")
	require.Contains(t, out, "no repo-level record",
		"unaffected repos must not receive empty records")
}

// Phase 2.2 criterion 1: each per-repo record opens with a two-to-four
// sentence user-facing summary written for someone who has never seen the
// plan, taking over the role of the removed root release note.
func TestUpdateFeatureChangelogStepOpensRepoRecordWithUserFacingSummary(t *testing.T) {
	out := renderStep(t, updateFeatureChangelog())
	require.Contains(t, out, "User-facing summary first", "the per-repo record must lead with a user-facing summary")
	require.Contains(t, out, "2-4 sentence summary", "the summary length must be pinned")
	require.Contains(t, out, "never seen the plan", "the summary must be written for a reader without the plan")
	require.Contains(t, out, "No file paths, no internal package names", "the summary must exclude implementation detail")
	require.Contains(t, out, "release note", "the per-repo record must be framed as the repo's release note")
	require.NotContains(t, out, "CHANGELOG.md")
}

// Criterion 3: a derived write failing on a missing/broken repo footprint is
// surfaced to the user via the error's repair offer, never skipped silently.
func TestUpdateFeatureChangelogStepSurfacesFootprintRepair(t *testing.T) {
	out := renderStep(t, updateFeatureChangelog())
	lower := strings.ToLower(out)
	require.Contains(t, lower, "footprint is missing or broken",
		"the STOP section must cover a failed derived write's footprint error")
	require.Contains(t, lower, "surface the repair offer",
		"the footprint repair offer must be surfaced to the user")
	require.Contains(t, lower, "rather than skipping the repo silently",
		"a footprint failure must never be skipped silently")
}

// Criterion 3: the update_changelog instruction directs prefixing member-repo
// paths in Files-changed lists with `<repo-name>: ` (colocated paths stay
// unprefixed), which is what the feature-changelog step derives repos from.
func TestUpdateChangelogStepDirectsRepoPrefixedFilesChanged(t *testing.T) {
	out := renderStep(t, updateChangelog())

	require.Contains(t, out, "`<repo-name>: path",
		"the Files-changed example must show the repo-name prefix shape")
	lower := strings.ToLower(out)
	// Plan 000046: the prefix rule is defined by the number of registered
	// repos, never by which repo shares the working tree.
	require.Contains(t, lower, "prefix every path with its repo's name",
		"paths must carry the repo-name prefix")
	require.Contains(t, lower, "whenever more than one repo is registered",
		"the prefix is required whenever more than one repo is registered")
	require.Contains(t, lower, "an unprefixed path belongs to the only registered repo",
		"unprefixed paths belong to the only registered repo")
	require.NotContains(t, lower, "working tree",
		"the prefix rule must not use the working tree as a stand-in")
	require.Contains(t, lower, "one entry per affected repo",
		"the prefix rule must be tied to the downstream per-repo derivation")
}

// Phase 2.1 criterion 2: driving the implement workflow end to end never
// instructs the agent to create or modify a CHANGELOG.md; every non-terminal
// template and the finished template are free of the file name and of the
// removed step's name.
func TestNoImplementTemplateMentionsRootChangelogFile(t *testing.T) {
	all := map[string]workflow.StepCallback{
		"read_plan":                readPlan(),
		"analyze":                  analyze(),
		"implement":                implementStep(),
		"test":                     testStep(),
		"verify":                   verify(),
		"update_plan":              updatePlan(),
		"update_changelog":         updateChangelog(),
		"test_plan":                testPlan(),
		"update_feature_changelog": updateFeatureChangelog(),
		"reconcile_spec":           reconcileSpec(),
	}
	rendered := map[string]string{"finished": renderFinishedStep(t)}
	for name, cb := range all {
		rendered[name] = renderStep(t, cb)
	}
	for name, out := range rendered {
		require.NotContains(t, out, "CHANGELOG.md", "%s must not direct the agent at a CHANGELOG.md file", name)
		require.NotContains(t, out, "update_repo_changelog", "%s must not name the removed step", name)
	}
}

func TestReconcileSpecStepMentionsSourcesAndCommitCommand(t *testing.T) {
	out := renderStep(t, reconcileSpec())
	require.Contains(t, out, "spec file read test", "reconcile_spec must read the feature's spec via `spec file read`")
	require.Contains(t, out, "plan file read test plan", "reconcile_spec must read the plan's implementation history via `plan file read`")
	require.Contains(t, out, ".spektacular/tmp/spec_reconcile.md", "reconcile_spec must stage its record at the scratch path")
	require.Contains(t, out, "spec file write test --from .spektacular/tmp/spec_reconcile.md", "reconcile_spec must commit the record via `spec file write`")
}

// --- Phase 1.5: terminal-step closure of test-plan and changelog artifacts ---

// TestImplementFinished_ClosesTestPlanAndChangelog seeds both terminal
// artifacts (test-plan.md under PlanDir and the project-namespaced <name>.md
// under ChangelogDir) with draft metadata dated in the past, then
// asserts finished() transitions both to final with today's closed_date
// while preserving created_date.
func TestImplementFinished_ClosesTestPlanAndChangelog(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{
		Command:      "spektacular",
		PlanDir:      "plans",
		ChangelogDir: "changelog",
		SpecDir:      "specs",
	}
	planName := "fixture"

	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	testPlanPath := filepath.Join(cfg.PlanDir, planName, "test-plan.md")
	changelogPath := "changelog/fixture.md"

	for _, p := range []string{testPlanPath, changelogPath} {
		seed, err := metadata.Render(metadata.Metadata{
			CreatedDate:    created,
			DocumentStatus: metadata.StatusDraft,
		}, []byte("# Body of "+p+"\n"))
		require.NoError(t, err)
		require.NoError(t, st.Write(p, seed))
	}

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err := finished()(data, writer, st, cfg)
	require.NoError(t, err)

	for _, p := range []string{testPlanPath, changelogPath} {
		raw, err := st.Read(p)
		require.NoError(t, err, "%s must remain readable after finished()", p)
		meta, _, err := metadata.Split(raw)
		require.NoError(t, err)
		require.NotNil(t, meta, "%s must carry frontmatter", p)
		require.Equal(t, metadata.StatusFinal, meta.DocumentStatus, "%s must be final", p)
		require.True(t, meta.CreatedDate.Equal(created), "%s created_date must be preserved, got %s", p, meta.CreatedDate)
		require.True(t, meta.ClosedDate.Equal(today()), "%s closed_date must be today, got %s", p, meta.ClosedDate)
	}
}

// TestImplementFinished_FailsHardOnMissingChangelog asserts finished()
// surfaces a missing project-level changelog record as an error rather than
// silently tolerating it — the previous swallow (via errors.Is(err,
// store.ErrNotFound)) let update_feature_changelog silently no-op and leave
// no record on disk, and the workflow still marked itself finished. That
// regression is what motivated the hard-fail here.
func TestImplementFinished_FailsHardOnMissingChangelog(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{
		Command:      "spektacular",
		PlanDir:      "plans",
		ChangelogDir: "changelog",
		SpecDir:      "specs",
	}
	planName := "fixture"

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err := finished()(data, writer, st, cfg)
	require.Error(t, err, "finished() must error when the project-level changelog is missing")
	require.Contains(t, err.Error(), "changelog", "error must name the missing artifact")

	changelogPath := filepath.Join(cfg.ChangelogDir, planName+".md")
	require.False(t, st.Exists(changelogPath), "finished() must not create the changelog on the fly — the update_feature_changelog step owns that write")
}

// TestImplementFinished_TolerantOfMissingTestPlan asserts a missing test
// plan alone is still not a hard failure — only the changelog is required
// at this point in the FSM. This preserves the pre-existing tolerance for
// test-plan absence, which is out of scope for the changelog fix.
func TestImplementFinished_TolerantOfMissingTestPlan(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{
		Command:      "spektacular",
		PlanDir:      "plans",
		ChangelogDir: "changelog",
		SpecDir:      "specs",
	}
	planName := "fixture"

	changelogPath := filepath.Join(cfg.ChangelogDir, planName+".md")
	seed, err := metadata.Render(metadata.Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
	}, []byte("# body\n"))
	require.NoError(t, err)
	require.NoError(t, st.Write(changelogPath, seed))

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err = finished()(data, writer, st, cfg)
	require.NoError(t, err, "finished() must not error on a missing test plan when the changelog is present")

	testPlanPath := filepath.Join(cfg.PlanDir, planName, "test-plan.md")
	require.False(t, st.Exists(testPlanPath), "finished() must not create test-plan.md when it was missing")
}

// --- where each repo's code lives, in the code-touching steps ---

// repoListSteps are the two implement callbacks whose templates must send the
// agent to `repo list`: read_plan, which opens the workflow with the "Where
// the code lives" preamble, and update_feature_changelog, which attributes
// changes to repos. The other code-touching steps (analyze, implement, test,
// verify) rely on the agent carrying that forward from read_plan.
func repoListSteps() map[string]workflow.StepCallback {
	return map[string]workflow.StepCallback{
		"read_plan":                readPlan(),
		"update_feature_changelog": updateFeatureChangelog(),
	}
}

// laterCodeTouchingSteps are the code-touching steps after read_plan that
// must not repeat the preamble.
func laterCodeTouchingSteps() map[string]workflow.StepCallback {
	return map[string]workflow.StepCallback{
		"analyze":     analyze(),
		"implement":   implementStep(),
		"test":        testStep(),
		"verify":      verify(),
		"update_plan": updatePlan(),
	}
}

// No implement step carries a roster. The steps that need to know where a
// repo's code lives send the agent to `repo list`, which reports the registry
// as it stands right now — on a fresh run and on a resume alike.
func TestRepoListStepsSendTheAgentToRepoList(t *testing.T) {
	for name, cb := range repoListSteps() {
		t.Run(name, func(t *testing.T) {
			out := renderStep(t, cb)
			require.Contains(t, out, "repo list", "%s must send the agent to `repo list`", name)
			require.Contains(t, out, "`root`", "%s must name the root that repo list reports", name)
			require.NotContains(t, out, "## Repos", "%s must not point at a Repos section", name)
			require.NotContains(t, out, "{{", "%s must leave no unrendered mustache", name)
		})
	}
}

func TestWhereTheCodeLivesPreambleRenderedOnceByReadPlan(t *testing.T) {
	out := renderStepWithData(t, readPlan(), map[string]any{"name": "test"})
	require.Contains(t, out, "Where the code lives", "read_plan must open with the code-location preamble")
	require.Contains(t, out, "the rest of this workflow", "read_plan must scope the preamble to the whole workflow")

	for name, cb := range laterCodeTouchingSteps() {
		t.Run(name, func(t *testing.T) {
			out := renderStepWithData(t, cb, map[string]any{"name": "test"})
			require.NotContains(t, out, "Where the code lives", "%s must not repeat the code-location preamble", name)
			require.NotContains(t, out, "repo list", "%s must not repeat the repo list direction", name)
		})
	}
}

// renderTaskStep renders a step for a single-task run over a plan holding
// two tasks, the selected one and another.
func renderTaskStep(t *testing.T, cb workflow.StepCallback) string {
	t.Helper()
	root := t.TempDir()
	st := store.NewFileStore(root, "project")
	plan := "# Plan: test\n\n## Milestones & Tasks\n\n### Milestone 1: M\n\n" +
		"#### - [ ] Task: The selected task\n**Id:** sel-1\n**Repo:** r\n**Depends on:** none\n**Execution:** agent\n\n" +
		"#### - [ ] Task: Another task\n**Id:** other-2\n**Repo:** r\n**Depends on:** none\n**Execution:** agent\n"
	require.NoError(t, st.Write(filepath.Join("plans", "test", "plan.md"), []byte(plan)))

	data := &testData{values: map[string]any{"name": "test", "task": "sel-1"}}
	writer := &captureWriter{}
	_, err := cb(data, writer, st, workflow.Config{Command: "spektacular", PlanDir: "plans"})
	require.NoError(t, err)
	return writer.result.Instruction
}

// A single-task run names the selected task in every step that works on it,
// and limits that work to it; a whole-plan run carries no task scoping.
func TestTaskScopedStepsNameTheSelectedTask(t *testing.T) {
	for name, cb := range map[string]workflow.StepCallback{
		"analyze":     analyze(),
		"implement":   implementStep(),
		"test":        testStep(),
		"verify":      verify(),
		"update_plan": updatePlan(),
	} {
		t.Run(name, func(t *testing.T) {
			out := renderTaskStep(t, cb)
			require.Contains(t, out, "only task `The selected task`")
			require.Contains(t, out, "`sel-1`")
			require.Contains(t, out, "do not start, test, verify or tick any other task")
			require.NotContains(t, out, "Another task")

			whole := renderStep(t, cb)
			require.NotContains(t, whole, "only task")
			require.Contains(t, whole, "#### - [ ] Task: <title>", "a whole-plan run works from the first unchecked task")
			require.Contains(t, whole, "#### - [ ] Phase N.M:", "a plan written before tasks still implements")
		})
	}
}

func TestReadPlanInTaskRunStillReadsTheWholePlan(t *testing.T) {
	out := renderTaskStep(t, readPlan())
	require.Contains(t, out, "only task `The selected task`")
	for _, doc := range []string{"plan file read <plan_name> plan", "plan file read <plan_name> context", "plan file read <plan_name> research"} {
		require.Contains(t, out, doc)
	}
	require.Contains(t, out, "design document the plan references")
}
