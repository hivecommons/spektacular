package implement

// Result is returned by the new and goto subcommands. PlanName and
// PlanDocument are the plan's address; PlanPath is its location relative to
// the folder holding the config file that declares the plan store.
type Result struct {
	Step         string `json:"step"`
	PlanPath     string `json:"plan_path"`
	PlanName     string `json:"plan_name"`
	PlanDocument string `json:"plan_document"`
	Instruction  string `json:"instruction"`
}

// StepEntry holds a step name and its current status.
type StepEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// StatusResult is returned by the status subcommand. UncheckedPhases is the
// count of open work items in the plan file — tasks, or `#### - [ ] Phase`
// headings in a plan written before tasks; zero if the plan file cannot be
// read. The field keeps its name so existing readers keep working.
type StatusResult struct {
	PlanName string `json:"plan_name"`
	// PlanDocument is the plan's document name, always "plan": with
	// PlanName it is the address `plan file read` takes.
	PlanDocument string `json:"plan_document"`
	// PlanPath is the plan's location relative to the folder holding the
	// config file that declares the plan store, never a host path.
	PlanPath        string      `json:"plan_path"`
	CurrentStep     string      `json:"current_step"`
	CompletedSteps  []string    `json:"completed_steps"`
	TotalSteps      int         `json:"total_steps"`
	Progress        string      `json:"progress"`
	Steps           []StepEntry `json:"steps"`
	UncheckedPhases int         `json:"unchecked_phases"`
	// Task is the id of the one task a single-task run implements; absent
	// for a whole-plan run.
	Task string `json:"task,omitempty"`
}

// StepsResult is returned by the steps subcommand.
type StepsResult struct {
	Steps []string `json:"steps"`
}
