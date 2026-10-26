package agentusage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"
)

// Claude Code record types the reader uses; every other record is skipped
// before it is decoded.
var (
	markAssistant = []byte(`"type":"assistant"`)
	markUser      = []byte(`"type":"user"`)
	markCostState = []byte(`"type":"cost-state"`)
)

// Claude Code's placeholder model for messages it writes itself, which no
// model gave and which carry no tokens.
const syntheticModel = "<synthetic>"

// parsed is one transcript file as read: the transcript, the lines that
// could not be read, the agent's own totals when the file holds them, and
// how many records it recognised (responses with usage, and totals).
type parsed struct {
	transcript Transcript
	skipped    int
	totals     map[string]Usage
	recognised int
}

type rawRecord struct {
	Type       string                   `json:"type"`
	Timestamp  time.Time                `json:"timestamp"`
	Message    json.RawMessage          `json:"message"`
	ModelUsage map[string]rawModelUsage `json:"modelUsage"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type rawUsage struct {
	Input      int64 `json:"input_tokens"`
	Output     int64 `json:"output_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
}

type rawModelUsage struct {
	Input      int64 `json:"inputTokens"`
	Output     int64 `json:"outputTokens"`
	CacheRead  int64 `json:"cacheReadInputTokens"`
	CacheWrite int64 `json:"cacheCreationInputTokens"`
}

// rawBlock is a content block: a tool_use in an assistant record, or a
// tool_result in a user record. A tool_result's own content is never
// decoded.
type rawBlock struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Input     *rawInput `json:"input"`
	ToolUseID string    `json:"tool_use_id"`
}

type rawInput struct {
	Command     string `json:"command"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

// response is one model response being assembled over its records.
type response struct {
	at    time.Time
	model string
	usage Usage
}

// readTranscript reads one Claude Code transcript file, read-only and line
// by line with no cap on a line's length. A last line with no newline is
// still read; blank lines are ignored; a line that does not decode is
// skipped and counted.
func readTranscript(path string) (parsed, error) {
	f, err := os.Open(path)
	if err != nil {
		return parsed{}, err
	}
	defer f.Close()

	p := parsed{transcript: Transcript{File: path}}
	t := &p.transcript
	responses := map[string]*response{}
	var order []string
	pending := map[string]int{} // Bash tool-call id to its index in Calls

	seen := func(at time.Time) {
		if at.IsZero() {
			return
		}
		if t.Start.IsZero() || at.Before(t.Start) {
			t.Start = at
		}
		if at.After(t.End) {
			t.End = at
		}
	}

	br := bufio.NewReader(f)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 && wanted(line) {
			var rec rawRecord
			if json.Unmarshal(line, &rec) != nil {
				p.skipped++
			} else {
				switch rec.Type {
				case "assistant":
					seen(rec.Timestamp)
					if !p.assistant(rec, responses, &order, pending) {
						p.skipped++
					}
				case "user":
					seen(rec.Timestamp)
					if !user(rec, t, pending) {
						p.skipped++
					}
				case "cost-state":
					p.recognised++
					p.totals = map[string]Usage{}
					for model, u := range rec.ModelUsage {
						p.totals[model] = Usage{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return parsed{}, rerr
		}
	}

	for _, id := range order {
		r := responses[id]
		if r.model == syntheticModel && r.usage.IsZero() {
			continue
		}
		t.Requests = append(t.Requests, Request{At: r.at, Model: r.model, Usage: r.usage})
	}
	sort.SliceStable(t.Requests, func(i, j int) bool { return t.Requests[i].At.Before(t.Requests[j].At) })
	sort.SliceStable(t.Calls, func(i, j int) bool { return t.Calls[i].Start.Before(t.Calls[j].Start) })
	sort.SliceStable(t.Launches, func(i, j int) bool { return t.Launches[i].At.Before(t.Launches[j].At) })
	return p, nil
}

// wanted reports whether a line is a record type the reader uses, by
// substring, before it is decoded.
func wanted(line []byte) bool {
	return bytes.Contains(line, markAssistant) || bytes.Contains(line, markUser) || bytes.Contains(line, markCostState)
}

// assistant folds one assistant record into its response, keyed by message
// id: the first record gives the time and model, and each token type keeps
// the largest count any record gave. Its tool calls are recorded. It
// reports false for a record with no message.
func (p *parsed) assistant(rec rawRecord, responses map[string]*response, order *[]string, pending map[string]int) bool {
	var m rawMessage
	if len(rec.Message) == 0 || json.Unmarshal(rec.Message, &m) != nil {
		return false
	}
	if m.Usage != nil && m.ID != "" {
		p.recognised++
		r, ok := responses[m.ID]
		if !ok {
			r = &response{at: rec.Timestamp, model: m.Model}
			responses[m.ID] = r
			*order = append(*order, m.ID)
		}
		if r.model == "" {
			r.model = m.Model
		}
		r.usage = Usage{
			Input:      max(r.usage.Input, m.Usage.Input),
			Output:     max(r.usage.Output, m.Usage.Output),
			CacheRead:  max(r.usage.CacheRead, m.Usage.CacheRead),
			CacheWrite: max(r.usage.CacheWrite, m.Usage.CacheWrite),
		}
	}

	t := &p.transcript
	for _, b := range blocks(m.Content) {
		if b.Type != "tool_use" || b.Input == nil {
			continue
		}
		switch b.Name {
		case "Bash":
			if _, dup := pending[b.ID]; dup || b.ID == "" {
				continue
			}
			pending[b.ID] = len(t.Calls)
			t.Calls = append(t.Calls, ShellCall{Command: b.Input.Command, Start: rec.Timestamp, End: rec.Timestamp})
		case "Agent", "Task":
			t.Launches = append(t.Launches, Launch{ToolUseID: b.ID, Description: b.Input.Description, Prompt: b.Input.Prompt, At: rec.Timestamp})
		}
	}
	return true
}

// user records the end of each shell call whose result the record carries.
// It reports false for a record with no message.
func user(rec rawRecord, t *Transcript, pending map[string]int) bool {
	var m rawMessage
	if len(rec.Message) == 0 || json.Unmarshal(rec.Message, &m) != nil {
		return false
	}
	for _, b := range blocks(m.Content) {
		if b.Type != "tool_result" {
			continue
		}
		if i, ok := pending[b.ToolUseID]; ok && rec.Timestamp.After(t.Calls[i].Start) {
			t.Calls[i].End = rec.Timestamp
		}
	}
	return true
}

// blocks decodes a message's content when it is a list of blocks; plain
// text content has none.
func blocks(content json.RawMessage) []rawBlock {
	c := bytes.TrimSpace(content)
	if len(c) == 0 || c[0] != '[' {
		return nil
	}
	var bs []rawBlock
	if json.Unmarshal(c, &bs) != nil {
		return nil
	}
	return bs
}
