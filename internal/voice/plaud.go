package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultPlaudCommand starts Plaud's official MCP server (free, OAuth; the
// token lives in ~/.plaud/tokens-mcp.json after `npx --loglevel=info -y @plaud-ai/mcp@latest install`).
const DefaultPlaudCommand = "npx -y @plaud-ai/mcp@latest"

// plaudListPageSize is the list_files page size requested.
const plaudListPageSize = 20

// plaudSource reads one recording through the Plaud MCP: list_files →
// get_note + get_transcript.
type plaudSource struct {
	dial Dialer
}

func newPlaud(opts Options) *plaudSource {
	if opts.Dial != nil {
		return &plaudSource{dial: opts.Dial}
	}
	command := opts.Command
	if command == "" {
		command = DefaultPlaudCommand
	}
	return &plaudSource{dial: stdioDialer(command)}
}

func (p *plaudSource) Name() string { return "plaud" }

type plaudFile struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

func (p *plaudSource) Fetch(ctx context.Context, q Query) (Note, error) {
	client, err := p.dial(ctx)
	if err != nil {
		return Note{}, err
	}
	defer func() { _ = client.Close() }()

	args := map[string]any{"page": 1, "page_size": plaudListPageSize}
	if q.Title != "" {
		args["query"] = q.Title
	}
	text, err := client.Call(ctx, "list_files", args)
	if err != nil {
		return Note{}, plaudAuthError(err)
	}
	files, err := decodePlaudFiles(text)
	if err != nil {
		return Note{}, err
	}
	i, err := pick("plaud", q, len(files),
		func(i int) string { return files[i].Name },
		func(i int) time.Time { return files[i].CreatedAt })
	if err != nil {
		return Note{}, err
	}
	f := files[i]

	noteText, noteErr := client.Call(ctx, "get_note", map[string]any{"file_id": f.ID})
	transcriptText, trErr := client.Call(ctx, "get_transcript", map[string]any{"file_id": f.ID})
	if noteErr != nil && trErr != nil {
		return Note{}, plaudAuthError(trErr)
	}
	n := Note{
		Title:      f.Name,
		Summary:    renderPlaudNote(noteText),
		Transcript: renderPlaudTranscript(transcriptText),
		URI:        "plaud://recordings/" + f.ID,
		RecordedAt: f.CreatedAt,
	}
	if n.Title == "" {
		n.Title = "Plaud recording " + f.ID
	}
	if strings.TrimSpace(n.Summary) == "" && strings.TrimSpace(n.Transcript) == "" {
		return Note{}, fmt.Errorf("plaud: recording %q (%s) has no note or transcript yet; let the Plaud app finish processing it", n.Title, f.ID)
	}
	return n, nil
}

// plaudAuthError turns a 401-shaped failure into the one next step.
func plaudAuthError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "401") || strings.Contains(strings.ToLower(msg), "not authenticated") || strings.Contains(strings.ToLower(msg), "unauthori") {
		return fmt.Errorf("plaud: not signed in: run `npx --loglevel=info -y @plaud-ai/mcp@latest install` (or ask your agent to \"log me into Plaud\") and retry: %w", err)
	}
	return err
}

// decodePlaudFiles accepts a JSON array of recordings, or an object wrapping
// one under files/items/data/results/recordings.
func decodePlaudFiles(text string) ([]plaudFile, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		if i := strings.IndexAny(text, "[{"); i > 0 {
			return decodePlaudFiles(text[i:])
		}
		return nil, fmt.Errorf("plaud: list_files returned non-JSON output: %s", firstLine(text))
	}
	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case map[string]any:
		for _, key := range []string{"files", "items", "data", "results", "recordings"} {
			if arr, ok := v[key].([]any); ok {
				items = arr
				break
			}
		}
		if items == nil {
			if _, ok := v["id"]; ok {
				items = []any{v}
			}
		}
	}
	out := make([]plaudFile, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		f := plaudFile{
			ID:        jsonString(m, "id", "file_id"),
			Name:      jsonString(m, "name", "title", "filename"),
			CreatedAt: jsonTime(m, "created_at", "start_at", "create_time"),
		}
		if f.ID != "" {
			out = append(out, f)
		}
	}
	return out, nil
}

func jsonString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

func jsonTime(m map[string]any, keys ...string) time.Time {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
				if t, err := time.Parse(layout, v); err == nil {
					return t
				}
			}
		case float64:
			// Epoch seconds or milliseconds.
			if v > 1e12 {
				return time.UnixMilli(int64(v))
			}
			return time.Unix(int64(v), 0)
		}
	}
	return time.Time{}
}

// renderPlaudNote returns get_note output as Markdown: plain text passes
// through; JSON is walked for its note_list/markdown/content strings.
func renderPlaudNote(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return text
	}
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return text
	}
	var parts []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				parts = append(parts, s)
			}
		case []any:
			for _, it := range t {
				walk(it)
			}
		case map[string]any:
			if s := jsonString(t, "title"); s != "" {
				parts = append(parts, "### "+s)
			}
			for _, key := range []string{"note_list", "notes", "markdown", "content", "summary", "text", "note"} {
				if inner, ok := t[key]; ok {
					walk(inner)
					return
				}
			}
		}
	}
	walk(raw)
	return strings.Join(parts, "\n\n")
}

// renderPlaudTranscript turns get_transcript output into "[mm:ss] Speaker:
// text" lines when it is JSON; plain text passes through.
func renderPlaudTranscript(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return text
	}
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return text
	}
	var segs []any
	switch t := raw.(type) {
	case []any:
		segs = t
	case map[string]any:
		for _, key := range []string{"source_list", "segments", "transcript", "items", "data"} {
			if arr, ok := t[key].([]any); ok {
				segs = arr
				break
			}
			if s, ok := t[key].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	var b strings.Builder
	for _, s := range segs {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		line := jsonString(m, "text", "content", "sentence")
		if line == "" {
			continue
		}
		if ms, ok := m["start_time"].(float64); ok {
			sec := int(ms / 1000)
			fmt.Fprintf(&b, "[%02d:%02d] ", sec/60, sec%60)
		}
		if sp := jsonString(m, "speaker", "speaker_name"); sp != "" {
			b.WriteString(sp + ": ")
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSpace(b.String())
}
