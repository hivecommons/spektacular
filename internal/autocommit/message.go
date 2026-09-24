package autocommit

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// ValidateMessage checks that an agent-written commit message says what it
// must: that it is not empty, that it names the spec the work belongs to,
// and that it names every milestone the commit marks. That is what makes a
// commit traceable back to its spec and milestone just by reading it.
//
// The spec name is matched case-sensitively, since it is an identifier the
// agent is handed verbatim. "Milestone N" is matched case-insensitively,
// since it is prose the agent writes itself. An empty milestones slice means
// no milestone is required.
//
// The returned error is an *output.ErrorResponse carrying the code only: the
// command layer adds the next_action, because only it knows the exact goto
// to re-run.
func ValidateMessage(message, specName string, milestones []int) error {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return output.NewError("commit_message_required", "the git commit message is empty")
	}
	if specName != "" && !strings.Contains(trimmed, specName) {
		return output.NewError("commit_message_invalid",
			fmt.Sprintf("the git commit message does not name the spec %q", specName))
	}
	for _, m := range milestones {
		if !strings.Contains(strings.ToLower(trimmed), strings.ToLower(fmt.Sprintf("Milestone %d", m))) {
			return output.NewError("commit_message_invalid",
				fmt.Sprintf("the git commit message does not name %q, whose phases are now all complete", fmt.Sprintf("Milestone %d", m)))
		}
	}
	return nil
}

// PreWorkflowMessage is the message for the commit that saves the user's own
// uncommitted changes before a workflow starts. Spektacular writes this one
// rather than the agent, because it describes work Spektacular did not do
// and must say so plainly in the history.
func PreWorkflowMessage(kind, specName string) string {
	return fmt.Sprintf("Save uncommitted changes before %s workflow for %s\n\n"+
		"These are the user's changes from before the %s workflow for %s started. "+
		"Spektacular committed them separately at the user's request so they are not "+
		"mixed with the agent's work.\n", kind, specName, kind, specName)
}
