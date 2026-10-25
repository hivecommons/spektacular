// Package agentusage reads coding agents' own session files, read-only,
// into plain data: each model request with its time, model and tokens, each
// shell call with when it started and finished, each helper launch with its
// brief, and the agent's own usage totals for the session.
//
// It knows only Claude Code's on-disk transcripts today (Claude), but its
// types are neutral, so a reader for another agent can sit beside it. It
// knows nothing of runs, logs or configuration: it never imports runstats,
// tracelog or config, and attributing tokens to the parts of a run is the
// caller's work.
//
// The package never creates, writes, renames or deletes a file, and never
// fails its caller: a missing directory, an unreadable file or a file with
// nothing it recognises comes back as a Problem with a reason code, and a
// line it cannot read is skipped and counted.
package agentusage
