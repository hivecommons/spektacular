package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/epic"
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
		{Name: "split", Src: []string{"verification"}, Dst: "split", Callback: split()},
		{Name: "finished", Src: []string{"split"}, Dst: "finished", Callback: finished()},
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
		sources, err := sourcesFrom(data)
		if err != nil {
			return "", err
		}
		var opts metadata.UpdateOptions
		if len(sources) > 0 {
			opts.Sources = &sources
		}
		merged, err := metadata.Merge(nil, []byte(rendered), opts)
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
		extra, err := interviewExtra(data)
		if err != nil {
			return "", err
		}
		return "", writeStep("interview", "overview", "steps/spec/00b-interview.md", data, out, st, cfg, extra)
	}
}

// interviewExtra is what the interview template is given beyond the standard
// variables: the links the spec was seeded from and the epic it joined, so the
// template can open a seeded interview. Both are empty for a plain spec.
func interviewExtra(data workflow.Data) (map[string]any, error) {
	sources, err := sourcesFrom(data)
	if err != nil {
		return nil, err
	}
	uris := make([]string, len(sources))
	for i, s := range sources {
		uris[i] = s.URI
	}
	return map[string]any{
		"sources": uris,
		// seeded gates the template's seeded branch: a mustache section
		// over the list itself would repeat the branch once per source.
		"seeded": len(uris) > 0,
		"epic":   stepkit.GetString(data, "epic"),
	}, nil
}

// sourcesFrom reads the stamped sources spec new stored in the workflow data.
// In the process that started the workflow they are []metadata.SourceRef; once
// state.json has round-tripped them they come back as []any of map[string]any,
// so both shapes are decoded through JSON. Absent sources yield nil.
func sourcesFrom(data workflow.Data) ([]metadata.SourceRef, error) {
	v, ok := data.Get("sources")
	if !ok || v == nil {
		return nil, nil
	}
	if refs, ok := v.([]metadata.SourceRef); ok {
		return refs, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("reading sources from workflow data: %w", err)
	}
	var refs []metadata.SourceRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("reading sources from workflow data: %w", err)
	}
	return refs, nil
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
		return "", writeStep("verification", "split", "steps/spec/08-verification.md", data, out, st, cfg, map[string]any{
			"spec_template": scaffold,
		})
	}
}

// split runs the split check once on the complete spec, and acts on a split
// the user asked for earlier in the workflow. It tells the template the epic
// the spec already belongs to, if any, so a split extends that epic.
func split() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		var extra map[string]any
		if !cfg.DryRun && st != nil {
			if name := specEpic(st, cfg, stepkit.GetString(data, "name")); name != "" {
				extra = map[string]any{"epic_name": name}
			}
		}
		return "", writeStep("split", "finished", "steps/spec/08b-split.md", data, out, st, cfg, extra)
	}
}

// specEpic reads the epic the stored spec names, or "" when it names none or
// cannot be read.
func specEpic(st store.Store, cfg workflow.Config, name string) string {
	raw, err := st.Read(SpecFilePath(cfg.SpecDir, name))
	if err != nil {
		return ""
	}
	fm, _, err := metadata.Split(raw)
	if err != nil || fm == nil {
		return ""
	}
	return fm.Epic
}

// epicSourceURIs reads the links of the sources the named epic records, for
// the chaining offer. An epic that cannot be read yields none.
func epicSourceURIs(st store.Store, cfg workflow.Config, name string) []string {
	raw, err := st.Read(artifact.Address{Kind: artifact.KindEpic, Feature: name}.StorePath(cfg.EpicDir))
	if err != nil {
		return nil
	}
	e, err := epic.Parse(raw)
	if err != nil {
		return nil
	}
	uris := make([]string, 0, len(e.Sources))
	for _, s := range e.Sources {
		uris = append(uris, s.URI)
	}
	return uris
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
				// A spec in an epic gets the chaining offer: the template
				// is told the epic and the sources it was started from, so
				// the agent can look for a source item with no spec yet.
				if name := specEpic(st, cfg, stepkit.GetString(data, "name")); name != "" {
					sources := epicSourceURIs(st, cfg, name)
					extra = map[string]any{"epic_name": name, "epic_sources": sources, "epic_has_sources": len(sources) > 0}
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
