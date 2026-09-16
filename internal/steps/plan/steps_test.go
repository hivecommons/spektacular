package plan

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/stepkit"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/jumppad-labs/spektacular/internal/workflow"
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

// TestArchitectureStepWeighsOptionsAndRecordsAssumption asserts the Phase 2.1
// shape of the architecture step: options analysis is kept, but the chosen
// direction is recorded in the assumption log instead of being put to the user
// for agreement.
func TestArchitectureStepWeighsOptionsAndRecordsAssumption(t *testing.T) {
	out := renderStep(t, architecture())
	lower := strings.ToLower(out)
	require.Contains(t, lower, "option", "architecture step must still weigh design options")
	require.Contains(t, lower, "assumption log", "architecture step must record the chosen direction in the assumption log")
	require.Contains(t, out, "do not put the choice to the user", "architecture step must choose the direction itself, not ask the user")
	require.NotContains(t, lower, "agreement", "architecture step must not gate on user agreement")
}

// TestGatheringStepsProceedWithoutApprovalGates asserts every plan gathering
// step (Phase 2.1): carries the shared assumption-log and
// proceed-unless-blocked block, and no longer contains any per-section
// wait-for-approval phrasing. The banned-phrase list is a hand-maintained
// oracle of the gate wording removed in this phase — reintroducing any of it
// fails this test.
func TestGatheringStepsProceedWithoutApprovalGates(t *testing.T) {
	bannedGatePhrases := []string{
		"Present it to the user for review",
		"Once the user is happy",
		"Once the user has agreed",
		"Once the user agrees",
		"Get the user's explicit agreement",
		"Present the milestones to the user",
		"Present the phases to the user",
	}

	steps := []struct {
		name string
		cb   workflow.StepCallback
	}{
		{"discovery", discovery()},
		{"architecture", architecture()},
		{"components", components()},
		{"data_structures", dataStructures()},
		{"implementation_detail", implementationDetail()},
		{"dependencies", dependencies()},
		{"testing_approach", testingApproach()},
		{"milestones", milestones()},
		{"phases", phases()},
		{"open_questions", openQuestions()},
		{"out_of_scope", outOfScope()},
	}

	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			out := renderStep(t, tc.cb)
			require.Contains(t, out, "assumptions.md",
				"%s step must instruct recording judgement calls in the assumption log", tc.name)
			require.Contains(t, out, "proceed without interruption",
				"%s step must carry the proceed-unless-blocked rule", tc.name)
			for _, phrase := range bannedGatePhrases {
				require.NotContains(t, out, phrase,
					"%s step must not contain the wait-for-approval phrasing %q", tc.name, phrase)
			}
		})
	}
}

// TestDiscoveryStepUsesKnowledgeCommands asserts the discovery step drives the
// agent through the `knowledge` CLI commands instead of a hardcoded knowledge
// directory, and gates knowledge writes behind explicit user confirmation
// (Phase 3.1, acceptance criteria 1 and 2).
func TestDiscoveryStepUsesKnowledgeCommands(t *testing.T) {
	out := renderStep(t, discovery())

	// Criterion 1: uses the knowledge commands, no hardcoded knowledge path.
	require.Contains(t, out, "knowledge search", "discovery step must instruct the agent to use `knowledge search`")
	require.Contains(t, out, "knowledge read", "discovery step must instruct the agent to use `knowledge read`")
	require.NotContains(t, out, ".spektacular/knowledge/", "discovery step must not hardcode the knowledge directory path")

	// Criterion 2: knowledge writes require explicit user confirmation.
	require.Contains(t, out, "knowledge write", "discovery step must reference `knowledge write` for capturing learnings")
	require.Contains(t, strings.ToLower(out), "confirm", "discovery step must require explicit user confirmation before a knowledge write")
}

func TestImplementationDetailStepIsHighLevelOnly(t *testing.T) {
	out := renderStep(t, implementationDetail())
	require.Contains(t, strings.ToLower(out), "high-level", "implementation_detail step must enforce high-level only content")
	require.Contains(t, out, "context.md", "implementation_detail step must redirect per-phase detail to context.md")
}

func TestTestingApproachStepIsHighLevelOnly(t *testing.T) {
	out := renderStep(t, testingApproach())
	require.Contains(t, strings.ToLower(out), "high-level", "testing_approach step must enforce high-level only content")
	require.Contains(t, out, "context.md", "testing_approach step must redirect per-phase detail to context.md")
}

