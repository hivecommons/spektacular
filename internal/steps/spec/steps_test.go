package spec

import (
	"os"
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

func TestStepsOrderMatchesExpected(t *testing.T) {
	expected := []string{
		"new",
		"interview",
		"overview",
		"requirements",
		"acceptance_criteria",
		"constraints",
		"technical_approach",
		"success_metrics",
		"non_goals",
		"verification",
		"finished",
	}
	got := Steps()
	require.Len(t, got, len(expected))
	for i, step := range got {
		require.Equal(t, expected[i], step.Name, "step %d name mismatch", i)
	}
}

func TestFSMWalkFromNewToFinished(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	st := store.NewFileStore(tmp, "project")
	writer := &captureWriter{}

	wf := workflow.New(Steps(), statePath, workflow.Config{Command: "spektacular", DryRun: true}, st, writer)
	wf.SetData("name", "test")
	wf.SetData("spec_template", "spec content")

	require.Equal(t, "start", wf.Current())

	// After the "new" step change, the workflow stays at "new" state and returns
	// an instruction. The next transition advances to "interview".
	expectedStates := []string{
		"new",
		"interview",
		"overview",
		"requirements",
		"acceptance_criteria",
		"constraints",
		"technical_approach",
		"success_metrics",
		"non_goals",
		"verification",
		"finished",
	}

	for _, want := range expectedStates {
		require.NoError(t, wf.Next(), "transition to %s failed", want)
		require.Equal(t, want, wf.Current(), "expected state %s after transition", want)
	}
}

func TestOverviewStepRendersInstruction(t *testing.T) {
	out := renderStep(t, overview())
	require.NotEmpty(t, out)
}

func TestVerificationStepPassesSpecTemplate(t *testing.T) {
	// Verification must render with a spec_template extra var populated from
	// the scaffold so the template can embed the scaffold body.
	tmp := t.TempDir()
	data := &testData{values: map[string]any{"name": "test"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err := verification()(data, writer, st, workflow.Config{Command: "spektacular"})
	require.NoError(t, err)
	require.NotEmpty(t, writer.result.Instruction)
}

func TestNewStepWritesScaffold(t *testing.T) {
	tmp := t.TempDir()

	// Create .spektacular directory for working-context.md
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// Change to temp directory so relative path resolution works
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(origWd)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	next, err := new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)
	require.Equal(t, "", next, "new step should return empty string when using writeStep")
	require.True(t, st.Exists(SpecFilePath("specs", "fixture")))
}

// TestSpecFilePath_UsesConfiguredDirectory asserts the path helper roots the
// spec file under the given directory argument (Phase 2.2, criterion 1).
func TestSpecFilePath_UsesConfiguredDirectory(t *testing.T) {
	require.Equal(t, "my-specs/x.md", SpecFilePath("my-specs", "x"))
}

// TestNewStep_WritesUnderConfiguredSpecDir runs the new callback with a
// non-default SpecDir and asserts the spec file lands under that directory
// (Phase 2.2, criterion 1).
func TestNewStep_WritesUnderConfiguredSpecDir(t *testing.T) {
	tmp := t.TempDir()

	// Create .spektacular directory for working-context.md
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// Change to temp directory so relative path resolution works
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(origWd)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err = new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "my-specs"})
	require.NoError(t, err)
	require.True(t, st.Exists(SpecFilePath("my-specs", "fixture")), "spec must land under my-specs")
	require.False(t, st.Exists(SpecFilePath("specs", "fixture")), "spec must not land under default specs")
}

