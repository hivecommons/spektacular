// Package repo holds the step table for the guided repo add workflow: the
// conversation that registers a repo by asking for the repo itself and
// proposing every other value from what that repo says about itself.
//
// It is a peer of the spec, plan and implement step tables in every respect
// except one: it is deliberately not a member of the group that contends for
// the project's single workflow slot, so an add can run alongside a spec or
// plan already in progress.
package repo

import (
	"os"
	"path/filepath"

	repodomain "github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// Steps returns the ordered step configs for a guided add.
//
// The graph is linear apart from one multi-source transition: placement may
// be reached from name, description, role or tags, which is what carries the
// delegation shortcut. A user who answers the first proposal by handing over
// the whole set advances straight to placement with every proposed value
// supplied in that one call, and the remaining questions never happen.
//
// The first step "new" is internal: it asks nothing and advances on its own,
// so starting an add renders the first real question rather than a preamble.
// The terminal step must be named "finished" — workflow.State.InProgress
// compares against that literal, so a differently named terminal state would
// leave a completed add looking permanently unfinished to the resume check.
func Steps() []workflow.StepConfig {
	return []workflow.StepConfig{
		{Name: "new", Src: []string{"start"}, Dst: "new", Callback: new()},
		{Name: "locate", Src: []string{"new"}, Dst: "locate", Callback: locate()},
		{Name: "name", Src: []string{"locate"}, Dst: "name", Callback: name()},
		{Name: "description", Src: []string{"name"}, Dst: "description", Callback: description()},
		{Name: "role", Src: []string{"description"}, Dst: "role", Callback: role()},
		{Name: "tags", Src: []string{"role"}, Dst: "tags", Callback: tags()},
		{Name: "placement", Src: []string{"name", "description", "role", "tags"}, Dst: "placement", Callback: placement()},
		{Name: "confirm", Src: []string{"placement"}, Dst: "confirm", Callback: confirm()},
		{Name: "register", Src: []string{"confirm"}, Dst: "register", Callback: register()},
		{Name: "finished", Src: []string{"register"}, Dst: "finished", Callback: finished()},
	}
}

// buildResult is the stepkit.ResultBuilder for the guided add workflow.
func buildResult(stepName, instanceName, primaryPath, instruction string) any {
	return Result{
		Step:        stepName,
		RepoPath:    primaryPath,
		RepoName:    instanceName,
		Instruction: instruction,
	}
}

// writeStep is a thin wrapper around stepkit.WriteStepResult pre-applied with
// the guided add's strategy and result builder. The strategy is built per
// call because the folder it names is gathered during the flow rather than
// configured up front.
func writeStep(stepName, nextStep, templatePath string, data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config, extra map[string]any) error {
	return stepkit.WriteStepResult(
		stepkit.StepRequest{
			StepName:     stepName,
			NextStep:     nextStep,
			TemplatePath: templatePath,
			Strategy:     strategy{repoPath: targetDir(data)},
			Extra:        extra,
		},
		data, out, st, cfg,
		buildResult,
	)
}

// targetDir resolves the folder the repo's code lives at from the location
// gathered during the flow. It returns an absolute path where it can, and the
// empty string before a location has been given, which is the state the
// locate step exists to leave behind.
func targetDir(data workflow.Data) string {
	location := stepkit.GetString(data, "location")
	if location == "" {
		return ""
	}
	if filepath.IsAbs(location) {
		return filepath.Clean(location)
	}
	abs, err := filepath.Abs(location)
	if err != nil {
		return filepath.Clean(location)
	}
	return abs
}

// new asks nothing and advances on its own: an add that was started with the
// repo already named should open on the first proposal, and one started cold
// should open on the question asking which repo to add. Either way the
// preamble is not a step the user sees.
func new() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "locate", nil
	}
}