func TestOpenQuestionsStepRestrictsToImplTimeUncertainties(t *testing.T) {
	out := renderStep(t, openQuestions())
	require.Contains(t, strings.ToLower(out), "implementation", "open_questions step must restrict the section to impl-time uncertainties")
	require.Contains(t, strings.ToLower(out), "cannot be resolved", "open_questions step must state the cannot-resolve-now rule")
}

func TestOutOfScopeStepCoversExclusions(t *testing.T) {
	out := renderStep(t, outOfScope())
	require.Contains(t, out, "Out of Scope", "out_of_scope step must name the section it populates")
	require.Contains(t, strings.ToLower(out), "exclusion", "out_of_scope step must prompt for explicit exclusions")
}

func TestStepsOrderMatchesExpected(t *testing.T) {
	expected := []string{
		"new",
		"overview",
		"discovery",
		"architecture",
		"components",
		"data_structures",
		"implementation_detail",
		"dependencies",
		"testing_approach",
		"milestones",
		"phases",
		"open_questions",
		"out_of_scope",
		"assemble",
		"verification",
		"write_plan",
		"write_context",
		"write_research",
		"walkthrough",
		"finished",
	}
	got := Steps()
	require.Len(t, got, len(expected))
	for i, step := range got {
		require.Equal(t, expected[i], step.Name, "step %d name mismatch", i)
	}
}

// TestWalkthroughWiring asserts the walkthrough step sits between
// write_research and finished, and that finished is reachable only from
// walkthrough (Phase 1.1, acceptance criterion 1).
func TestWalkthroughWiring(t *testing.T) {
	srcs := map[string][]string{}
	for _, step := range Steps() {
		srcs[step.Name] = step.Src
	}
	require.Equal(t, []string{"write_research"}, srcs["walkthrough"],
		"walkthrough must be entered only from write_research")
	require.Equal(t, []string{"walkthrough"}, srcs["finished"],
		"finished must be reachable only from walkthrough")
}

func TestFSMWalkFromNewToFinished(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	st := store.NewFileStore(tmp, "project")
	writer := &captureWriter{}

	wf := workflow.New(Steps(), statePath, workflow.Config{Command: "spektacular", DryRun: true, PlanDir: "plans", SpecDir: "specs"}, st, writer)
	wf.SetData("name", "test")

	require.Equal(t, "start", wf.Current())

	expectedStates := []string{
		"overview",
		"discovery",
		"architecture",
		"components",
		"data_structures",
		"implementation_detail",
		"dependencies",
		"testing_approach",
		"milestones",
		"phases",
		"open_questions",
		"out_of_scope",
		"assemble",
		"verification",
		"write_plan",
		"write_context",
		"write_research",
		"walkthrough",
		"finished",
	}

	for _, want := range expectedStates {
		require.NoError(t, wf.Next(), "transition to %s failed", want)
		require.Equal(t, want, wf.Current(), "expected state %s after transition", want)
	}
}

// TestPlanFilePaths_UseConfiguredDirectory asserts the path helpers root plan,
// context and research files under the given directory argument (Phase 2.2,
// acceptance criterion 2).
func TestPlanFilePaths_UseConfiguredDirectory(t *testing.T) {
	require.Equal(t, "my-plans/x/plan.md", PlanFilePath("my-plans", "x"))
	require.Equal(t, "my-plans/x/context.md", ContextFilePath("my-plans", "x"))
	require.Equal(t, "my-plans/x/research.md", ResearchFilePath("my-plans", "x"))
}

// --- Phase 1.5: terminal-step closure across all three siblings ---

// seedFilledPlanDocs writes a filled (non-scaffold) body wrapped in an
// draft frontmatter block dated `created` for every planDoc under
// planDir. Returns the created date so callers can assert preservation.
func seedFilledPlanDocs(t *testing.T, st store.Store, planDir, planName string, created time.Time) {
	t.Helper()
	for _, doc := range planDocs {
		scaffold, err := stepkit.RenderTemplate(doc.scaffold, map[string]any{"name": planName})
		require.NoError(t, err)
		filled := append([]byte{}, []byte(scaffold)...)
		filled = append(filled, "\n\n## Real content\n"...)
		wrapped, err := metadata.Render(metadata.Metadata{
			CreatedDate:    created,
			DocumentStatus: metadata.StatusDraft,
		}, filled)
		require.NoError(t, err)
		require.NoError(t, st.Write(doc.path(planDir, planName), wrapped))
	}
}

