package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file holds plan 000030_agent-actionable-errors's Phase 3.2 regression
// test: it reproduces the exact incident described in the spec's Overview —
// an agent driving this CLI hit a rejected step transition and got back only
// a bare, unhelpful error with no indication of the current step or what to
// do next, and got stuck retrying blindly. The test drives the real command
// wrapper end to end (cobra -> runRoot -> internal/workflow), not
// internal/workflow directly, since the whole point is proving the full
// stack behaves correctly, not just the translation logic in isolation
// (already covered in internal/workflow/workflow_test.go).

// TestGoto_UnreachableStepReturnsActionableError reproduces the original
// incident: an agent lands on "overview" and tries to jump straight to
// "milestones", a step unreachable from there. internal/steps/plan/steps.go
// wires "milestones" as reachable only from "testing_approach"
// ({Name: "milestones", Src: []string{"testing_approach"}, ...}), while the
// only step reachable from "overview" is "discovery"
// ({Name: "discovery", Src: []string{"overview"}, ...}) — both hand-verified
// against that file, not derived from this test's own subject.
func TestGoto_UnreachableStepReturnsActionableError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	// 1. Seed an in-progress workflow via the real CLI. plan's "new" step
	// callback (internal/steps/plan/steps.go's new()) unconditionally
	// returns "overview", so `plan new` auto-advances one step further and
	// genuinely rests on "overview" — this is the real, traceable
	// consequence of plan.Steps()'s wiring, not an assumption.
	resetRootCmd(t)
	seedStdout, seedStderr, seedCode := runRootCmd(t, "plan", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, seedCode)
	require.Empty(t, seedStderr)
	require.NotEmpty(t, seedStdout)

	// 2. Issue the unreachable goto — mirroring the original incident's
	// "overview" -> "milestones" jump.
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "plan", "goto", "--data", `{"step":"milestones"}`)

	// Exit code is non-zero and stderr is completely empty: no fallback to
	// cobra's own error/usage printer, no need to watch two streams to
	// reliably notice the failure.
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	// stdout parses as a single JSON object with "error": true. A successful
	// Unmarshal here also rules out a second, appended response: encoding/json
	// rejects trailing non-whitespace data after the decoded value.
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)

	// state.current names the workflow's actual current step.
	require.NotNil(t, er.State)
	require.Equal(t, "overview", er.State.Current)

	// state.valid_actions names exactly the step(s) that would currently
	// succeed — from "overview", only "discovery" is legally reachable.
	require.Equal(t, []string{"discovery"}, er.State.ValidActions)

	// next_action gives a concrete, runnable recovery command instead of
	// leaving the agent to guess or retry blindly — the exact gap the
	// original incident exposed.
	require.NotEmpty(t, er.NextAction)
	require.Contains(t, er.NextAction, "goto --data")
	require.Contains(t, er.NextAction, `"step":"discovery"`)

	// The failure is printed exactly once: guards against the old
	// double-print bug where cobra's own default error printer ran alongside
	// the tool's own response. er.Message is compared via its JSON-encoded
	// form since stdout holds it with JSON string-escaping (e.g. `\"`) that
	// the raw Go string doesn't have.
	require.Equal(t, 1, strings.Count(stdout, `"error": true`))
	msgJSON, err := json.Marshal(er.Message)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(stdout, string(msgJSON)))
}
