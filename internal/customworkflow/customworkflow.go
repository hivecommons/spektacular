package customworkflow

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/templates"
	"gopkg.in/yaml.v3"
)

const (
	ProjectWorkflowDir = ".spektacular/workflows"
	UserWorkflowDir    = "spektacular/workflows"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Definition struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description,omitempty"`
	Steps       []StepDefinition `yaml:"steps"`
}

type StepDefinition struct {
	Name        string       `yaml:"name"`
	Prompt      string       `yaml:"prompt"`
	Transitions []Transition `yaml:"transitions,omitempty"`
}

type Transition struct {
	To   string     `yaml:"to"`
	When *Condition `yaml:"when,omitempty"`
}

type Condition struct {
	Key       string `yaml:"key"`
	Equals    any    `yaml:"equals,omitempty"`
	NotEquals any    `yaml:"not_equals,omitempty"`
	Exists    *bool  `yaml:"exists,omitempty"`
}

type Loaded struct {
	Definition Definition
	FS         fs.FS
	Source     string
}

type Result struct {
	Workflow    string   `json:"workflow"`
	RunName     string   `json:"run_name"`
	Step        string   `json:"step"`
	NextSteps   []string `json:"next_steps"`
	Instruction string   `json:"instruction"`
}

type StepsResult struct {
	Workflow string   `json:"workflow"`
	Steps    []string `json:"steps"`
}

type StepEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type StatusResult struct {
	Workflow       string      `json:"workflow"`
	RunName        string      `json:"run_name"`
	CurrentStep    string      `json:"current_step"`
	CompletedSteps []string    `json:"completed_steps"`
	TotalSteps     int         `json:"total_steps"`
	Progress       string      `json:"progress"`
	Steps          []StepEntry `json:"steps"`
}

func Kind(name string) string {
	return "workflow:" + name
}

func Load(name, projectRoot string) (Loaded, error) {
	if !slugPattern.MatchString(name) {
		return Loaded{}, output.NewError("workflow_invalid", fmt.Sprintf("workflow name %q must contain only lowercase letters, digits, '-' or '_'", name)).
			WithResource(name).
			WithNextAction("use a workflow name such as marketing-ideation")
	}

	candidates := []struct {
		label string
		fsys  fs.FS
	}{
		{label: filepath.Join(projectRoot, ProjectWorkflowDir, name), fsys: os.DirFS(filepath.Join(projectRoot, ProjectWorkflowDir, name))},
	}
	if userDir := userConfigWorkflowDir(name); userDir != "" {
		candidates = append(candidates, struct {
			label string
			fsys  fs.FS
		}{label: userDir, fsys: os.DirFS(userDir)})
	}
	if builtIn, err := fs.Sub(templates.FS, filepath.ToSlash(filepath.Join("workflows", name))); err == nil {
		candidates = append(candidates, struct {
			label string
			fsys  fs.FS
		}{label: "built-in:" + name, fsys: builtIn})
	}

	for _, candidate := range candidates {
		def, ok, err := readDefinition(candidate.fsys)
		if err != nil {
			return Loaded{}, fmt.Errorf("loading workflow %s from %s: %w", name, candidate.label, err)
		}
		if !ok {
			continue
		}
		if def.Name == "" {
			def.Name = name
		}
		loaded := Loaded{Definition: def, FS: candidate.fsys, Source: candidate.label}
		if err := loaded.Validate(name); err != nil {
			return Loaded{}, err
		}
		return loaded, nil
	}

	return Loaded{}, output.NewError("workflow_not_found", fmt.Sprintf("workflow %q was not found", name)).
		WithResource(name).
		WithNextAction(fmt.Sprintf("create %s/%s/workflow.yaml or use a built-in workflow; run `workflow steps <name>` after adding it", ProjectWorkflowDir, name))
}

