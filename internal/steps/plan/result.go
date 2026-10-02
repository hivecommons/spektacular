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

// StepsResult is returned by the steps subcommand.
type StepsResult struct {
	Steps []string `json:"steps"`
}
