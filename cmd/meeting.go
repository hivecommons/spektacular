package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hivecommons/spektacular/internal/meeting"
)

const (
	meetingFromStdin   = "stdin"
	meetingFormatJSONL = "jsonl"
	// meetingIDPrefixBytes is the random part of every qid and rid, so ids
	// from two runs for one meeting never collide.
	meetingIDPrefixBytes = 2
)

var meetingCmd = &cobra.Command{
	Use:   "meeting",
	Short: "Live meeting mode: read transcript events as JSON lines on stdin, write intents as JSON lines on stdout",
	Long: `Meeting is the live mode a hive runs as a long-lived child process for each
meeting it attends. Hive writes the meeting's final transcript segments, chat,
answers to Spek's questions, research updates and the end of the meeting to
stdin, one JSON object per line; Spek writes intents to stdout, one JSON object
per line: chat.post, ask, notes.append, research, say and heartbeat. The wire
format is section 2 of the quorum contract (hivecommons/quorum docs/contract.md,
quorum.contract/v1). Hive is the policy point: Spek never talks to the meeting.

Stdout carries contract lines only. Logs, skipped input and errors go to
stderr. On an end event or EOF Spek writes a closing notes.append in section
summary and exits 0.

Spek decides at natural pauses (no final segment for SPEK_MEETING_PAUSE_MS,
default 1500) and when addressed by name (SPEK_MEETING_NAMES, default
hive,spek,spektacular), with one bounded model call per decision
(SPEK_MEETING_LLM_TIMEOUT_MS, default 2500). The model backend is
SPEK_MEETING_LLM (anthropic, command or none); see docs/meeting.md.

Example:
  spektacular meeting --from stdin --format jsonl --title "Weekly sync" --context ./meeting-ctx`,
	Args:        cobra.NoArgs,
	Annotations: map[string]string{gateAnnotation: gateExempt, logAnnotation: logExempt},
	RunE:        runMeeting,
}

func init() {
	meetingCmd.Flags().String("from", meetingFromStdin, "where events come from (only stdin)")
	meetingCmd.Flags().String("format", meetingFormatJSONL, "wire format (only jsonl)")
	meetingCmd.Flags().String("title", "", "the meeting's title")
	meetingCmd.Flags().String("context", "", "a directory of files the model may read: prior knowledge, agenda, notes")
}

// quietError is a failure already reported on stderr. runRoot exits 1 for it
// without writing the usual error envelope, so a command whose stdout is a
// line protocol keeps stdout clean.
type quietError struct{ err error }

func (q quietError) Error() string { return q.err.Error() }
func (q quietError) Unwrap() error { return q.err }

func isQuiet(err error) bool {
	var q quietError
	return errors.As(err, &q)
}

func runMeeting(cmd *cobra.Command, _ []string) error {
	stderr := cmd.ErrOrStderr()
	fail := func(format string, args ...any) error {
		err := fmt.Errorf(format, args...)
		fmt.Fprintf(stderr, "spektacular meeting: %v\n", err)
		return quietError{err}
	}

	from, _ := cmd.Flags().GetString("from")
	format, _ := cmd.Flags().GetString("format")
	title, _ := cmd.Flags().GetString("title")
	ctxDir, _ := cmd.Flags().GetString("context")
	if from != meetingFromStdin {
		return fail("--from %q is not supported; only --from %s", from, meetingFromStdin)
	}
	if format != meetingFormatJSONL {
		return fail("--format %q is not supported; only --format %s", format, meetingFormatJSONL)
	}

	settings := meeting.FromEnv(os.Getenv, stderr)
	files, err := meeting.LoadContext(ctxDir, settings.ContextMaxBytes)
	if err != nil {
		return fail("reading --context: %v", err)
	}

	// The project's configured agent picks a default one-shot command when
	// no model backend is set explicitly. Running outside a project is
	// normal for a hive, so a missing config is not an error.
	agentName := ""
	if projectConfigExists() {
		if cfg, err := loadConfigLenient(); err == nil {
			agentName = cfg.Agent
		}
	}
	decider, backend, err := meeting.SelectDecider(os.Getenv, agentName, nil, stderr)
	if err != nil {
		return fail("%v", err)
	}

	prefix := make([]byte, meetingIDPrefixBytes)
	_, _ = rand.Read(prefix)
	engine := meeting.NewEngine(meeting.Options{
		Settings: settings,
		Title:    title,
		Context:  files,
		IDPrefix: hex.EncodeToString(prefix) + "-",
		Log:      stderr,
	})
	fmt.Fprintf(stderr, "spektacular meeting: %s, model backend %s, %d context file(s), pause %s, heartbeat %s\n",
		meeting.ContractVersion, backend, len(files), settings.Pause, settings.Heartbeat)

	session := &meeting.Session{
		Engine:   engine,
		Encoder:  meeting.NewEncoder(cmd.OutOrStdout(), stderr),
		Decider:  decider,
		Settings: settings,
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := meeting.Run(ctx, session, cmd.InOrStdin(), nil); err != nil {
		return fail("writing intents: %v", err)
	}
	return nil
}
