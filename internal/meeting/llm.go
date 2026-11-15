package meeting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Model backend selection. SPEK_MEETING_LLM picks one explicitly; left
// unset, SelectDecider picks the first that is configured.
const (
	EnvLLM      = "SPEK_MEETING_LLM"
	EnvModel    = "SPEK_MEETING_MODEL"
	EnvLLMCmd   = "SPEK_MEETING_LLM_CMD"
	EnvMaxToken = "SPEK_MEETING_MAX_TOKENS"

	BackendAnthropic = "anthropic"
	BackendCommand   = "command"
	BackendNone      = "none"

	// DefaultModel is the fastest model tier; a live meeting needs a
	// decision in well under three seconds.
	DefaultModel = "claude-haiku-5-5"
	// DefaultMaxTokens bounds one decision's reply; the JSON it returns is
	// small.
	DefaultMaxTokens = 1024
)

// agentCommands are the default one-shot commands for agents whose CLI can
// answer a prompt on stdin non-interactively, using their fastest model.
// Other agents need SPEK_MEETING_LLM_CMD.
var agentCommands = map[string][]string{
	"claude": {"claude", "-p", "--model", "haiku", "--output-format", "text"},
}

// AnthropicCompleter calls the Claude API through the official SDK. The
// client reads its credentials (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN,
// ANTHROPIC_BASE_URL and the SDK's other variables) from the environment.
type AnthropicCompleter struct {
	Client    anthropic.Client
	Model     string
	MaxTokens int64
}

// Name identifies the backend in logs.
func (a AnthropicCompleter) Name() string { return BackendAnthropic + ":" + a.Model }

// Complete sends one request with structured output and low effort, and
// no retries: a late decision is worth less than a skipped one.
func (a AnthropicCompleter) Complete(ctx context.Context, system, user string, schema map[string]any) (string, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.Model),
		MaxTokens: a.MaxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	}
	msg, err := a.Client.Messages.New(ctx, params, option.WithMaxRetries(0))
	if err != nil {
		return "", err
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return "", errors.New("model declined the request")
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String(), nil
}

// CommandCompleter runs a one-shot command with the prompt on stdin and
// reads the reply from stdout; the command's stderr is passed through to
// Spek's stderr. It lets a hive use whatever agent CLI it already runs.
type CommandCompleter struct {
	Argv []string
	Log  io.Writer
}

// Name identifies the backend in logs.
func (c CommandCompleter) Name() string { return BackendCommand + ":" + strings.Join(c.Argv, " ") }

// Complete runs the command, killed when ctx expires.
func (c CommandCompleter) Complete(ctx context.Context, system, user string, _ map[string]any) (string, error) {
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Stdin = strings.NewReader(system + "\n\n" + user + "\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	if c.Log != nil {
		cmd.Stderr = c.Log
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%s: %w", c.Argv[0], err)
	}
	return out.String(), nil
}

// SelectDecider picks the model backend for a meeting:
//
//  1. SPEK_MEETING_LLM=anthropic|command|none, when set, decides;
//  2. otherwise anthropic when ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN is set;
//  3. otherwise command when SPEK_MEETING_LLM_CMD is set, or when the
//     project's configured agent has a known one-shot command on PATH;
//  4. otherwise none: the rule-based behaviour only.
//
// agent is the project's configured agent (config.yaml `agent`), or "".
// It returns the decider and a description for the startup log line.
func SelectDecider(getenv func(string) string, agent string, lookPath func(string) (string, error), log io.Writer) (Decider, string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	model := strings.TrimSpace(getenv(EnvModel))
	if model == "" {
		model = DefaultModel
	}
	maxTokens := int64(DefaultMaxTokens)
	if v := strings.TrimSpace(getenv(EnvMaxToken)); v != "" {
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil && n > 0 {
			maxTokens = n
		}
	}
	anthropicBackend := func() (Decider, string, error) {
		c := AnthropicCompleter{Client: anthropic.NewClient(), Model: model, MaxTokens: maxTokens}
		return ModelDecider{Completer: c}, c.Name(), nil
	}
	commandBackend := func(required bool) (Decider, string, error) {
		argv := strings.Fields(getenv(EnvLLMCmd))
		if len(argv) == 0 {
			if def, ok := agentCommands[agent]; ok {
				if _, err := lookPath(def[0]); err == nil {
					argv = def
				}
			}
		}
		if len(argv) == 0 {
			if required {
				return nil, "", fmt.Errorf("%s=command needs %s (the command that reads a prompt on stdin and prints the reply)", EnvLLM, EnvLLMCmd)
			}
			return nil, "", nil
		}
		c := CommandCompleter{Argv: argv, Log: log}
		return ModelDecider{Completer: c}, c.Name(), nil
	}

	switch strings.ToLower(strings.TrimSpace(getenv(EnvLLM))) {
	case BackendAnthropic:
		return anthropicBackend()
	case BackendCommand:
		return commandBackend(true)
	case BackendNone:
		return NopDecider{}, BackendNone, nil
	case "":
	default:
		return nil, "", fmt.Errorf("%s=%q is not one of %s, %s, %s", EnvLLM, getenv(EnvLLM), BackendAnthropic, BackendCommand, BackendNone)
	}
	if getenv("ANTHROPIC_API_KEY") != "" || getenv("ANTHROPIC_AUTH_TOKEN") != "" {
		return anthropicBackend()
	}
	if d, name, err := commandBackend(false); err != nil || d != nil {
		return d, name, err
	}
	return NopDecider{}, BackendNone + " (no model configured; rules only)", nil
}
