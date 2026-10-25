package agentusage

import "time"

// Problem codes: why session data could not be used. They are the reason
// codes a run report gives for unknown token figures.
const (
	// ProblemDirMissing: the sessions directory does not exist.
	ProblemDirMissing = "sessions_dir_missing"
	// ProblemUnreadable: a session file or directory could not be read.
	ProblemUnreadable = "session_unreadable"
	// ProblemUnrecognised: a session file holds no record the reader knows.
	ProblemUnrecognised = "format_unrecognised"
)

// Usage is a count of tokens of each type.
type Usage struct {
	Input, Output, CacheRead, CacheWrite int64
}

// Add is u plus v.
func (u Usage) Add(v Usage) Usage {
	return Usage{
		Input:      u.Input + v.Input,
		Output:     u.Output + v.Output,
		CacheRead:  u.CacheRead + v.CacheRead,
		CacheWrite: u.CacheWrite + v.CacheWrite,
	}
}

// Sub is u less v, field by field.
func (u Usage) Sub(v Usage) Usage {
	return Usage{
		Input:      u.Input - v.Input,
		Output:     u.Output - v.Output,
		CacheRead:  u.CacheRead - v.CacheRead,
		CacheWrite: u.CacheWrite - v.CacheWrite,
	}
}

// IsZero reports whether u counts no tokens.
func (u Usage) IsZero() bool { return u == Usage{} }

// Request is one model response: the time of its first record, the model
// that gave it, and its tokens.
type Request struct {
	At    time.Time
	Model string
	Usage Usage
}

// ShellCall is one shell command the agent ran: its text, the time it was
// issued and the time its result came back (Start when no result was
// recorded).
type ShellCall struct {
	Command    string
	Start, End time.Time
}

// Launch is one helper agent the transcript started: the launching tool
// call's id, the helper's short description and its brief, and when it was
// issued.
type Launch struct {
	ToolUseID, Description, Prompt string
	At                             time.Time
}

// Transcript is one agent's record within a session: the main agent's, or
// one helper's.
type Transcript struct {
	// ID is "main", or the helper's agent id; File is its path.
	ID, File string
	// LaunchID is the id of the tool call that started a helper; ParentID
	// is the agent id of the helper that started it ("" for a helper the
	// main agent started). Both are "" for the main transcript.
	LaunchID, ParentID string
	// Description is a helper's description from its metadata.
	Description string
	// Start and End are the first and last record times.
	Start, End time.Time
	Requests   []Request   // by time
	Calls      []ShellCall // by start
	Launches   []Launch    // by time
}

// Session is one agent session: its main transcript first, then each
// helper's, with the agent's own usage totals by model when it wrote them
// (nil otherwise), and the lines it could not read.
type Session struct {
	ID          string
	Transcripts []Transcript
	Totals      map[string]Usage
	Skipped     int
}

// Problem is why session data could not be used: one of the Problem codes
// and a message naming the path.
type Problem struct {
	Code, Message string
}
