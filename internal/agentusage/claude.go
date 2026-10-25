package agentusage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Claude reads Claude Code's session transcripts under Dir, its projects
// directory: one folder per project, each holding <session>.jsonl and, for
// a session that launched helpers, <session>/subagents/agent-<id>.jsonl
// with agent-<id>.meta.json beside it.
type Claude struct{ Dir string }

// DefaultClaudeDir is Claude Code's projects directory:
// $CLAUDE_CONFIG_DIR/projects when the variable is set, otherwise
// ~/.claude/projects. It is "" when neither can be worked out.
func DefaultClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// Sessions reads every session with activity at or after since: a session
// whose main file or any helper file was modified then or later. Sessions
// are in path order. It never fails: a missing directory, an unreadable
// file or a file with no record it recognises comes back as a Problem, and
// the other sessions are still read.
func (c Claude) Sessions(since time.Time) ([]*Session, []Problem) {
	if c.Dir == "" {
		return nil, []Problem{{Code: ProblemDirMissing, Message: "no Claude Code projects directory: set CLAUDE_CONFIG_DIR or HOME, or pass --sessions-dir"}}
	}
	projects, err := os.ReadDir(c.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, []Problem{{Code: ProblemDirMissing, Message: fmt.Sprintf("the Claude Code projects directory %s does not exist", c.Dir)}}
	}
	if err != nil {
		return nil, []Problem{{Code: ProblemUnreadable, Message: fmt.Sprintf("the Claude Code projects directory %s cannot be read: %v", c.Dir, err)}}
	}

	var candidates []string
	var problems []Problem
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(c.Dir, p.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			problems = append(problems, Problem{Code: ProblemUnreadable, Message: fmt.Sprintf("project directory %s cannot be read: %v", dir, err)})
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			fi, err := e.Info()
			if err != nil {
				problems = append(problems, Problem{Code: ProblemUnreadable, Message: fmt.Sprintf("session %s: %v", path, err)})
				continue
			}
			if since.IsZero() || latestChange(path, fi) >= since.UnixNano() {
				candidates = append(candidates, path)
			}
		}
	}
	sort.Strings(candidates)

	var sessions []*Session
	for _, path := range candidates {
		s, ps, recognised, err := readSession(path)
		problems = append(problems, ps...)
		if err != nil {
			problems = append(problems, Problem{Code: ProblemUnreadable, Message: fmt.Sprintf("session %s: %v", path, err)})
			continue
		}
		if recognised == 0 {
			problems = append(problems, Problem{Code: ProblemUnrecognised, Message: fmt.Sprintf("session %s holds no Claude Code record with token usage", path)})
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, problems
}
