package worktree

import (
	"fmt"
	"os/exec"
	"strings"
)

// SetupRunner runs a repo's worktree setup command in a directory, returning
// its combined, trimmed output; an error means the command could not run or
// exited non-zero.
type SetupRunner interface {
	Run(dir, command string) (output string, err error)
}

type shellRunner struct{}

// Run runs command through `sh -c` with dir as its working directory. A
// failure carries the exit status and the command's own output, since that
// is the text a user needs to act on.
func (shellRunner) Run(dir, command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if err != nil {
		if out != "" {
			return out, fmt.Errorf("%w: %s", err, out)
		}
		return out, err
	}
	return out, nil
}

// NewSetupRunner returns the SetupRunner backed by the system's POSIX shell.
func NewSetupRunner() SetupRunner { return shellRunner{} }