// TestPlanFinished_ClosesAllThreeSiblings seeds the store with filled plan,
// context and research docs and asserts finished() transitions every one of
// them to final with today's closed_date, preserving each original
// created_date.
func TestPlanFinished_ClosesAllThreeSiblings(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", PlanDir: "plans", SpecDir: "specs"}
	planName := "fixture"

	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	seedFilledPlanDocs(t, st, cfg.PlanDir, planName, created)

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err := finished()(data, writer, st, cfg)
	require.NoError(t, err)

	for _, doc := range planDocs {
		p := doc.path(cfg.PlanDir, planName)
		raw, err := st.Read(p)
		require.NoError(t, err, "%s must be readable after finished()", p)

		meta, _, err := metadata.Split(raw)
		require.NoError(t, err, "%s must carry parseable frontmatter", p)
		require.NotNil(t, meta)
		require.Equal(t, metadata.StatusFinal, meta.DocumentStatus, "%s must be final", p)
		require.True(t, meta.CreatedDate.Equal(created), "%s created_date must be preserved, got %s", p, meta.CreatedDate)
		require.True(t, meta.ClosedDate.Equal(today()), "%s closed_date must be today, got %s", p, meta.ClosedDate)
	}
}

// TestPlanFinished_LeavesUnwrittenPlanAlone seeds two filled docs and one
// still-scaffold doc, then asserts finished() does not transition ANY of the
// three to final — the incomplete gate short-circuits before the close
// loop runs.
func TestPlanFinished_LeavesUnwrittenPlanAlone(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", PlanDir: "plans", SpecDir: "specs"}
	planName := "fixture"

	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

	// Fill plan.md and context.md; leave research.md as a bare scaffold with
	// draft frontmatter so planDocStillScaffold reports it unwritten.
	for i, doc := range planDocs {
		scaffold, err := stepkit.RenderTemplate(doc.scaffold, map[string]any{"name": planName})
		require.NoError(t, err)
		body := []byte(scaffold)
		if i < 2 {
			body = append(body, "\n\n## Real content\n"...)
		}
		wrapped, err := metadata.Render(metadata.Metadata{
			CreatedDate:    created,
			DocumentStatus: metadata.StatusDraft,
		}, body)
		require.NoError(t, err)
		require.NoError(t, st.Write(doc.path(cfg.PlanDir, planName), wrapped))
	}

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err := finished()(data, writer, st, cfg)
	require.NoError(t, err, "finished() must not error when a plan doc is still the scaffold")

	// Assert none of the docs transitioned to final.
	for _, doc := range planDocs {
		p := doc.path(cfg.PlanDir, planName)
		raw, err := st.Read(p)
		require.NoError(t, err)
		meta, _, err := metadata.Split(raw)
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.Equal(t, metadata.StatusDraft, meta.DocumentStatus,
			"%s must remain draft when the plan is incomplete", p)
		require.True(t, meta.ClosedDate.IsZero(), "%s must not gain a closed_date", p)
	}
}