// TestNewStep_ResetsWorkingContext verifies that the new step drops the
// previous session's working context, leaving .spektacular/working-context.md
// empty for this run. The CLI writes no roster into it: `repo list` is the
// live source for where code lives. A legacy .spektacular/context.md is
// deliberately ignored: left byte-for-byte as it was.
func TestNewStep_ResetsWorkingContext(t *testing.T) {
	tmp := setupNewStepEnv(t)

	spektacularDir := filepath.Join(tmp, ".spektacular")
	workingContextPath := filepath.Join(spektacularDir, "working-context.md")
	require.NoError(t, os.WriteFile(workingContextPath, []byte("old context"), 0644))
	legacyPath := filepath.Join(spektacularDir, "context.md")
	require.NoError(t, os.WriteFile(legacyPath, []byte("legacy"), 0644))

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err := new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	content, err := os.ReadFile(workingContextPath)
	require.NoError(t, err)
	require.Empty(t, string(content), "the previous session's working context must be dropped, leaving the file to the agent")

	legacy, err := os.ReadFile(legacyPath)
	require.NoError(t, err)
	require.Equal(t, "legacy", string(legacy), "a legacy .spektacular/context.md must be left untouched")
}

// TestNewStep_CreatesNoLegacyContextFile verifies that, in a project with no
// .spektacular/context.md, the new step does not create one: the reset
// targets working-context.md only.
func TestNewStep_CreatesNoLegacyContextFile(t *testing.T) {
	tmp := setupNewStepEnv(t)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err := new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmp, ".spektacular", "context.md"))
	require.ErrorIs(t, err, os.ErrNotExist, "no legacy context.md may be created")
}

// TestNewStep_ReturnsInstructionToWriteContext verifies that the new step
// returns an instruction (via writeStep) telling the agent to write
// conversation context to .spektacular/working-context.md (Phase 2.1).
func TestNewStep_ReturnsInstructionToWriteContext(t *testing.T) {
	tmp := t.TempDir()

	// Create .spektacular directory
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// Change to the temp directory so relative path resolution works
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(origWd)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err = new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	// Verify instruction was written
	require.NotEmpty(t, writer.result.Instruction, "new step should return an instruction")
	require.Contains(t, writer.result.Instruction, ".spektacular/working-context.md", "instruction should name the working-context file")
	require.Contains(t, writer.result.Instruction, "conversation context", "instruction should mention conversation context")
}

// TestNewStep_InstructionIncludesDetailedFormat verifies that the instruction
// specifies what to capture: problem, requirements, constraints, alternatives,
// exact phrasing (Phase 2.1 acceptance criteria).
func TestNewStep_InstructionIncludesDetailedFormat(t *testing.T) {
	tmp := t.TempDir()

	// Create .spektacular directory
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// Change to the temp directory so relative path resolution works
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(origWd)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err = new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	instruction := strings.ToLower(writer.result.Instruction)
	require.Contains(t, instruction, "problem", "instruction should specify capturing the problem")
	require.Contains(t, instruction, "requirements", "instruction should specify capturing requirements")
	require.Contains(t, instruction, "constraints", "instruction should specify capturing constraints")
	require.Contains(t, instruction, "alternatives", "instruction should specify capturing alternatives")
	require.Contains(t, instruction, "exact phrasing", "instruction should specify capturing exact phrasing")
}

// --- Phase 1.5: metadata stamping and terminal-step closure ---

// setupNewStepEnv creates a temp dir with an empty `.spektacular/` directory,
// chdirs into it (restoring the original working directory on cleanup) and
// returns the dir. new() resets working-context.md at a relative path off the cwd,
// so callers exercising new() must run inside a suitable working directory.
func setupNewStepEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	return tmp
}

// TestSpecNew_StampsMetadataOnFirstWrite asserts new() routes its scaffold
// write through metadata.Merge so the stored spec carries a draft
// frontmatter block with created_date=today and no closed_date.
func TestSpecNew_StampsMetadataOnFirstWrite(t *testing.T) {
	tmp := setupNewStepEnv(t)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err := new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	raw, err := st.Read(SpecFilePath("specs", "fixture"))
	require.NoError(t, err)

	meta, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.NotNil(t, meta, "new() must attach frontmatter to the initial scaffold write")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus, "initial write must be draft")
	require.True(t, meta.CreatedDate.Equal(today()), "created_date must be today, got %s", meta.CreatedDate)
	require.True(t, meta.ClosedDate.IsZero(), "closed_date must be absent on a draft artifact")
}