func readDefinition(fsys fs.FS) (Definition, bool, error) {
	for _, name := range []string{"workflow.yaml", "workflow.yml"} {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Definition{}, false, err
		}
		var def Definition
		decoder := yaml.NewDecoder(bytes.NewReader(raw))
		decoder.KnownFields(true)
		if err := decoder.Decode(&def); err != nil {
			return Definition{}, false, err
		}
		return def, true, nil
	}
	return Definition{}, false, nil
}

func userConfigWorkflowDir(name string) string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, UserWorkflowDir, name)
}

func (l Loaded) Validate(requested string) error {
	if l.Definition.Name != requested {
		return output.NewError("workflow_invalid", fmt.Sprintf("workflow file declares name %q, but %q was requested", l.Definition.Name, requested)).
			WithResource(l.Source).
			WithNextAction("make the workflow.yaml name match its directory name")
	}
	if !slugPattern.MatchString(l.Definition.Name) {
		return output.NewError("workflow_invalid", fmt.Sprintf("workflow name %q must be slug-safe", l.Definition.Name)).
			WithResource(l.Source)
	}
	if len(l.Definition.Steps) == 0 {
		return output.NewError("workflow_invalid", "workflow declares no steps").WithResource(l.Source)
	}

	seen := make(map[string]bool, len(l.Definition.Steps))
	for i, step := range l.Definition.Steps {
		if !slugPattern.MatchString(step.Name) {
			return output.NewError("workflow_invalid", fmt.Sprintf("steps[%d].name %q must be slug-safe", i, step.Name)).WithResource(l.Source)
		}
		if seen[step.Name] {
			return output.NewError("workflow_invalid", fmt.Sprintf("step %q is declared more than once", step.Name)).WithResource(l.Source)
		}
		seen[step.Name] = true
		if step.Prompt == "" {
			return output.NewError("workflow_invalid", fmt.Sprintf("step %q has no prompt", step.Name)).WithResource(l.Source)
		}
		if _, err := fs.Stat(l.FS, step.Prompt); err != nil {
			return output.NewError("workflow_invalid", fmt.Sprintf("step %q prompt %q cannot be read", step.Name, step.Prompt)).WithResource(l.Source)
		}
	}
	for _, step := range l.Definition.Steps {
		for _, transition := range l.transitionsFrom(step) {
			if !seen[transition.To] {
				return output.NewError("workflow_invalid", fmt.Sprintf("step %q transitions to unknown step %q", step.Name, transition.To)).WithResource(l.Source)
			}
			if transition.When != nil && transition.When.Key == "" {
				return output.NewError("workflow_invalid", fmt.Sprintf("step %q transition to %q has a condition with no key", step.Name, transition.To)).WithResource(l.Source)
			}
		}
	}
	return nil
}

func (l Loaded) Steps() []workflow.StepConfig {
	incoming := make(map[string][]string, len(l.Definition.Steps))
	conditions := make(map[string]map[string]*Condition, len(l.Definition.Steps))
	incoming[l.Definition.Steps[0].Name] = append(incoming[l.Definition.Steps[0].Name], "start")
	for _, step := range l.Definition.Steps {
		for _, transition := range l.transitionsFrom(step) {
			incoming[transition.To] = append(incoming[transition.To], step.Name)
			if transition.When != nil {
				if conditions[transition.To] == nil {
					conditions[transition.To] = map[string]*Condition{}
				}
				conditions[transition.To][step.Name] = transition.When
			}
		}
	}

	steps := make([]workflow.StepConfig, 0, len(l.Definition.Steps))
	for _, step := range l.Definition.Steps {
		step := step
		transitions := l.transitionsFrom(step)
		cfg := workflow.StepConfig{
			Name:     step.Name,
			Src:      incoming[step.Name],
			Dst:      step.Name,
			Callback: l.callback(step),
			Terminal: len(transitions) == 0,
		}
		if bySrc := conditions[step.Name]; len(bySrc) > 0 {
			cfg.Condition = func(src string, data workflow.Data) bool {
				cond := bySrc[src]
				if cond == nil {
					return true
				}
				return cond.Matches(data)
			}
		}
		steps = append(steps, cfg)
	}
	return steps
}

