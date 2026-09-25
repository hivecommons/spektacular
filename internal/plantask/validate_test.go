package plantask

import (
	"errors"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

var repos = []string{"spektacular", "docs"}

func requireCode(t *testing.T, err error, code string) *output.ErrorResponse {
	t.Helper()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.True(t, errors.As(err, &er), "want *output.ErrorResponse, got %T", err)
	require.Equal(t, code, er.Code)
	require.NotEmpty(t, er.NextAction)
	return er
}

func TestValidate_WellFormedPlanPasses(t *testing.T) {
	require.NoError(t, Validate(Parse([]byte(taskPlan)), repos))
}

// block renders one task under a milestone. Each field line is passed whole
// so a test can drop or alter exactly one.
func block(title string, lines ...string) string {
	return "#### - [ ] Task: " + title + "\n" + strings.Join(lines, "\n") + "\n\n"
}

func plan(blocks ...string) []byte {
	return []byte("## Milestones & Tasks\n\n### Milestone 1: M\n\n" + strings.Join(blocks, ""))
}

func TestValidate_RefusesEachRule(t *testing.T) {
	good := []string{"**Id:** b", "**Repo:** spektacular", "**Depends on:** none", "**Execution:** agent"}
	other := block("Other", "**Id:** a", "**Repo:** spektacular", "**Depends on:** none", "**Execution:** agent")

	cases := []struct {
		name     string
		body     []byte
		wantTask string // appears in the message
		wantRule string // appears in the message
		wantNext string // appears in next_action
	}{
		{
			name:     "missing id",
			body:     plan(other, block("Broken", good[1], good[2], good[3])),
			wantTask: `task "Broken"`, wantRule: "no **Id:**", wantNext: "plan task-id",
		},
		{
			name:     "missing repo",
			body:     plan(other, block("Broken", good[0], good[2], good[3])),
			wantTask: `task "Broken" (b)`, wantRule: "no **Repo:**", wantNext: "spektacular, docs",
		},
		{
			name:     "missing dependency declaration",
			body:     plan(other, block("Broken", good[0], good[1], good[3])),
			wantTask: `task "Broken" (b)`, wantRule: "no dependency declaration", wantNext: "none",
		},
		{
			name:     "empty dependency declaration",
			body:     plan(other, block("Broken", good[0], good[1], "**Depends on:**", good[3])),
			wantTask: `task "Broken" (b)`, wantRule: "no dependency declaration", wantNext: "none",
		},
		{
			name:     "missing execution",
			body:     plan(other, block("Broken", good[0], good[1], good[2])),
			wantTask: `task "Broken" (b)`, wantRule: "no **Execution:**", wantNext: "agent",
		},
		{
			name:     "two repos",
			body:     plan(other, block("Broken", good[0], "**Repo:** spektacular, docs", good[2], good[3])),
			wantTask: `task "Broken" (b)`, wantRule: "more than one repo", wantNext: "one of: spektacular, docs",
		},
		{
			name:     "unregistered repo",
			body:     plan(other, block("Broken", good[0], "**Repo:** elsewhere", good[2], good[3])),
			wantTask: `task "Broken" (b)`, wantRule: `"elsewhere"`, wantNext: "one of: spektacular, docs",
		},
		{
			name:     "unknown dependency",
			body:     plan(other, block("Broken", good[0], good[1], "**Depends on:**", "- zzz — Nowhere", good[3])),
			wantTask: `task "Broken" (b)`, wantRule: "depends on zzz", wantNext: "Depends on",
		},
		{
			name: "two-task cycle",
			body: plan(
				block("First", "**Id:** a", good[1], "**Depends on:**", "- b — Second", good[3]),
				block("Second", good[0], good[1], "**Depends on:**", "- a — First", good[3]),
			),
			wantTask: `task "First" (a)`, wantRule: `"First" -> "Second" -> "First"`, wantNext: "cycle",
		},
		{
			name: "longer cycle",
			body: plan(
				block("One", "**Id:** a", good[1], "**Depends on:**", "- c — Three", good[3]),
				block("Two", "**Id:** b", good[1], "**Depends on:**", "- a — One", good[3]),
				block("Three", "**Id:** c", good[1], "**Depends on:**", "- b — Two", good[3]),
			),
			wantTask: `task "One" (a)`, wantRule: `"One" -> "Three" -> "Two" -> "One"`, wantNext: "cycle",
		},
		{
			name:     "duplicate id",
			body:     plan(other, block("Broken", "**Id:** a", good[1], good[2], good[3])),
			wantTask: `task "Broken" (a)`, wantRule: `already used by task "Other"`, wantNext: "plan task-id",
		},
		{
			name:     "unknown executor",
			body:     plan(other, block("Broken", good[0], good[1], good[2], "**Execution:** robot")),
			wantTask: `task "Broken" (b)`, wantRule: `"robot"`, wantNext: "agent",
		},
		{
			name:     "human without reason",
			body:     plan(other, block("Broken", good[0], good[1], good[2], "**Execution:** human")),
			wantTask: `task "Broken" (b)`, wantRule: "no reason", wantNext: "human —",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Parse(tc.body)
			require.Equal(t, FormatTasks, p.Format)
			er := requireCode(t, Validate(p, repos), CodeTaskInvalid)
			require.Contains(t, er.Message, tc.wantTask)
			require.Contains(t, er.Message, tc.wantRule)
			require.Contains(t, er.NextAction, tc.wantNext)
		})
	}
}

func TestValidate_HumanWithReasonAndSeparatorVariantsPass(t *testing.T) {
	body := plan(
		block("A", "**Id:** a", "**Repo:** `docs`", "**Depends on:** NONE", "**Execution:** human — sign-off by legal"),
		block("B", "**Id:** b", "**Repo:** spektacular", "**Depends on:**", "- `a` - A", "**Execution:** human: needs the vault"),
	)
	p := Parse(body)
	require.NoError(t, Validate(p, repos))
	require.Equal(t, "docs", p.Tasks[0].Repo)
	require.Equal(t, Execution{Type: "human", Reason: "sign-off by legal"}, p.Tasks[0].Execution)
	require.Equal(t, []string{"a"}, p.Tasks[1].DependsOn)
	require.Equal(t, Execution{Type: "human", Reason: "needs the vault"}, p.Tasks[1].Execution)
}

func TestValidate_NoRegisteredReposSaysHowToRegister(t *testing.T) {
	body := plan(block("A", "**Id:** a", "**Repo:** spektacular", "**Depends on:** none", "**Execution:** agent"))
	er := requireCode(t, Validate(Parse(body), nil), CodeTaskInvalid)
	require.Contains(t, er.NextAction, "repo add")
}
