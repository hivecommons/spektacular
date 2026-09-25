package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// taskPlanName is a plan name whose ID prefix satisfies the default
// spec.id_method, so `plan file write` accepts it.
const taskPlanName = "20260709000000-feature"

// taskBlock renders one "#### - [ ] Task:" block. Field lines are passed whole
// so a test can drop or alter exactly one; criteria follow the lines.
func taskBlock(title string, done bool, fields []string, criteria ...string) string {
	box := " "
	if done {
		box = "x"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#### - [%s] Task: %s\n", box, title)
	for _, f := range fields {
		fmt.Fprintln(&b, f)
	}
	fmt.Fprintf(&b, "\nSummary of %s.\n", title)
	if len(criteria) > 0 {
		b.WriteString("\n**Acceptance criteria**:\n")
		for _, c := range criteria {
			fmt.Fprintln(&b, c)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// agentFields returns the four required lines for an agent task in the
// registered test repo.
func agentFields(id string, deps ...string) []string {
	return []string{"**Id:** " + id, "**Repo:** testproj", dependsLine(deps...), "**Execution:** agent"}
}

// dependsLine renders a "**Depends on:**" declaration; each dep is
// "<id> — <title>".
func dependsLine(deps ...string) string {
	if len(deps) == 0 {
		return "**Depends on:** none"
	}
	return "**Depends on:**\n- " + strings.Join(deps, "\n- ")
}

// taskPlanDoc wraps milestone bodies in a plan document. Each milestone body
// is the concatenation of its task blocks.
func taskPlanDoc(milestones ...string) string {
	var b strings.Builder
	b.WriteString("# Plan: " + taskPlanName + "\n\n## Overview\n\nfixture\n\n## Milestones & Tasks\n\n")
	for i, m := range milestones {
		fmt.Fprintf(&b, "### Milestone %d: Milestone %d\n\n", i+1, i+1)
		b.WriteString(m)
	}
	b.WriteString("## Testing Approach\n\n- [ ] not a task criterion\n")
	return b.String()
}

// writePlanDoc writes body as plan <name>'s document <doc> (a bare document
// name such as "plan") through `plan file write` and returns the command's
// stdout and exit code.
func writePlanDoc(t *testing.T, name, doc, body string) (string, int) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(src, []byte(body), 0o644))
	stdout, _, code := runRootCmd(t, "plan", "file", "write", name, doc, "--from", src)
	return stdout, code
}

// decodeError parses a structured failure from stdout.
func decodeError(t *testing.T, stdout string) output.ErrorResponse {
	t.Helper()
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er), "stdout: %s", stdout)
	require.True(t, er.IsError)
	return er
}