// TestSpecFinished_ClosesTheSpec seeds the store with a filled (non-scaffold)
// spec body carrying a draft frontmatter block dated in the past and
// asserts finished() transitions the metadata to final with today's
// closed_date while preserving the seeded created_date.
func TestSpecFinished_ClosesTheSpec(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", SpecDir: "specs"}

	// Seed a filled body with draft frontmatter dated in the past.
	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	filled, err := metadata.Render(metadata.Metadata{
		CreatedDate:    created,
		DocumentStatus: metadata.StatusDraft,
	}, []byte("# Fixture\n\nFilled body that is not the scaffold.\n"))
	require.NoError(t, err)
	require.NoError(t, st.Write(SpecFilePath("specs", "fixture"), filled))

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}

	_, err = finished()(data, writer, st, cfg)
	require.NoError(t, err)

	raw, err := st.Read(SpecFilePath("specs", "fixture"))
	require.NoError(t, err)

	meta, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, metadata.StatusFinal, meta.DocumentStatus, "finished() must transition status to final")
	require.True(t, meta.CreatedDate.Equal(created), "created_date must be preserved from seed, got %s", meta.CreatedDate)
	require.True(t, meta.ClosedDate.Equal(today()), "closed_date must be today, got %s", meta.ClosedDate)
}

// TestSpecFinished_LeavesUnwrittenSpecAlone writes a scaffold-only spec (still
// wearing a draft frontmatter block from a hypothetical new() call) and
// asserts finished() does NOT transition it to final — the still-scaffold
// gate should prevent the close.
func TestSpecFinished_LeavesUnwrittenSpecAlone(t *testing.T) {
	tmp := t.TempDir()
	st := store.NewFileStore(tmp, "project")
	cfg := workflow.Config{Command: "spektacular", SpecDir: "specs"}

	scaffold, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": "fixture"})
	require.NoError(t, err)
	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	seed, err := metadata.Render(metadata.Metadata{
		CreatedDate:    created,
		DocumentStatus: metadata.StatusDraft,
	}, []byte(scaffold))
	require.NoError(t, err)
	require.NoError(t, st.Write(SpecFilePath("specs", "fixture"), seed))

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}

	_, err = finished()(data, writer, st, cfg)
	require.NoError(t, err, "finished() must not crash when the spec is still the scaffold")

	// The stored bytes must be byte-identical to the seed — no metadata mutation.
	raw, err := st.Read(SpecFilePath("specs", "fixture"))
	require.NoError(t, err)
	require.Equal(t, string(seed), string(raw), "still-scaffold gate must prevent any write on finished()")

	meta, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus, "still-scaffold artifact must remain draft")
	require.True(t, meta.ClosedDate.IsZero(), "still-scaffold artifact must not gain a closed_date")
}

// TestSpecStillScaffold_FrontmatterTolerant confirms specStillScaffold strips a
// leading frontmatter block before comparing against the rendered scaffold so
// the wrapped and bare scaffolds are treated equivalently, and a filled body
// wrapped in frontmatter is still reported as no-longer-scaffold.
func TestSpecStillScaffold_FrontmatterTolerant(t *testing.T) {
	const specName = "fixture"
	cfg := workflow.Config{Command: "spektacular", SpecDir: "specs"}

	scaffold, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": specName})
	require.NoError(t, err)
	scaffoldBytes := []byte(scaffold)

	fm := metadata.Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
	}

	t.Run("scaffold_with_frontmatter_is_still_scaffold", func(t *testing.T) {
		st := store.NewFileStore(t.TempDir(), "project")
		wrapped, err := metadata.Render(fm, scaffoldBytes)
		require.NoError(t, err)
		require.NoError(t, st.Write(SpecFilePath(cfg.SpecDir, specName), wrapped))

		still, err := specStillScaffold(st, cfg, specName)
		require.NoError(t, err)
		require.True(t, still, "scaffold wrapped in frontmatter must be reported as still scaffold")
	})

	t.Run("filled_body_with_frontmatter_is_not_scaffold", func(t *testing.T) {
		st := store.NewFileStore(t.TempDir(), "project")
		filled := append([]byte{}, scaffoldBytes...)
		filled = append(filled, "\n\n## Real content\n"...)
		wrapped, err := metadata.Render(fm, filled)
		require.NoError(t, err)
		require.NoError(t, st.Write(SpecFilePath(cfg.SpecDir, specName), wrapped))

		still, err := specStillScaffold(st, cfg, specName)
		require.NoError(t, err)
		require.False(t, still, "a filled body wrapped in frontmatter must not be reported as still scaffold")
	})
}