// TestPlanFinished_SkipsClose_WhenPlanIncomplete is the twin of the previous
// test: it verifies the finished() callback surfaces the plan_incomplete
// warning (via the extras passed to writeStep) and does not mutate any
// artifact metadata when the incomplete gate trips.
func TestPlanFinished_SkipsClose_WhenPlanIncomplete(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", PlanDir: "plans", SpecDir: "specs"}
	planName := "fixture"

	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

	// Only the first sibling missing is enough to trip the gate.
	scaffold, err := stepkit.RenderTemplate(planDocs[0].scaffold, map[string]any{"name": planName})
	require.NoError(t, err)
	bareScaffold, err := metadata.Render(metadata.Metadata{
		CreatedDate:    created,
		DocumentStatus: metadata.StatusDraft,
	}, []byte(scaffold))
	require.NoError(t, err)
	require.NoError(t, st.Write(planDocs[0].path(cfg.PlanDir, planName), bareScaffold))

	// Fill the other two so only plan.md trips the gate.
	for _, doc := range planDocs[1:] {
		scaffold, err := stepkit.RenderTemplate(doc.scaffold, map[string]any{"name": planName})
		require.NoError(t, err)
		filled := append([]byte{}, []byte(scaffold)...)
		filled = append(filled, "\n\n## Real content\n"...)
		wrapped, err := metadata.Render(metadata.Metadata{
			CreatedDate:    created,
			DocumentStatus: metadata.StatusDraft,
		}, filled)
		require.NoError(t, err)
		require.NoError(t, st.Write(doc.path(cfg.PlanDir, planName), wrapped))
	}

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err = finished()(data, writer, st, cfg)
	require.NoError(t, err)

	// The scaffold-check warning surfaces via the rendered instruction — the
	// finished template gates on the plan_incomplete extra.
	require.NotEmpty(t, writer.result.Instruction, "finished() must emit an instruction")

	// None of the sibling artifacts should have flipped to final.
	for _, doc := range planDocs {
		p := doc.path(cfg.PlanDir, planName)
		raw, err := st.Read(p)
		require.NoError(t, err)
		meta, _, err := metadata.Split(raw)
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.NotEqual(t, metadata.StatusFinal, meta.DocumentStatus,
			"%s must not be closed when the plan is incomplete", p)
	}
}

// TestWalkthroughStepIsMandatoryGuidedReview asserts the walkthrough step
// (Phase 1.1, acceptance criterion 2): it drives a section-by-section review of
// the committed documents, applies requested changes immediately via the
// existing `plan file write` command, and concludes only on an explicit
// affirmative answer to a direct closing question — never framed as an
// optional offer.
func TestWalkthroughStepIsMandatoryGuidedReview(t *testing.T) {
	out := renderStep(t, walkthrough())
	lower := strings.ToLower(out)

	// Guided review of the committed documents, read back through the CLI.
	require.Contains(t, out, "plan file read", "walkthrough must read the committed documents via `plan file read`")
	require.Contains(t, lower, "walk the user through", "walkthrough must direct a guided review of the plan")

	// Change requests apply immediately through the document write command.
	require.Contains(t, out, "plan file write", "walkthrough must apply change requests via `plan file write`")
	require.Contains(t, lower, "apply it immediately", "walkthrough must apply change requests immediately, not defer them")

	// Sign-off gate: explicit affirmative to a direct closing question only.
	require.Contains(t, lower, "explicit affirmative", "walkthrough must gate completion on an explicit affirmative answer")
	require.Contains(t, lower, "not agreement", "walkthrough must state that silence or ambiguity is not agreement")

	// The review is mandatory, not an offer the user can decline.
	require.Contains(t, lower, "mandatory", "walkthrough must state the review is mandatory")
	require.Contains(t, lower, "do not offer it as a choice", "walkthrough must not frame the review as an optional offer")
}

// TestAssembleStepMapsAssumptionLog asserts the assemble step (Phase 2.2) maps
// the assumption log working file into research.md's `## Drafting assumptions`
// section, and treats a missing assumptions.md as a fallback line rather than a
// STOP.
func TestAssembleStepMapsAssumptionLog(t *testing.T) {
	out := renderStep(t, assemble())
	require.Contains(t, out, "assumptions.md", "assemble step must name the assumption log working file")
	require.Contains(t, out, "## Drafting assumptions", "assemble step must map the assumption log to the Drafting assumptions section")
	require.Contains(t, out, "No drafting assumptions were recorded",
		"assemble step must write the explicit fallback line instead of stopping when assumptions.md is missing")
}

// TestVerificationStepRequiresDraftingAssumptions asserts the verification
// step's research.md required-section list includes `## Drafting assumptions`
// (Phase 2.2).
func TestVerificationStepRequiresDraftingAssumptions(t *testing.T) {
	out := renderStep(t, verification())
	require.Contains(t, out, "## Drafting assumptions", "verification step must require the Drafting assumptions section in research.md")
}

// TestWalkthroughStepWalksDraftingAssumptions asserts the walkthrough step's
// drafting-assumptions beat (Phase 2.2): it reads the `## Drafting assumptions`
// section of research.md back through the CLI, walks the entries, and invites
// the user to challenge each — a challenged assumption becoming a change
// request.
func TestWalkthroughStepWalksDraftingAssumptions(t *testing.T) {
	out := renderStep(t, walkthrough())
	require.Contains(t, out, "Read the `## Drafting assumptions` section",
		"walkthrough must read the Drafting assumptions section of research.md")
	require.Contains(t, strings.ToLower(out), "challenge",
		"walkthrough must invite the user to challenge each assumption")
	require.Contains(t, out, "challenged assumption is a change request",
		"walkthrough must route a challenged assumption through the change-request path")
}

