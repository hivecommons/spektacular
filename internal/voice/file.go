package voice

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// fileSource reads one transcript or note file (.md, .txt, .srt), or the
// newest such file in a folder.
type fileSource struct{}

func (fileSource) Name() string { return "file" }

func (fileSource) Fetch(_ context.Context, q Query) (Note, error) {
	if q.Path == "" {
		return Note{}, fmt.Errorf("file: --path is required")
	}
	info, err := os.Stat(q.Path)
	if err != nil {
		return Note{}, fmt.Errorf("file: %w", err)
	}
	if info.IsDir() {
		return fetchFromFolder("file", q.Path, q)
	}
	return readNoteFile(q.Path, info.ModTime())
}

// stdinSource reads a pasted or dictated note from standard input.
type stdinSource struct{ r io.Reader }

func (stdinSource) Name() string { return "stdin" }

func (s stdinSource) Fetch(_ context.Context, _ Query) (Note, error) {
	if s.r == nil {
		return Note{}, fmt.Errorf("stdin: nothing to read")
	}
	data, err := io.ReadAll(s.r)
	if err != nil {
		return Note{}, fmt.Errorf("stdin: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return Note{}, fmt.Errorf("stdin: nothing to read; pipe or paste the note text")
	}
	n := noteFromText(text, "")
	n.URI = "stdin://"
	n.RecordedAt = time.Now()
	return n, nil
}

var noteExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".srt": true, ".vtt": true}

// fetchFromFolder picks the newest note file in dir (or the newest whose
// name or title matches the query).
func fetchFromFolder(source, dir string, q Query) (Note, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Note{}, fmt.Errorf("%s: %w", source, err)
	}
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	for _, e := range entries {
		if e.IsDir() || !noteExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cands = append(cands, cand{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	i, err := pick(source, q, len(cands),
		func(i int) string {
			return strings.TrimSuffix(filepath.Base(cands[i].path), filepath.Ext(cands[i].path))
		},
		func(i int) time.Time { return cands[i].mod })
	if err != nil {
		return Note{}, err
	}
	return readNoteFile(cands[i].path, cands[i].mod)
}

func readNoteFile(path string, mod time.Time) (Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Note{}, fmt.Errorf("file: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return Note{}, fmt.Errorf("file: %s is empty", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".srt" || ext == ".vtt" {
		text = stripSubtitles(text)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	n := noteFromText(text, base)
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	n.URI = "file://" + abs
	n.RecordedAt = mod
	return n, nil
}

// noteFromText treats Markdown with headings as a summary and anything else
// as a transcript. fallbackTitle is used when the text has no "# " heading.
func noteFromText(text, fallbackTitle string) Note {
	n := Note{}
	lines := strings.Split(text, "\n")
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		n.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "# "))
		text = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	if strings.Contains("\n"+text, "\n#") {
		n.Summary = text
	} else {
		n.Transcript = text
	}
	if n.Title == "" {
		n.Title = fallbackTitle
	}
	if n.Title == "" {
		n.Title = titleFromText(text)
	}
	return n
}

var (
	subtitleIndex = regexp.MustCompile(`^\d+$`)
	subtitleTime  = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}[,.]\d{3} --> `)
	subtitleTag   = regexp.MustCompile(`<[^>]+>`)
)

// stripSubtitles drops SRT/VTT cue numbers and timestamps, keeping the text.
func stripSubtitles(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "", t == "WEBVTT", subtitleIndex.MatchString(t), subtitleTime.MatchString(t):
			continue
		}
		out = append(out, subtitleTag.ReplaceAllString(t, ""))
	}
	return strings.Join(out, " ")
}

// titleFromText makes a short title from the first words of a note.
func titleFromText(text string) string {
	line := strings.TrimLeft(firstLine(strings.TrimSpace(text)), "#-*[] ")
	if i := strings.IndexAny(line, ".!?"); i > 0 {
		line = line[:i]
	}
	words := strings.Fields(line)
	if len(words) > 8 {
		words = words[:8]
	}
	t := strings.TrimRight(strings.Join(words, " "), ".,;:!?")
	if t == "" {
		return "Voice note"
	}
	return t
}