// TestNewStep_InstructionIncludesCaveat verifies that the instruction includes
// the caveat to skip if no meaningful context exists (Phase 2.1 acceptance criteria).
func TestNewStep_InstructionIncludesCaveat(t *testing.T) {
	tmp := t.TempDir()

	// Create .spektacular directory
	spektacularDir := filepath.Join(tmp, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// Change to the temp directory so relative path resolution works
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	defer os.Chdir(origWd)

	data := &testData{values: map[string]any{"name": "fixture"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	_, err = new()(data, writer, st, workflow.Config{Command: "spektacular", SpecDir: "specs"})
	require.NoError(t, err)

	instruction := writer.result.Instruction
	require.Contains(t, instruction, "no meaningful context", "instruction should include caveat about skipping if no context")
}

// --- Phase 1.2: give the interview access to the project's repo roster and a
// cross-repo question ---

// The interview step carries no roster of its own: it sends the agent to
// `repo list`, the live source for which repos exist and where their code is.
func TestInterviewStepSendsTheAgentToRepoList(t *testing.T) {
	out := renderStep(t, interview())

	require.Contains(t, out, "repo list", "interview must send the agent to `repo list`")
	require.Contains(t, out, "`root`", "interview must name the root that repo list reports")
	require.NotContains(t, out, "## Repos", "interview must not point at a Repos section")
	require.NotContains(t, out, "{{", "interview must leave no unrendered mustache")
}

// TestInterviewStepDirectsCrossRepoQuestion asserts the interview
// instruction's prose directs asking a cross-repo impact question — shaped by
// what each OTHER repo's role/description actually is, not generically —
// whenever the feature reads as focused on one repo and more than one repo is
// registered. This is instructional prose in the template rather than
// dynamic per-invocation behavior, so the assertion is a stable-substring
// content check against the actual template wording
// (templates/steps/spec/00b-interview.md).
func TestInterviewStepDirectsCrossRepoQuestion(t *testing.T) {
	repos := []any{
		map[string]any{
			"name":        "billing-api",
			"description": "the payments backend",
			"role":        "backend",
			"tags":        "go, api",
		},
		map[string]any{
			"name":        "docs-site",
			"description": "the user documentation",
			"role":        "documentation",
			"tags":        "docs",
		},
	}

	out := renderStepWithData(t, interview(), map[string]any{"name": "test", "repos": repos})

	require.Contains(t, out,
		"ask at least one question about whether it also needs changes in another registered repo",
		"interview must direct asking a cross-repo impact question when the feature reads as focused on one repo")
	require.Contains(t, out,
		"Shape the question by what that other repo actually is, not generically",
		"interview must direct shaping the cross-repo question by the other repo's actual role/description")
}

// --- Phase 3.2: the technical-approach step offers to capture a settled
// design as a design document ---

// TestTechnicalApproachStepOffersDesignCapture asserts the rendered
// technical-approach instruction carries the design-capture offer introduced
// in Phase 3.2: the offer itself, the three-part bar that gates it, and all
// three decision outcomes. The expected strings are hand-copied from
// templates/steps/spec/05-technical_approach.md.
func TestTechnicalApproachStepOffersDesignCapture(t *testing.T) {
	out := renderStep(t, technicalApproach())

	require.Contains(t, out, "**When the design is settled, offer to capture it rather than compress it away.**",
		"technical_approach must offer to capture a settled design instead of compressing it away")
	require.Contains(t, out, "Offer, never write unprompted.",
		"technical_approach must state the agent offers and never writes unprompted")

	// The three-part bar: settled, worked, and unreadable if inlined.
	require.Contains(t, out, "The bar is high, and all three parts must hold",
		"technical_approach must state the capture bar has three parts that all must hold")
	require.Contains(t, out, "the detail is **settled**",
		"technical_approach must require the detail be settled")
	require.Contains(t, out, "it is **worked**",
		"technical_approach must require the detail be a worked shape")
	require.Contains(t, out, "**make the spec unreadable if written inline**",
		"technical_approach must require the detail would make the spec unreadable inline")

	// All three outcomes of the offer.
	require.Contains(t, out, "- **Accept**", "technical_approach must name the accept outcome")
	require.Contains(t, out, "- **Defer**", "technical_approach must name the defer outcome")
	require.Contains(t, out, "- **Decline**", "technical_approach must name the decline outcome")

	// Phase 3.3: the offer covers a design that has not been written down yet,
	// not only one the user already holds. Without this framing the agent reads
	// the offer as conditional on a document already existing and never makes
	// it for a design settled in the conversation.
	require.Contains(t, out, "This covers two situations, not one.",
		"technical_approach must say the offer covers both an unwritten and an already-written design")
}

// TestTechnicalApproachStepNamesAllDesignCommands asserts that accepting the
// offer runs BOTH halves of the capture — a document write and the reference
// that makes it visible to the plan workflow — and that the write half names
// both of its commands: `design author` for a design Spektacular wrote with
// the user, `design write` for one the user handed over. A rendered
// instruction naming only one half would leave either an unreferenced design
// or a broken reference; one naming only `design write` would push an
// authored design through the verbatim command and lose its lifecycle record.
//
// Each command name must survive as a contiguous run of characters, which is
// the point of asserting the rendered form: a template that wraps `design
// author` across a newline reads as a broken command to the agent following
// it, not merely to this test.
func TestTechnicalApproachStepNamesAllDesignCommands(t *testing.T) {
	out := renderStep(t, technicalApproach())

	require.Contains(t, out, "spektacular design author",
		"technical_approach must name the design author command")
	require.Contains(t, out, "spektacular design write",
		"technical_approach must name the design write command")
	require.Contains(t, out, "spektacular design ref add",
		"technical_approach must name the design ref add command")
	require.Contains(t, out, "Both steps, every time",
		"technical_approach must require both the write and the reference, every time")

	// The accept path hands the writing to the skill that owns it, and that
	// skill covers a design nobody has written down yet.
	require.Contains(t, out, "invoke the `spek-design` skill",
		"the accept path must hand the writing to the spek-design skill")
	require.Contains(t, out, "authoring interview when the design still has to be worked out",
		"the accept path must work for a design that does not yet exist in written form")
}

// TestTechnicalApproachStepMakesDeclineFinal asserts the decline outcome is
// terminal for the detail and does not silently fall back to inlining the
// design in the spec body, and that the agent may not read acceptance into
// silence.
func TestTechnicalApproachStepMakesDeclineFinal(t *testing.T) {
	out := renderStep(t, technicalApproach())

	require.Contains(t, out, "A decline is final for that detail",
		"technical_approach must make a decline final for that detail")
	require.Contains(t, out, "Declining does not mean the detail moves into the spec body instead",
		"technical_approach must state a decline does not move the design into the spec body")
	require.Contains(t, out, "Silence or deflection is not acceptance.",
		"technical_approach must state silence or deflection is not acceptance")
}

// TestTechnicalApproachStepDoesNotFetchANestedSkill guards the lesson recorded
// in plan 000041: `spektacular skill <name>` does not resolve for skills
// nested under templates/skills/workflows/, so an instruction must never send
// the agent to fetch one that way.
func TestTechnicalApproachStepDoesNotFetchANestedSkill(t *testing.T) {
	out := renderStep(t, technicalApproach())

	require.NotContains(t, out, "skill spek-",
		"technical_approach must not tell the agent to fetch a workflow skill via `skill spek-…` — that path does not resolve")
	require.NotContains(t, out, "{{",
		"technical_approach must leave no unrendered mustache")
}