// locate is the only question asked cold. It also seeds a provisional name
// from the target folder so an add interrupted before the name is agreed
// still reports a readable name in its resume report.
func locate() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		seedProvisionalName(data)
		return "", writeStep("locate", "name", "steps/repo/01-locate.md", data, out, st, cfg, nil)
	}
}

// name is the first step to state a proposal, so it is where the repo is
// examined. The result is cached in workflow data, which the engine persists,
// so the examination happens once per add and survives an interruption.
func name() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("name", "description", "steps/repo/02-name.md", data, out, st, cfg, evidenceExtra(data))
	}
}

func description() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("description", "role", "steps/repo/03-description.md", data, out, st, cfg, evidenceExtra(data))
	}
}

func role() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("role", "tags", "steps/repo/04-role.md", data, out, st, cfg, evidenceExtra(data))
	}
}

func tags() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("tags", "placement", "steps/repo/05-tags.md", data, out, st, cfg, evidenceExtra(data))
	}
}

// placement establishes whether the target repo can actually take the
// project's files, which is what decides whether this step asks anything at
// all. The probe is a real filesystem operation, not a permission-bit read.
func placement() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra := evidenceExtra(data)
		extra["writable"] = writable(targetDir(data))
		return "", writeStep("placement", "confirm", "steps/repo/06-placement.md", data, out, st, cfg, extra)
	}
}

func confirm() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("confirm", "register", "steps/repo/07-confirm.md", data, out, st, cfg, nil)
	}
}

// register is the only step that writes anything. Its work runs before the
// transition it belongs to is committed, so a failure leaves the workflow on
// confirm with nothing written.
func register() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		if cfg.DryRun {
			return "", writeStep("register", "finished", "steps/repo/08-register.md", data, out, st, cfg, registrationExtra(nil))
		}

		res, err := doRegister(data, st)
		if err != nil {
			return "", err
		}
		data.Set("registration", map[string]any{
			"registered":    res.Registered,
			"footprint":     res.Footprint,
			"metadata_note": res.MetadataNote,
		})

		return "", writeStep("register", "finished", "steps/repo/08-register.md", data, out, st, cfg, registrationExtra(data))
	}
}

func finished() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("finished", "", "steps/repo/09-finished.md", data, out, st, cfg, registrationExtra(data))
	}
}

// registrationExtra surfaces what registration did, so the closing
// instructions can report the outcome. A nil data yields no variables, which
// is the dry-run case where nothing was registered.
func registrationExtra(data workflow.Data) map[string]any {
	if data == nil {
		return nil
	}
	raw, ok := data.Get("registration")
	if !ok {
		return nil
	}
	vars, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return map[string]any{"registration": vars}
}

// seedProvisionalName gives the workflow instance a readable name before the
// name step has agreed one. The shared step-rendering pipeline and the resume
// report both read the instance name out of data's "name" key, so without
// this an add interrupted early reports an unfinished workflow with an empty
// name. The value is superseded by whatever the name step agrees.
func seedProvisionalName(data workflow.Data) {
	if stepkit.GetString(data, "name") != "" {
		return
	}
	dir := targetDir(data)
	if dir == "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	data.Set("name", filepath.Base(dir))
}

// evidenceExtra returns the template variables carrying what the repo says
// about itself, examining it on first use and reading the cached result back
// on every later step.
//
// The cache lives in workflow data, so it is persisted with the rest of the
// gathered answers and an interrupted add resumes without re-reading the
// repo. What is cached is the already-rendered variable map rather than the
// Evidence value: workflow data round-trips through JSON, so caching the
// value would make these instructions render one way before an interruption
// and another way after it.
func evidenceExtra(data workflow.Data) map[string]any {
	if cached, ok := data.Get("evidence"); ok {
		if vars, ok := cached.(map[string]any); ok {
			return map[string]any{"evidence": vars}
		}
	}

	vars := evidenceVars(repodomain.Examine(targetDir(data)))
	data.Set("evidence", vars)
	return map[string]any{"evidence": vars}
}
