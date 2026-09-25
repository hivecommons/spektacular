package spec

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/internal/workingcontext"
)

// SpecFilePath returns the store-relative path for a spec file under the
// configured spec directory: where the spec addressed by name is stored.
func SpecFilePath(dir, name string) string {
	return artifact.Address{Kind: artifact.KindSpec, Feature: name}.StorePath(dir)
}

// Steps returns the ordered step configs for a spec workflow.
// Each step has an explicit named callback — no string-based dispatch.
// The first step "new" is internal: it creates the spec file and produces no
// output, allowing the caller to automatically advance to "overview".
func Steps() []workflow.StepConfig {
	return []workflow.StepConfig{
		{Name: "new", Src: []string{"start"}, Dst: "new", Callback: new()},
		{Name: "interview", Src: []string{"new"}, Dst: "interview", Callback: interview()},
		{Name: "overview", Src: []string{"interview"}, Dst: "overview", Callback: overview()},
		{Name: "requirements", Src: []string{"overview"}, Dst: "requirements", Callback: requirements()},
		{Name: "acceptance_criteria", Src: []string{"requirements"}, Dst: "acceptance_criteria", Callback: acceptanceCriteria()},
		{Name: "constraints", Src: []string{"acceptance_criteria"}, Dst: "constraints", Callback: constraints()},
		{Name: "technical_approach", Src: []string{"constraints"}, Dst: "technical_approach", Callback: technicalApproach()},
		{Name: "success_metrics", Src: []string{"technical_approach"}, Dst: "success_metrics", Callback: successMetrics()},
		{Name: "non_goals", Src: []string{"success_metrics"}, Dst: "non_goals", Callback: nonGoals()},
		{Name: "verification", Src: []string{"non_goals"}, Dst: "verification", Callback: verification()},
		{Name: "finished", Src: []string{"verification"}, Dst: "finished", Callback: finished()},
	}
}

// buildResult is the stepkit.ResultBuilder for the spec workflow.
func buildResult(stepName, instanceName, primaryPath, instruction string) any {
	return Result{
		Step:        stepName,
		SpecPath:    primaryPath,
		SpecName:    instanceName,
		Instruction: instruction,
	}
}

// writeStep is a thin wrapper around stepkit.WriteStepResult pre-applied with
// the spec strategy and result builder.
func writeStep(stepName, nextStep, templatePath string, data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config, extra map[string]any) error {
	return stepkit.WriteStepResult(
		stepkit.StepRequest{
			StepName:     stepName,
			NextStep:     nextStep,
			TemplatePath: templatePath,
			Strategy:     strategy{specDir: cfg.SpecDir},
			Extra:        extra,
		},
		data, out, st, cfg,
		buildResult,
	)
}

// new creates the spec file, clears the working context, and returns an instruction
// to write conversation context before proceeding to overview.
func new() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		if cfg.DryRun {
			return "", writeStep("new", "interview", "steps/spec/00-new.md", data, out, st, cfg, nil)
		}
		if st == nil {
			return "", fmt.Errorf("store required for new step")
		}
		name := stepkit.GetString(data, "name")
		rendered, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": name})
		if err != nil {
			return "", err
		}
		merged, err := metadata.Merge(nil, []byte(rendered), metadata.UpdateOptions{})
		if err != nil {
			return "", err
		}
		if err := st.Write(SpecFilePath(cfg.SpecDir, name), merged); err != nil {
			return "", err
		}

		// Reset the working context for fresh conversation context: drop the previous
		// session's content so nothing carries over. The relative path
		// resolves against the current working directory (which is the
		// project root when running `go run . spec new`).
		if err := workingcontext.Reset(filepath.FromSlash(workingcontext.RelPath)); err != nil {
			return "", fmt.Errorf("resetting working context: %w", err)
		}

		return "", writeStep("new", "interview", "steps/spec/00-new.md", data, out, st, cfg, nil)
	}
}

func interview() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("interview", "overview", "steps/spec/00b-interview.md", data, out, st, cfg, nil)
	}
}

func overview() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("overview", "requirements", "steps/spec/01-overview.md", data, out, st, cfg, nil)
	}
}

func requirements() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("requirements", "acceptance_criteria", "steps/spec/02-requirements.md", data, out, st, cfg, nil)
	}
}

func acceptanceCriteria() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("acceptance_criteria", "constraints", "steps/spec/03-acceptance_criteria.md", data, out, st, cfg, nil)
	}
}

func constraints() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("constraints", "technical_approach", "steps/spec/04-constraints.md", data, out, st, cfg, nil)
	}
}

func technicalApproach() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("technical_approach", "success_metrics", "steps/spec/05-technical_approach.md", data, out, st, cfg, nil)
	}
}

func successMetrics() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("success_metrics", "non_goals", "steps/spec/06-success_metrics.md", data, out, st, cfg, nil)
	}
}

func nonGoals() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("non_goals", "verification", "steps/spec/07-non_goals.md", data, out, st, cfg, nil)
	}
}

func verification() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		specName := stepkit.GetString(data, "name")
		scaffold, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": specName})
		if err != nil {
			return "", err
		}
		return "", writeStep("verification", "finished", "steps/spec/08-verification.md", data, out, st, cfg, map[string]any{
			"spec_template": scaffold,
		})
	}
}

func finished() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		// The completed spec is committed to the store by the agent during the
		// verification step via `spec file write`. Read it back through the
		// store; if it is still the empty scaffold the agent skipped that
		// write, so surface a warning in the finished instruction.
		var extra map[string]any
		if !cfg.DryRun && st != nil {
			unwritten, err := specStillScaffold(st, cfg, stepkit.GetString(data, "name"))
			if err != nil {
				return "", err
			}
			if unwritten {
				extra = map[string]any{"spec_unwritten": true}
			} else {
				if err := metadata.Close(st, SpecFilePath(cfg.SpecDir, stepkit.GetString(data, "name")), metadata.StatusFinal); err != nil && !errors.Is(err, store.ErrNotFound) {
					return "", err
				}
			}
		}
		return "", writeStep("finished", "", "steps/spec/09-finished.md", data, out, st, cfg, extra)
	}
}

// specStillScaffold reads the spec file back through the store and reports
// whether it still holds the unfilled scaffold — i.e. the completed spec was
// never committed with `spec file write`. A spec that cannot be read is also
// treated as unwritten. A leading YAML frontmatter block on the stored artifact
// is stripped before the comparison so the check answers "is the body still the
// scaffold?", tolerating the metadata block that Spektacular now prepends on
// every write.
func specStillScaffold(st store.Store, cfg workflow.Config, specName string) (bool, error) {
	stored, err := st.Read(SpecFilePath(cfg.SpecDir, specName))
	if err != nil {
		return true, nil
	}
	scaffold, err := stepkit.RenderTemplate("scaffold/spec.md", map[string]any{"name": specName})
	if err != nil {
		return false, err
	}
	body := stored
	if fm, split, splitErr := metadata.Split(stored); splitErr == nil && fm != nil {
		body = split
	}
	return string(body) == scaffold, nil
}
