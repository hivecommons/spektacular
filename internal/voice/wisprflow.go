package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// wisprFlowHistorySQL reads recent dictations from Wispr Flow's local
// history; the best available text wins (edited > formatted > raw ASR).
const wisprFlowHistorySQL = `SELECT transcriptEntityId AS id, ` +
	`COALESCE(NULLIF(editedText,''), NULLIF(formattedText,''), asrText) AS text, ` +
	`timestamp FROM History WHERE status='formatted' AND isArchived=0 ` +
	`ORDER BY timestamp DESC LIMIT 50`

// wisprFlowSource reads Wispr Flow dictations. Path may be the app's
// flow.sqlite (the default) or a folder of exported .md/.txt notes.
type wisprFlowSource struct {
	run Runner
}

func newWisprFlow(opts Options) *wisprFlowSource {
	run := opts.Run
	if run == nil {
		run = execRunner
	}
	return &wisprFlowSource{run: run}
}

func execRunner(ctx context.Context, argv ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	if err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s: %s", argv[0], firstLine(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("%s: %w", argv[0], err)
	}
	return out, nil
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// DefaultWisprFlowPath is where the macOS app keeps its history.
func DefaultWisprFlowPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Wispr Flow", "flow.sqlite")
}

func (w *wisprFlowSource) Name() string { return "wisprflow" }

func (w *wisprFlowSource) Fetch(ctx context.Context, q Query) (Note, error) {
	path := q.Path
	if path == "" {
		path = DefaultWisprFlowPath()
	}
	info, err := os.Stat(path)
	if err != nil {
		return Note{}, fmt.Errorf("wisprflow: %w (is Wispr Flow installed? pass --path to a flow.sqlite or an export folder)", err)
	}
	if info.IsDir() {
		return fetchFromFolder("wisprflow", path, q)
	}
	out, err := w.run(ctx, "sqlite3", "-readonly", "-json", path, wisprFlowHistorySQL)
	if err != nil {
		return Note{}, fmt.Errorf("wisprflow: reading history: %w", err)
	}
	var rows []struct {
		ID        string `json:"id"`
		Text      string `json:"text"`
		Timestamp string `json:"timestamp"`
	}
	if strings.TrimSpace(string(out)) != "" {
		if err := json.Unmarshal(out, &rows); err != nil {
			return Note{}, fmt.Errorf("wisprflow: decoding history: %w", err)
		}
	}
	times := make([]time.Time, len(rows))
	for i, r := range rows {
		times[i] = parseLooseTime(r.Timestamp)
	}
	i, err := pick("wisprflow", q, len(rows),
		func(i int) string { return firstLine(rows[i].Text) },
		func(i int) time.Time { return times[i] })
	if err != nil {
		return Note{}, err
	}
	r := rows[i]
	return Note{
		Title:      titleFromText(r.Text),
		Transcript: strings.TrimSpace(r.Text),
		URI:        "wisprflow://history/" + r.ID,
		RecordedAt: times[i],
	}, nil
}

func parseLooseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05", "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
