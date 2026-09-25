package spec

// Result is returned by the new and goto subcommands. SpecName is the spec's
// address; SpecPath is its location relative to the folder holding the config
// file that declares the spec store.
type Result struct {
	Step        string `json:"step"`
	SpecPath    string `json:"spec_path"`
	SpecName    string `json:"spec_name"`
	Instruction string `json:"instruction"`
}

// StepEntry holds a step name and its current status.
type StepEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// StatusResult is returned by the status subcommand.
type StatusResult struct {
	SpecName       string      `json:"spec_name"`
	SpecPath       string      `json:"spec_path"`
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
