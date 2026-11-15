// Package meeting implements `spektacular meeting`, the live mode a hive runs
// as a long-lived child process for each meeting it attends.
//
// The wire format is section 2 of the quorum contract
// (hivecommons/quorum docs/contract.md, version quorum.contract/v1): one JSON
// object per line in both directions. Hive writes events (transcript
// segments, chat, answers, research updates, end) to the child's stdin and
// reads intents (chat.post, ask, notes.append, research, say, heartbeat) from
// its stdout. Hive is the policy point; this package never talks to a meeting.
package meeting

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

// ContractVersion is the version of the quorum contract this package speaks.
const ContractVersion = "quorum.contract/v1"

// Input event types (stdin, hive to Spek).
const (
	EventSegment        = "segment"
	EventChat           = "chat"
	EventAnswer         = "answer"
	EventResearchUpdate = "research_update"
	EventEnd            = "end"
)

// Intent types (stdout, Spek to hive).
const (
	IntentChatPost    = "chat.post"
	IntentAsk         = "ask"
	IntentNotesAppend = "notes.append"
	IntentResearch    = "research"
	IntentSay         = "say"
	IntentHeartbeat   = "heartbeat"
)

// Notes sections the contract allows on notes.append.
const (
	SectionDecisions = "decisions"
	SectionActions   = "actions"
	SectionQuestions = "questions"
	SectionSummary   = "summary"
	SectionNotes     = "notes"
)

// Research statuses a research_update may carry.
const (
	ResearchQueued  = "queued"
	ResearchRunning = "running"
	ResearchDone    = "done"
	ResearchFailed  = "failed"
)

// MaxTextChars bounds every text field Spek emits. It matches quorum's
// QUORUM_CHAT_MAX_CHARS default, so hive never has to truncate a chat.post.
const MaxTextChars = 2000

// Person is a speaker or chat author.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Event is one stdin line. Fields not used by a type are left zero; unknown
// fields are ignored, as the contract requires.
type Event struct {
	Type      string  `json:"type"`
	MeetingID string  `json:"meeting_id,omitempty"`
	Seq       int64   `json:"seq,omitempty"`
	Speaker   *Person `json:"speaker,omitempty"`
	Author    *Person `json:"author,omitempty"`
	Text      string  `json:"text,omitempty"`
	Start     float64 `json:"start,omitempty"`
	End       float64 `json:"end,omitempty"`
	Final     bool    `json:"final,omitempty"`
	At        string  `json:"at,omitempty"`
	QID       string  `json:"qid,omitempty"`
	By        string  `json:"by,omitempty"`
	RID       string  `json:"rid,omitempty"`
	Status    string  `json:"status,omitempty"`
	Summary   string  `json:"summary,omitempty"`
	URL       string  `json:"url,omitempty"`
}

// ParseEvent decodes one stdin line. A line that is not a JSON object, has
// no type, or misses a field its type needs is an error the caller logs and
// skips. An unknown type is returned as-is; the engine logs and skips it.
func ParseEvent(line []byte) (Event, error) {
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return Event{}, fmt.Errorf("not a JSON object: %w", err)
	}
	ev.Type = strings.TrimSpace(ev.Type)
	switch ev.Type {
	case "":
		return Event{}, fmt.Errorf("missing type")
	case EventSegment, EventChat:
		if strings.TrimSpace(ev.Text) == "" {
			return Event{}, fmt.Errorf("%s without text", ev.Type)
		}
	case EventAnswer:
		if ev.QID == "" || strings.TrimSpace(ev.Text) == "" {
			return Event{}, fmt.Errorf("answer needs qid and text")
		}
	case EventResearchUpdate:
		if ev.RID == "" {
			return Event{}, fmt.Errorf("research_update without rid")
		}
		switch ev.Status {
		case ResearchQueued, ResearchRunning, ResearchDone, ResearchFailed:
		default:
			return Event{}, fmt.Errorf("research_update with unknown status %q", ev.Status)
		}
	}
	return ev, nil
}

// Intent is one stdout line. Only the fields its type carries are set, and
// MarshalJSON writes only those.
type Intent struct {
	Type    string
	Text    string
	QID     string
	RID     string
	Query   string
	Reason  string
	Section string
}

// wireIntent fixes the key order on the wire: type first.
type wireIntent struct {
	Type    string `json:"type"`
	QID     string `json:"qid,omitempty"`
	RID     string `json:"rid,omitempty"`
	Section string `json:"section,omitempty"`
	Query   string `json:"query,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Text    string `json:"text,omitempty"`
}

// MarshalJSON writes the intent in the contract's exact shape: only the
// fields its type carries.
func (in Intent) MarshalJSON() ([]byte, error) {
	w := wireIntent{Type: in.Type}
	switch in.Type {
	case IntentChatPost, IntentSay:
		w.Text = in.Text
	case IntentAsk:
		w.QID, w.Text = in.QID, in.Text
	case IntentNotesAppend:
		w.Section, w.Text = in.Section, in.Text
	case IntentResearch:
		w.RID, w.Query, w.Reason = in.RID, in.Query, in.Reason
	}
	return json.Marshal(w)
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Validate reports whether the intent is a valid contract line. The encoder
// refuses anything that fails it, so stdout only ever carries valid lines.
func (in Intent) Validate() error {
	text := func(field, v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s: empty %s", in.Type, field)
		}
		if n := len([]rune(v)); n > MaxTextChars {
			return fmt.Errorf("%s: %s is %d characters, over %d", in.Type, field, n, MaxTextChars)
		}
		return nil
	}
	id := func(field, v string) error {
		if !idPattern.MatchString(v) {
			return fmt.Errorf("%s: invalid %s %q", in.Type, field, v)
		}
		return nil
	}
	switch in.Type {
	case IntentHeartbeat:
		return nil
	case IntentChatPost, IntentSay:
		return text("text", in.Text)
	case IntentAsk:
		if err := id("qid", in.QID); err != nil {
			return err
		}
		return text("text", in.Text)
	case IntentNotesAppend:
		switch in.Section {
		case SectionDecisions, SectionActions, SectionQuestions, SectionSummary, SectionNotes:
		default:
			return fmt.Errorf("notes.append: unknown section %q", in.Section)
		}
		return text("text", in.Text)
	case IntentResearch:
		if err := id("rid", in.RID); err != nil {
			return err
		}
		if err := text("query", in.Query); err != nil {
			return err
		}
		return text("reason", in.Reason)
	default:
		return fmt.Errorf("unknown intent type %q", in.Type)
	}
}

// Encoder writes intents as JSON lines. It is safe for concurrent use and
// flushes after every line, so hive sees each intent as soon as it is made.
type Encoder struct {
	mu  sync.Mutex
	w   *bufio.Writer
	log io.Writer
}

// NewEncoder returns an encoder writing to w and reporting refused intents
// to log.
func NewEncoder(w, log io.Writer) *Encoder {
	return &Encoder{w: bufio.NewWriter(w), log: log}
}

// Emit validates and writes one intent. An invalid intent is logged and
// dropped; the returned error is only for write failures.
func (e *Encoder) Emit(in Intent) error {
	if err := in.Validate(); err != nil {
		fmt.Fprintf(e.log, "spektacular meeting: dropped invalid intent: %v\n", err)
		return nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return e.w.Flush()
}
