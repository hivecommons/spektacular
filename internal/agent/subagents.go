package agent

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/stepkit"
)

// GoalReviewerName is the name every installed goal reviewer helper carries,
// the one the implement workflow's check_goal step tells the agent to launch.
const GoalReviewerName = "spek-goal-reviewer"

// goalReviewer is one agent's installed goal reviewer: where it is written,
// the template it is rendered from, and the model it runs on unless the
// project's implement.goal_check.models names another.
type goalReviewer struct {
	path         string // project-relative
	template     string // under templates/subagents
	defaultModel string
}

// goalReviewers are the reviewer helpers each agent installs, in that
// agent's own sub-agent format. Each default model is a smaller, cheaper tier
// than the agent's default working model (checked 2026-10-21):
//
//   - claude: the `haiku` alias. Claude Code sub-agents live in
//     .claude/agents with `tools` and `model` frontmatter
//     (https://code.claude.com/docs/en/sub-agents).
//   - codex: `gpt-6-luna`, which the Codex docs name "a faster, lower-cost
//     option for lighter subagent work" beside the default `gpt-6.1-sol`.
//     Codex custom agents live in .codex/agents as TOML; `sandbox_mode` in
//     an agent file does not override the parent session, so the reviewer
//     is read-only by instruction only
//     (https://learn.chatgpt.com/docs/agent-configuration/subagents).
//   - copilot: `Claude Haiku 4.5`, a small model in Copilot's supported
//     model list. Copilot custom agents live in .github/agents with `tools`
//     and `model` frontmatter; `read` and `search` are the read-only tool
//     aliases (https://docs.github.com/en/copilot/reference/custom-agents-configuration,
//     https://docs.github.com/en/copilot/reference/ai-models/supported-models).
//   - omp: the `@smol` role alias, which omp maps to the user's own cheap
//     model through its model roles. omp agents live in .omp/agents with
//     `tools` and `model` frontmatter; read, grep and glob are its plan-mode
//     read-only tools
//     (https://github.com/can1357/oh-my-pi/blob/main/docs/task-agent-discovery.md).
//
// Bob has no per-helper model setting and installs none: its reviewer is the
// built-in read-only explore helper, which the check_goal step names.
var goalReviewers = map[string]goalReviewer{
	"claude":  {path: ".claude/agents/" + GoalReviewerName + ".md", template: "subagents/claude.md", defaultModel: "haiku"},
	"codex":   {path: ".codex/agents/" + GoalReviewerName + ".toml", template: "subagents/codex.toml", defaultModel: "gpt-6-luna"},
	"copilot": {path: ".github/agents/" + GoalReviewerName + ".agent.md", template: "subagents/copilot.agent.md", defaultModel: "Claude Haiku 4.5"},
	"omp":     {path: ".omp/agents/" + GoalReviewerName + ".md", template: "subagents/omp.md", defaultModel: "@smol"},
}

// GoalReviewerModel is the model agent's installed reviewer runs on: the
// project's implement.goal_check.models override for that agent, or the
// agent's default. It is "" for an agent that installs no reviewer.
func GoalReviewerModel(cfg config.Config, agent string) string {
	r, ok := goalReviewers[agent]
	if !ok {
		return ""
	}
	if m := cfg.Implement.GoalCheck.Models[agent]; m != "" {
		return m
	}
	return r.defaultModel
}

// installGoalReviewer writes agent's goal reviewer helper into projectPath,
// rendered from its template with the shared reviewer brief and the
// effective model. Re-running it rewrites the same bytes.
func installGoalReviewer(projectPath, agent string, cfg config.Config, out io.Writer) error {
	r, ok := goalReviewers[agent]
	if !ok {
		return fmt.Errorf("no goal reviewer is defined for agent %q", agent)
	}
	tmpl, err := fs.ReadFile(sourceFS, r.template)
	if err != nil {
		return fmt.Errorf("reading embedded reviewer template %s: %w", r.template, err)
	}
	rendered, err := mustache.RenderPartials(string(tmpl), stepkit.FSPartials{FS: sourceFS},
		map[string]string{"model": GoalReviewerModel(cfg, agent), "command": cfg.Command})
	if err != nil {
		return fmt.Errorf("rendering reviewer template %s: %w", r.template, err)
	}
	dest := filepath.Join(projectPath, filepath.FromSlash(r.path))
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("creating reviewer directory %s: %w", filepath.Dir(dest), err)
	}
	if err := writeFileAtomic(dest, []byte(rendered)); err != nil {
		return err
	}
	fmt.Fprintf(out, "  Agent:    %s\n", dest)
	return nil
}