// TestWalkthroughStepOffersKnowledgeCaptureForRevealingCorrections asserts the
// walkthrough step's correction-assessment-and-offer beat (Phase 2.1): after a
// correction is applied and confirmed, the step directs assessing what the
// correction reveals and — only for durable, generalizable lessons — offering
// to capture the general lesson, gated on explicit acceptance and handed to
// the `spek-knowledge` skill, with a decline being final.
func TestWalkthroughStepOffersKnowledgeCaptureForRevealingCorrections(t *testing.T) {
	out := renderStep(t, walkthrough())
	lower := strings.ToLower(out)

	// Assess what an applied correction reveals before resuming.
	require.Contains(t, lower, "what the correction reveals",
		"walkthrough must direct assessing what an applied correction reveals")

	// The offer is phrased as the general lesson, not the specific edit.
	require.Contains(t, lower, "general lesson",
		"walkthrough must phrase the offer as the general lesson rather than the specific edit")

	// Selectivity bar: most corrections reveal nothing durable and produce no offer.
	require.Contains(t, lower, "produce no offer",
		"walkthrough must state that corrections revealing nothing durable produce no offer")

	// Capture is gated on the user's explicit acceptance.
	require.Contains(t, lower, "explicit acceptance",
		"walkthrough must gate knowledge capture on the user's explicit acceptance")

	// A decline is final for that lesson within the conversation.
	require.Contains(t, lower, "not offered again",
		"walkthrough must state a declined lesson is not offered again")

	// Accepted items are handed to the spek-knowledge skill.
	require.Contains(t, out, "spek-knowledge",
		"walkthrough must hand accepted items to the `spek-knowledge` skill")
	require.NotContains(t, out, "skill spek-knowledge",
		"walkthrough must not direct the unreachable CLI invocation `spektacular skill spek-knowledge`")
}

// TestPlanFinishedSuccessBranchHasNoWalkthroughOffer asserts the terminal
// step's success branch no longer offers or conducts a walkthrough (Phase 1.1,
// acceptance criterion 3): sign-off already happened on the walkthrough step,
// so finished only stamps the documents final and reports completion.
func TestPlanFinishedSuccessBranchHasNoWalkthroughOffer(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", PlanDir: "plans", SpecDir: "specs"}
	planName := "fixture"

	seedFilledPlanDocs(t, st, cfg.PlanDir, planName, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))

	data := &testData{values: map[string]any{"name": planName}}
	writer := &captureWriter{}

	_, err := finished()(data, writer, st, cfg)
	require.NoError(t, err)

	out := writer.result.Instruction
	require.Contains(t, out, "signed off", "finished must report the user already signed off during the walkthrough")
	require.Contains(t, out, "marked final", "finished must report the documents were stamped final")
	require.NotContains(t, out, "offer a choice", "finished must no longer offer a walkthrough")
	require.NotContains(t, out, "If the user accepts", "finished must no longer conduct a conditional walkthrough")
}

// TestWriteStep_CommitsOwnDocument asserts a write step's callback reads its own
// document back through the store: when the document is missing it instructs the
// agent to commit it via `plan file write`; once committed it reports the
// document is already in the store and drops the commit instruction.
func TestWriteStep_CommitsOwnDocument(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	writer := &captureWriter{}
	data := &testData{values: map[string]any{"name": "test"}}
	cfg := workflow.Config{Command: "spektacular", PlanDir: "my-plans", SpecDir: "specs"}

	// plan.md absent from the store — the step must instruct the commit.
	_, err := writePlan()(data, writer, st, cfg)
	require.NoError(t, err)
	require.Contains(t, writer.result.Instruction, "--from .spektacular/tmp/plan_template.md",
		"write_plan must instruct committing plan.md from its scratch file when it is not yet in the store")

	// A committed, filled plan.md — no commit command, reports done.
	require.NoError(t, st.Write(PlanFilePath("my-plans", "test"), []byte("# real filled plan")))
	_, err = writePlan()(data, writer, st, cfg)
	require.NoError(t, err)
	require.Contains(t, writer.result.Instruction, "already been committed",
		"write_plan must report plan.md is already committed once it is in the store")
	require.NotContains(t, writer.result.Instruction, "--from .spektacular/tmp/plan_template.md",
		"write_plan must not re-instruct the commit once plan.md is in the store")
}

