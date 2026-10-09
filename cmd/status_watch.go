package cmd

import (
	"context"
	"io"
	"strings"
	"time"
)

// Watch intervals: the default, and the shortest accepted, which keeps a
// watch from re-reading every store many times a second.
const (
	defaultWatchInterval = 2 * time.Second
	minWatchInterval     = 200 * time.Millisecond
)

// Terminal control sequences for a watch: the alternate screen keeps the
// user's scrollback untouched, and the cursor is hidden while frames redraw.
const (
	enterAltScreen = "\x1b[?1049h\x1b[?25l"
	leaveAltScreen = "\x1b[?25h\x1b[?1049l"
	cursorHome     = "\x1b[H"
	clearLineEnd   = "\x1b[K"
	clearBelow     = "\x1b[J"
)

// watchStatus draws frame() on w, then redraws it every interval until ctx
// is done. Each frame overwrites the last in place, line by line, rather than
// clearing the screen first, so a refresh does not flicker.
func watchStatus(ctx context.Context, w io.Writer, interval time.Duration, frame func() string) error {
	if _, err := io.WriteString(w, enterAltScreen); err != nil {
		return err
	}
	defer io.WriteString(w, leaveAltScreen) //nolint:errcheck // best effort on the way out

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := io.WriteString(w, drawFrame(frame())); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// drawFrame is the bytes that replace the screen with text: home the cursor,
// write each line clearing what an older, longer line left behind, then
// clear everything below.
func drawFrame(text string) string {
	var b strings.Builder
	b.WriteString(cursorHome)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		b.WriteString(line + clearLineEnd)
		if i < len(lines)-1 {
			b.WriteString("\r\n")
		}
	}
	b.WriteString(clearBelow)
	return b.String()
}
