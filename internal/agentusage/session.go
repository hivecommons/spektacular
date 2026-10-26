package agentusage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// helperPaths are a session's helper transcripts,
// <session>/subagents/agent-<id>.jsonl, sorted by name.
func helperPaths(mainPath string) []string {
	dir := filepath.Join(strings.TrimSuffix(mainPath, ".jsonl"), "subagents")
	paths, _ := filepath.Glob(filepath.Join(dir, "agent-*.jsonl"))
	sort.Strings(paths)
	return paths
}

// readSession reads a Claude Code session: its main transcript, then each
// helper's with its metadata. The agent's own totals come from the main
// file. A helper that cannot be read is left out with a problem; a main
// file that cannot be read is an error. recognised counts the records
// recognised over every file.
func readSession(mainPath string) (s *Session, problems []Problem, recognised int, err error) {
	id := strings.TrimSuffix(filepath.Base(mainPath), ".jsonl")
	main, err := readTranscript(mainPath)
	if err != nil {
		return nil, nil, 0, err
	}
	main.transcript.ID = "main"
	s = &Session{ID: id, Transcripts: []Transcript{main.transcript}, Totals: main.totals, Skipped: main.skipped}
	recognised = main.recognised

	for _, path := range helperPaths(mainPath) {
		h, err := readTranscript(path)
		if err != nil {
			problems = append(problems, Problem{Code: ProblemUnreadable, Message: fmt.Sprintf("session %s: helper transcript %s: %v", id, path, err)})
			continue
		}
		h.transcript.ID = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "agent-"), ".jsonl")
		readMeta(strings.TrimSuffix(path, ".jsonl")+".meta.json", &h.transcript)
		s.Transcripts = append(s.Transcripts, h.transcript)
		s.Skipped += h.skipped
		recognised += h.recognised
	}
	return s, problems, recognised, nil
}

// latestChange is the latest modification time of a session's main file
// and every helper transcript, so a session whose only recent writes are in
// its helpers' files still counts as active.
func latestChange(mainPath string, main os.FileInfo) (latest int64) {
	latest = main.ModTime().UnixNano()
	for _, p := range helperPaths(mainPath) {
		if fi, err := os.Stat(p); err == nil && fi.ModTime().UnixNano() > latest {
			latest = fi.ModTime().UnixNano()
		}
	}
	return latest
}
