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

// StepsResult is returned by the steps subcommand.
type StepsResult struct {
	Steps []string `json:"steps"`
}
