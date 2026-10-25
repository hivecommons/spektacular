package tracelog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// runFileTimestampFormat stamps each run's file with its fixed-width,
// zero-padded start moment, so run files sort chronologically as plain
// strings. Nanosecond precision keeps two rapid restarts of the same named
// workflow from sharing a file.
const runFileTimestampFormat = "20060102T150405.000000000Z"

// standalonePrefix starts the name of every day file of standalone commands.
const standalonePrefix = "standalone_"

// gitignoreBody ignores everything in the log directory, so logs stay out of
// version control even in a project whose own ignore rules miss them.
const gitignoreBody = "*\n"

// fileLogger writes records as JSON Lines: one file per workflow run, and
// one file per UTC day for standalone commands.
type fileLogger struct {
	dir       string
	retention int
}

func newFileLogger(opts FileOptions) Logger {
	return &fileLogger{dir: opts.Directory, retention: opts.Retention}
}

// Write appends rec as one line to run's file. It never returns an error and
// never panics outward: any failure is contained and dropped, so writing a
// log can never change a command's output or exit status.
func (l *fileLogger) Write(run Run, rec Record) {
	defer func() { _ = recover() }()

	path := runFilePath(l.dir, run)
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)
	if created {
		if err := os.MkdirAll(l.dir, 0o755); err != nil {
			return
		}
		ensureGitignore(l.dir)
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, werr := f.Write(data)
	_ = f.Close()
	if werr != nil {
		return
	}

	l.sweep(created, path)
}

// sweep trims old logs when a write created a new file. Retention is added
// by a later task; until then it keeps everything.
func (l *fileLogger) sweep(created bool, current string) {
	_ = created
	_ = current
}

// runFilePath is the file a run's records go to. A workflow run's file is
// named from its start time, agent, kind and name; a standalone command's
// file from its UTC day.
func runFilePath(dir string, run Run) string {
	return filepath.Join(dir, runFileName(run))
}

func runFileName(run Run) string {
	if run.Standalone() {
		return standalonePrefix + run.StartedAt.UTC().Format("20060102") + ".jsonl"
	}
	return run.StartedAt.UTC().Format(runFileTimestampFormat) + "_" +
		agentOr(run.Agent) + "_" + sanitize(run.Kind) + "_" + sanitize(run.Name) + ".jsonl"
}

func agentOr(agent string) string {
	if agent == "" {
		return "unknown"
	}
	return sanitize(agent)
}

// sanitize replaces characters that are unsafe or awkward in a file name.
func sanitize(s string) string {
	return strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(s)
}

// ensureGitignore writes the directory's self-ignoring .gitignore when it is
// absent. Failures are dropped.
func ensureGitignore(dir string) {
	p := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.WriteFile(p, []byte(gitignoreBody), 0o644)
}
