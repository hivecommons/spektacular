package plan

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

// StatusResult is returned by the status subcommand.
type StatusResult struct {
	PlanName string `json:"plan_name"`
	// PlanDocument is the plan's document name, always "plan": with
	// PlanName it is the address `plan file read` takes.
	PlanDocument string `json:"plan_document"`
	// PlanPath is the plan's location relative to the folder holding the
	// config file that declares the plan store, never a host path.
	PlanPath       string      `json:"plan_path"`
	CurrentStep    string      `json:"current_step"`
	CompletedSteps []string    `json:"completed_steps"`
	TotalSteps     int         `json:"total_steps"`
	Progress       string      `json:"progress"`
	Steps          []StepEntry `json:"steps"`
}

// StepsResult is returned by the steps subcommand.
type StepsResult struct {
	Steps []string `json:"steps"`
}