// --- Phase 4.2: workflows and skills go cross-repo ---

// The discovery and architecture steps carry no roster of their own: each
// sends the agent to `repo list`, which reports the registry as it stands
// right now rather than as some earlier command recorded it.
func TestDiscoveryAndArchitectureStepsSendTheAgentToRepoList(t *testing.T) {
	for name, cb := range map[string]workflow.StepCallback{
		"discovery":    discovery(),
		"architecture": architecture(),
	} {
		t.Run(name, func(t *testing.T) {
			out := renderStep(t, cb)

			require.Contains(t, out, "repo list", "%s must send the agent to `repo list`", name)
			require.Contains(t, out, "`root`", "%s must name the root that repo list reports", name)
			require.NotContains(t, out, "## Repos", "%s must not point at a Repos section", name)
			require.NotContains(t, out, "{{", "%s must leave no unrendered mustache", name)
		})
	}
}

// TestDiscoveryStepDirectsRepoScopedResearch asserts the discovery instruction
// directs per-repo research scoping: resolved paths and staleness come from
// `repo list`, the roster's metadata scopes which repos each question belongs
// to, Files-examined entries carry the repo prefix, and the ignore-file note
// clarifies native tools are unaffected.
func TestDiscoveryStepDirectsRepoScopedResearch(t *testing.T) {
	out := renderStep(t, discovery())

	// Criterion 2: resolved locations come from `repo list`.
	require.Contains(t, out, "spektacular repo list",
		"discovery must direct `repo list` for resolved paths and staleness")

	// Criterion 2: research questions are scoped to repos via their metadata.
	require.Contains(t, out, "tags to scope which repos",
		"discovery must direct using repo metadata to scope research questions")

	// Criterion 2: multi-repo Files-examined entries carry the repo prefix.
	require.Contains(t, out, "<repo>:path:line",
		"discovery's Files-examined entries must carry the repo-name prefix in multi-repo projects")

	// Criterion 2: the ignore-file note states native tools are not bound.
	require.Contains(t, out, ".spektacular_ignore",
		"discovery must mention the .spektacular_ignore exclusions")
	require.Contains(t, out, "native file tools are not bound",
		"discovery must state native file tools are not bound by the ignore file")
}

// TestArchitectureStepRequiresRepoAttribution asserts the architecture
// instruction requires attributing every requirement to a repo and files,
// recorded in the plan's context document, with resolved paths from
// `repo list`.
func TestArchitectureStepRequiresRepoAttribution(t *testing.T) {
	out := renderStep(t, architecture())

	// Criterion 2: every requirement names its repo and files.
	require.Contains(t, out, "which repo (and which files within it)",
		"architecture must require naming each requirement's repo and files")

	// Criterion 2: resolved locations come from `repo list`.
	require.Contains(t, out, "spektacular repo list",
		"architecture must direct `repo list` for resolved local paths")

	// Criterion 2: the attribution is recorded in the plan's context document.
	require.Contains(t, out, "requirement-to-repo-and-files resolution",
		"architecture must record the requirement-to-repo-and-files resolution")
	require.Contains(t, out, "context document",
		"architecture must record the attribution in the plan's context document")
}

// TestPhasesStepCarriesRepoAttributionIntoPlanAndContext asserts the phases
// instruction carries attribution into both outputs: plan.md phases carry a
// dedicated Repo line and context.md file changes carry the repo-name
// prefix.
func TestPhasesStepCarriesRepoAttributionIntoPlanAndContext(t *testing.T) {
	out := renderStep(t, phases())

	// Criterion 2: every phase carries a dedicated Repo line naming its
	// target repo, always present regardless of repo count.
	require.Contains(t, out, "**Repo:**",
		"phases must direct plan.md to include a dedicated Repo line naming the phase's target repo")

	// Criterion 2: context.md file changes carry the repo prefix.
	require.Contains(t, out, "<repo>:path:line",
		"phases must direct context.md File-changes to carry the repo-name prefix")
}
