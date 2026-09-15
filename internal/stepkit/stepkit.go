// Package stepkit provides the shared step-rendering pipeline used by every
// workflow (spec, plan, implement). It owns mustache template lookup, standard
// template-variable assembly, and the Result serialization pattern. Each
// workflow injects its own path conventions via a PathStrategy and its own
// Result struct shape via a ResultBuilder closure.
package stepkit

import (
	"fmt"
	"io/fs"
	"maps"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/jumppad-labs/spektacular/templates"
)

// PathStrategy injects workflow-specific template variables and identifies
// which path is "primary" for the workflow's Result struct.
type PathStrategy interface {
	// PathVars returns the workflow-specific path and name template variables
	// given the workflow instance name (e.g. plan name, spec name) and the
	// store root.
	PathVars(instanceName, storeRoot string) map[string]any
	// PrimaryPathField returns the key in the PathVars map whose value is the
	// "primary" path for this workflow (e.g. "plan_path", "spec_path"). It is
	// used to populate the Result struct's primary path field.
	PrimaryPathField() string
}

// StepRequest bundles the inputs to WriteStepResult.
type StepRequest struct {
	StepName     string
	NextStep     string
	TemplatePath string
	Strategy     PathStrategy
	// Extra holds per-callback template variables that take precedence over
	// both standard vars and Strategy vars.
	Extra map[string]any
}

// ResultBuilder constructs a workflow-specific result struct. It receives the
// step name, the workflow instance name (plan/spec name), the primary path,
// and the rendered instruction text.
type ResultBuilder func(stepName, instanceName, primaryPath, instruction string) any

// workingContextFooterPath is the shared fragment WriteStepResult appends to
// every step instruction that has a next step.
const workingContextFooterPath = "partials/working-context-footer.md"

// WriteStepResult renders the step's template, builds a workflow-specific
// result via the supplied builder, and writes it to the output writer.
//
// When the step has a next step, the rendered working-context footer
// (templates/partials/working-context-footer.md) is appended after a
// horizontal rule, so every continuing step ends with the identical
// keep-context-current paragraph and step templates never carry their own
// copy. Terminal steps (NextStep == "") get no footer.
//
// The variable merge order is: standard vars → strategy path vars → extras.
// Later entries win, so Extra can override both standard vars and strategy
// vars. The "name" key is pulled from workflow data to resolve the instance
// name passed to the strategy and the result builder.
func WriteStepResult(
	req StepRequest,
	data workflow.Data,
	out workflow.ResultWriter,
	st store.Store,
	cfg workflow.Config,
	build ResultBuilder,
) error {
	if req.Strategy == nil {
		return fmt.Errorf("stepkit: StepRequest.Strategy is required")
	}
	if build == nil {
		return fmt.Errorf("stepkit: ResultBuilder is required")
	}

	instanceName := GetString(data, "name")

	var storeRoot string
	if st != nil {
		storeRoot = st.Root()
	}
	pathVars := req.Strategy.PathVars(instanceName, storeRoot)

	vars := map[string]any{
		"step":      req.StepName,
		"title":     StepTitle(req.StepName),
		"next_step": req.NextStep,
		"config":    map[string]any{"command": cfg.Command},
		// command is the spelling shared fragments use, so one fragment
		// renders identically here and in install-time skills.
		"command": cfg.Command,
	}
	maps.Copy(vars, pathVars)
	maps.Copy(vars, req.Extra)

	instruction, err := RenderTemplate(req.TemplatePath, vars)
	if err != nil {
		return err
	}
	if req.NextStep != "" {
		footer, err := RenderTemplate(workingContextFooterPath, vars)
		if err != nil {
			return err
		}
		instruction = strings.TrimRight(instruction, "\n") + "\n\n---\n\n" + footer
	}

	primaryPath, _ := pathVars[req.Strategy.PrimaryPathField()].(string)
	return out.WriteResult(build(req.StepName, instanceName, primaryPath, instruction))
}

// StepTitle converts a snake_case step name like "acceptance_criteria" into
// a Title Case string like "Acceptance Criteria".
func StepTitle(name string) string {
	words := strings.Split(name, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// GetString is a convenience helper that reads a string value from workflow
// data and returns "" when the key is missing or the value is not a string.
func GetString(data workflow.Data, key string) string {
	v, ok := data.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// RenderTemplate loads a mustache template from the embedded templates FS and
// renders it against the supplied data. Partial includes such as
// {{> partials/implement-plan-documents}} resolve from the same FS.
func RenderTemplate(templatePath string, data map[string]any) (string, error) {
	tmplBytes, err := templates.FS.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("loading template %s: %w", templatePath, err)
	}
	return mustache.RenderPartials(string(tmplBytes), FSPartials{FS: templates.FS}, data)
}

// FSPartials resolves mustache partial includes ({{> partials/<name>}}) from
// an fs.FS by reading "<name>.md". Unlike mustache's own providers, a missing
// partial is an error rather than a silent empty render, so a mistyped include
// can never quietly drop shared prose from an instruction.
type FSPartials struct{ FS fs.FS }

var _ mustache.PartialProvider = FSPartials{}

// Get returns the raw template text of the named partial.
func (p FSPartials) Get(name string) (string, error) {
	b, err := fs.ReadFile(p.FS, name+".md")
	if err != nil {
		return "", fmt.Errorf("loading partial %s: %w", name, err)
	}
	return string(b), nil
}
