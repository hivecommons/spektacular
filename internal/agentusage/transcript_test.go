package agentusage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ts is a fixture timestamp on 2026-10-25, as "15:04:05.000".
func ts(t *testing.T, clock string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, "2026-10-25T"+clock+"Z")
	require.NoError(t, err)
	return v
}

const fixtureProject = "testdata/claude/-tmp-proj"

func TestReadTranscriptCountsAResponseOnceWithItsLargestUsageAndFirstTime(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)

	// msg_1 is written over three records; the third carries more output.
	// The <synthetic> message with no tokens is not a request.
	require.Equal(t, []Request{
		{At: ts(t, "10:00:01.000"), Model: "claude-opus-5-5", Usage: Usage{Input: 3, Output: 40, CacheRead: 100, CacheWrite: 20}},
		{At: ts(t, "10:01:00.000"), Model: "claude-haiku-5-5", Usage: Usage{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}},
		{At: ts(t, "10:02:00.000"), Model: "claude-opus-5-5", Usage: Usage{Input: 5, Output: 6, CacheRead: 7, CacheWrite: 8}},
	}, p.transcript.Requests)
	require.Equal(t, ts(t, "10:00:00.000"), p.transcript.Start)
	require.Equal(t, ts(t, "10:02:00.000"), p.transcript.End)
}

func TestReadTranscriptPairsShellCallsWithTheirResults(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)

	require.Equal(t, []ShellCall{
		{Command: `spektacular plan new --data '{"name":"demo"}'`, Start: ts(t, "10:00:01.500"), End: ts(t, "10:00:03.000")},
		{Command: `spektacular   plan goto --data '{"step":"overview"}'`, Start: ts(t, "10:01:00.000"), End: ts(t, "10:01:05.000")},
		// No result was recorded: it ends where it started.
		{Command: "sleep 1", Start: ts(t, "10:02:00.000"), End: ts(t, "10:02:00.000")},
	}, p.transcript.Calls)
}

func TestReadTranscriptRecordsHelperLaunchesWithTheirBrief(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)

	require.Equal(t, []Launch{{
		ToolUseID:   "toolu_t1",
		Description: "Research the code",
		Prompt:      "Find the loader. Your parent span is 0123456789abcdef0123456789abcdef-0123456789abcdef.",
		At:          ts(t, "10:01:00.000"),
	}}, p.transcript.Launches)
}

func TestReadTranscriptReadsClaudeCodesOwnTotals(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)
	require.Equal(t, map[string]Usage{
		"claude-opus-5-5":  {Input: 1000, Output: 2000, CacheRead: 3000, CacheWrite: 4000},
		"claude-haiku-5-5": {Input: 10, Output: 20, CacheRead: 30, CacheWrite: 40},
	}, p.totals)

	// A helper transcript holds no totals: they are absent, not zero.
	h, err := readTranscript(filepath.Join(fixtureProject, "s1", "subagents", "agent-a1.jsonl"))
	require.NoError(t, err)
	require.Nil(t, h.totals)
}

func TestReadTranscriptSkipsAndCountsDamagedLines(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)
	// One assistant line is cut off; attachment, request-body and other
	// record types are ignored, not counted.
	require.Equal(t, 1, p.skipped)
}

func TestReadTranscriptReadsALastLineWithNoNewline(t *testing.T) {
	p, err := readTranscript(filepath.Join(fixtureProject, "s3.jsonl"))
	require.NoError(t, err)
	require.Len(t, p.transcript.Requests, 1)
	require.Equal(t, 1, p.recognised)
}

func TestReadSessionLinksHelpersToTheirLaunchAndParent(t *testing.T) {
	s, problems, _, err := readSession(filepath.Join(fixtureProject, "s1.jsonl"))
	require.NoError(t, err)
	require.Empty(t, problems)

	require.Equal(t, "s1", s.ID)
	require.Len(t, s.Transcripts, 3)
	main, a1, a2 := s.Transcripts[0], s.Transcripts[1], s.Transcripts[2]

	require.Equal(t, "main", main.ID)
	require.Empty(t, main.LaunchID)

	require.Equal(t, "a1", a1.ID)
	require.Equal(t, "toolu_t1", a1.LaunchID, "linked to the main agent's Agent call")
	require.Empty(t, a1.ParentID, "started by the main agent")
	require.Equal(t, "Research the code", a1.Description)
	require.Equal(t, []Request{
		{At: ts(t, "10:01:02.000"), Model: "claude-opus-5-5", Usage: Usage{Input: 11, Output: 12, CacheRead: 13, CacheWrite: 14}},
		{At: ts(t, "10:01:30.000"), Model: "claude-opus-5-5", Usage: Usage{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}},
	}, a1.Requests)
	require.Equal(t, "toolu_t2", a1.Launches[0].ToolUseID)

	require.Equal(t, "a2", a2.ID)
	require.Equal(t, "toolu_t2", a2.LaunchID, "linked to a1's Agent call")
	require.Equal(t, "a1", a2.ParentID, "a depth-2 helper names the helper that started it")
	require.Equal(t, filepath.Join(fixtureProject, "s1", "subagents", "agent-a2.jsonl"), a2.File)

	require.NotNil(t, s.Totals)
	require.Equal(t, 1, s.Skipped)
}

func TestReadSessionReadsAHelperWithNoMetadata(t *testing.T) {
	dir := copyFixtures(t)
	meta := filepath.Join(dir, "-tmp-proj", "s1", "subagents", "agent-a1.meta.json")
	require.NoError(t, removeFile(meta))

	s, _, _, err := readSession(filepath.Join(dir, "-tmp-proj", "s1.jsonl"))
	require.NoError(t, err)
	require.Equal(t, "a1", s.Transcripts[1].ID)
	require.Empty(t, s.Transcripts[1].LaunchID)
	require.Len(t, s.Transcripts[1].Requests, 2)
}

func TestUsageAddAndSub(t *testing.T) {
	a := Usage{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}
	b := Usage{Input: 10, Output: 20, CacheRead: 30, CacheWrite: 40}
	require.Equal(t, Usage{Input: 11, Output: 22, CacheRead: 33, CacheWrite: 44}, a.Add(b))
	require.Equal(t, Usage{Input: 9, Output: 18, CacheRead: 27, CacheWrite: 36}, b.Sub(a))
	require.True(t, Usage{}.IsZero())
	require.False(t, a.IsZero())
}