func (l Loaded) transitionsFrom(step StepDefinition) []Transition {
	if step.Transitions != nil {
		return step.Transitions
	}
	for i, candidate := range l.Definition.Steps {
		if candidate.Name == step.Name && i+1 < len(l.Definition.Steps) {
			return []Transition{{To: l.Definition.Steps[i+1].Name}}
		}
	}
	return nil
}

func (l Loaded) callback(step StepDefinition) workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, _ store.Store, cfg workflow.Config) (string, error) {
		vars := l.templateVars(step, data, cfg)
		raw, err := fs.ReadFile(l.FS, step.Prompt)
		if err != nil {
			return "", err
		}
		instruction, err := mustache.RenderPartials(string(raw), stepkit.FSPartials{FS: l.FS}, vars)
		if err != nil {
			return "", err
		}
		if len(l.transitionsFrom(step)) > 0 {
			footer, err := stepkit.RenderTemplate("partials/working-context-footer.md", vars)
			if err != nil {
				return "", err
			}
			instruction = strings.TrimRight(instruction, "\n") + "\n\n---\n\n" + footer
		}
		runName := stringValue(data, "name")
		if runName == "" {
			runName = l.Definition.Name
		}
		return "", out.WriteResult(Result{
			Workflow:    l.Definition.Name,
			RunName:     runName,
			Step:        step.Name,
			NextSteps:   l.nextStepsForData(step, data),
			Instruction: instruction,
		})
	}
}

func (l Loaded) templateVars(step StepDefinition, data workflow.Data, cfg workflow.Config) map[string]any {
	vars := map[string]any{}
	if snapper, ok := data.(interface{ Snapshot() map[string]any }); ok {
		for k, v := range snapper.Snapshot() {
			vars[k] = v
		}
	}
	runName := stringValue(data, "name")
	if runName == "" {
		runName = l.Definition.Name
	}
	vars["workflow"] = l.Definition.Name
	vars["workflow_description"] = l.Definition.Description
	vars["run_name"] = runName
	vars["name"] = runName
	vars["step"] = step.Name
	vars["title"] = stepkit.StepTitle(step.Name)
	vars["next_steps"] = l.nextStepsForData(step, data)
	vars["config"] = map[string]any{"command": cfg.Command}
	vars["command"] = cfg.Command
	return vars
}

func (l Loaded) nextSteps(step StepDefinition) []string {
	transitions := l.transitionsFrom(step)
	steps := make([]string, 0, len(transitions))
	for _, transition := range transitions {
		steps = append(steps, transition.To)
	}
	return steps
}

func (l Loaded) nextStepsForData(step StepDefinition, data workflow.Data) []string {
	transitions := l.transitionsFrom(step)
	steps := make([]string, 0, len(transitions))
	for _, transition := range transitions {
		if transition.When != nil && !transition.When.Matches(data) {
			continue
		}
		steps = append(steps, transition.To)
	}
	return steps
}

func (c Condition) Matches(data workflow.Data) bool {
	value, ok := data.Get(c.Key)
	if c.Exists != nil && ok != *c.Exists {
		return false
	}
	if c.Equals != nil && !valuesEqual(value, c.Equals) {
		return false
	}
	if c.NotEquals != nil && valuesEqual(value, c.NotEquals) {
		return false
	}
	return true
}

func valuesEqual(got, want any) bool {
	if gotNumber, gotOK := numberValue(got); gotOK {
		if wantNumber, wantOK := numberValue(want); wantOK {
			return gotNumber == wantNumber
		}
	}
	return reflect.DeepEqual(got, want)
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func stringValue(data workflow.Data, key string) string {
	v, ok := data.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
