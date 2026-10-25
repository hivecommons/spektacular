package agentusage

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// copyFixtures copies testdata/claude into a temp directory and returns the
// copy's projects directory.
func copyFixtures(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := "testdata/claude"
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	require.NoError(t, err)
	return dst
}

func removeFile(path string) error { return os.Remove(path) }

// touchAll sets every file under dir to mtime.
func touchAll(t *testing.T, dir string, mtime time.Time) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, mtime, mtime)
	}))
}

// hashes maps every file under dir to the hash of its bytes, mode and
// modification time.
func hashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[path] = hex.EncodeToString(sum[:]) + " " + fi.Mode().String() + " " + fi.ModTime().UTC().String()
		return nil
	}))
	return out
}

func ids(ss []*Session) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func codes(ps []Problem) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Code)
	}
	return out
}

var old = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func TestSessionsReadsOnlySessionsActiveSinceTheGivenTime(t *testing.T) {
	dir := copyFixtures(t)
	touchAll(t, dir, old)
	since := old.Add(time.Hour)

	// Every file is older than since: nothing is read.
	ss, problems := Claude{Dir: dir}.Sessions(since)
	require.Empty(t, ss)
	require.Empty(t, problems)

	// s3's own file is modified at since: it is read.
	s3 := filepath.Join(dir, "-tmp-proj", "s3.jsonl")
	require.NoError(t, os.Chtimes(s3, since, since))
	ss, _ = Claude{Dir: dir}.Sessions(since)
	require.Equal(t, []string{"s3"}, ids(ss))
}

func TestSessionsCountsActivityInHelperFilesOnly(t *testing.T) {
	dir := copyFixtures(t)
	touchAll(t, dir, old)
	since := old.Add(time.Hour)

	// Only s1's depth-2 helper was written after since.
	helper := filepath.Join(dir, "-tmp-proj", "s1", "subagents", "agent-a2.jsonl")
	later := since.Add(time.Minute)
	require.NoError(t, os.Chtimes(helper, later, later))

	ss, problems := Claude{Dir: dir}.Sessions(since)
	require.Empty(t, problems)
	require.Equal(t, []string{"s1"}, ids(ss))
	require.Len(t, ss[0].Transcripts, 3)
}

func TestSessionsReportsAFileWithNoRecognisedRecords(t *testing.T) {
	dir := copyFixtures(t)
	ss, problems := Claude{Dir: dir}.Sessions(time.Time{})

	require.Equal(t, []string{"s1", "s3"}, ids(ss), "sessions in path order; s2 is left out")
	require.Equal(t, []string{ProblemUnrecognised}, codes(problems))
	require.Contains(t, problems[0].Message, "s2.jsonl")
}

func TestSessionsReportsAMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-projects")
	ss, problems := Claude{Dir: missing}.Sessions(time.Time{})
	require.Empty(t, ss)
	require.Equal(t, []string{ProblemDirMissing}, codes(problems))
	require.Contains(t, problems[0].Message, missing)

	_, problems = Claude{}.Sessions(time.Time{})
	require.Equal(t, []string{ProblemDirMissing}, codes(problems))
}

func TestSessionsReportsAnUnreadableFileAndReadsTheRest(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not stop this user reading a file")
	}
	dir := copyFixtures(t)
	s3 := filepath.Join(dir, "-tmp-proj", "s3.jsonl")
	require.NoError(t, os.Chmod(s3, 0))
	t.Cleanup(func() { _ = os.Chmod(s3, 0o644) })

	ss, problems := Claude{Dir: dir}.Sessions(time.Time{})
	require.Equal(t, []string{"s1"}, ids(ss))
	require.ElementsMatch(t, []string{ProblemUnrecognised, ProblemUnreadable}, codes(problems))
	for _, p := range problems {
		if p.Code == ProblemUnreadable {
			require.Contains(t, p.Message, "s3.jsonl")
		}
	}
}

func TestSessionsCountsSkippedLines(t *testing.T) {
	dir := copyFixtures(t)
	ss, _ := Claude{Dir: dir}.Sessions(time.Time{})
	require.Equal(t, 1, ss[0].Skipped, "s1's damaged line")
	require.Equal(t, 0, ss[1].Skipped)
}

func TestSessionsLeavesEveryFileByteIdentical(t *testing.T) {
	dir := copyFixtures(t)
	touchAll(t, dir, old)
	before := hashes(t, dir)

	ss, _ := Claude{Dir: dir}.Sessions(time.Time{})
	require.NotEmpty(t, ss)

	require.Equal(t, before, hashes(t, dir))
}

func TestDefaultClaudeDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/opt/claude")
	require.Equal(t, filepath.Join("/opt/claude", "projects"), DefaultClaudeDir())

	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	require.Equal(t, filepath.Join(home, ".claude", "projects"), DefaultClaudeDir())
}
