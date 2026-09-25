package templates

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustReadTemplate(t *testing.T, p string) string {
	t.Helper()
	b, err := FS.ReadFile(p)
	require.NoErrorf(t, err, "reading %s", p)
	return string(b)
}

// TestSpecPlanGatheringStepsReferenceWorkDir verifies that every spec/plan
// gathering step instructs the agent to write its section to a per-section
// working file under .spektacular/work/<name>/, and that plan's read-only
// overview step (which drafts no section) does not.
func TestSpecPlanGatheringStepsReferenceWorkDir(t *testing.T) {
	specGathering := []string{
		"steps/spec/00b-interview.md",
		"steps/spec/01-overview.md",
		"steps/spec/02-requirements.md",
		"steps/spec/03-acceptance_criteria.md",
		"steps/spec/04-constraints.md",
		"steps/spec/05-technical_approach.md",
		"steps/spec/06-success_metrics.md",
		"steps/spec/07-non_goals.md",
	}
	for _, f := range specGathering {
		require.Containsf(t, mustReadTemplate(t, f), ".spektacular/work/{{spec_name}}/",
			"%s must instruct writing its section under .spektacular/work/{{spec_name}}/", f)
	}

	planGathering := []string{
		"steps/plan/02-discovery.md",
		"steps/plan/03-architecture.md",
		"steps/plan/04-components.md",
		"steps/plan/05-data_structures.md",
		"steps/plan/06-implementation_detail.md",
		"steps/plan/07-dependencies.md",
		"steps/plan/08-testing_approach.md",
		"steps/plan/09-milestones.md",
		"steps/plan/10-tasks.md",
		"steps/plan/11-open_questions.md",
		"steps/plan/12-out_of_scope.md",
	}
	for _, f := range planGathering {
		content := mustReadTemplate(t, f)
		require.Containsf(t, content, ".spektacular/work/{{plan_name}}/",
			"%s must instruct writing its section under .spektacular/work/{{plan_name}}/", f)
		// Phase 2.1: every plan gathering step records its judgement calls in
		// the shared assumption log alongside its section working file. Spec
		// gathering steps are intentionally exempt — the spec workflow is
		// unchanged.
		require.Containsf(t, content, ".spektacular/work/{{plan_name}}/assumptions.md",
			"%s must instruct appending judgement calls to the assumption log", f)
	}

	// Plan's overview step only reads the spec — it drafts no plan section and
	// must not reference a per-section working file.
	require.NotContains(t, mustReadTemplate(t, "steps/plan/01-overview.md"), ".spektacular/work/",
		"steps/plan/01-overview.md only reads the spec and must not write a section working file")
}

// TestImplementStepsHaveNoWorkFiles guards the spec/plan-only scope: the
// implement workflow produces code + a changelog, not an assembled document, so
// no implement step template may reference the per-section working-file dir.
func TestImplementStepsHaveNoWorkFiles(t *testing.T) {
	err := fs.WalkDir(FS, "steps/implement", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || path.Ext(p) != ".md" {
			return nil
		}
		require.NotContainsf(t, mustReadTemplate(t, p), ".spektacular/work/",
			"implement template %s must not reference the per-section working-file directory", p)
		return nil
	})
	require.NoError(t, err)
}

// TestAssemblyStepsCleanupWorkDir verifies the working directory is removed only
// at the final assembly step of each workflow (spec 08, plan 17) and nowhere
// else — gathering steps must never clean up, since the work files are the
// durable source until the document is committed.
func TestAssemblyStepsCleanupWorkDir(t *testing.T) {
	require.Contains(t, mustReadTemplate(t, "steps/spec/08-verification.md"),
		"rm -rf .spektacular/work/{{spec_name}}",
		"spec assembly step must remove the working dir after the spec is committed")
	require.Contains(t, mustReadTemplate(t, "steps/plan/17-write_research.md"),
		"rm -rf .spektacular/work/{{plan_name}}",
		"plan final-gate step must remove the working dir after all three docs are committed")

	// Exactly those two templates may contain the cleanup command.
	const cleanup = "rm -rf .spektacular/work"
	var cleaners []string
	for _, dir := range []string{"steps/spec", "steps/plan", "steps/implement"} {
		err := fs.WalkDir(FS, dir, func(p string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() || path.Ext(p) != ".md" {
				return nil
			}
			if strings.Contains(mustReadTemplate(t, p), cleanup) {
				cleaners = append(cleaners, p)
			}
			return nil
		})
		require.NoError(t, err)
	}
	require.ElementsMatch(t, []string{
		"steps/spec/08-verification.md",
		"steps/plan/17-write_research.md",
	}, cleaners, "only the assembly steps may clean up the working dir")
}

// TestAssemblyStepsMapSectionFiles verifies each assembly step names the
// per-section working files it reads back — catching a dropped mapping
// (especially the plan phases step's two files).
func TestAssemblyStepsMapSectionFiles(t *testing.T) {
	spec08 := mustReadTemplate(t, "steps/spec/08-verification.md")
	for _, f := range []string{
		"overview.md", "requirements.md", "constraints.md", "acceptance_criteria.md",
		"technical_approach.md", "success_metrics.md", "non_goals.md",
	} {
		require.Containsf(t, spec08, f, "spec assembly step must read section file %s", f)
	}

	planAssemble := mustReadTemplate(t, "steps/plan/13-assemble.md")
	for _, f := range []string{
		"architecture.md", "components.md", "data_structures.md", "implementation_detail.md",
		"dependencies.md", "testing_approach.md", "milestones.md", "open_questions.md",
		"out_of_scope.md", "research.md", "tasks_plan.md", "tasks_context.md",
		"assumptions.md",
	} {
		require.Containsf(t, planAssemble, f, "plan assembly step must read section file %s", f)
	}
}
