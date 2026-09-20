package migrate

import (
	"os"

	"gopkg.in/yaml.v3"
)

// defaultCommand is the command name used in remediation text when a
// project's settings do not record one.
const defaultCommand = "spektacular"

// PeekCommand returns the `command` recorded in the project settings file at
// path, for composing remediation text. It reads leniently, because the file
// may be one this build refuses to load, and falls back to "spektacular".
func PeekCommand(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return defaultCommand
	}
	var head struct {
		Command string `yaml:"command"`
	}
	if err := yaml.Unmarshal(raw, &head); err != nil || head.Command == "" {
		return defaultCommand
	}
	return head.Command
}
