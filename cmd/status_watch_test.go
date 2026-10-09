package cmd

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A frame overwrites the last in place: the cursor goes home, every line
// clears what a longer line before it left, and everything below is cleared.
func TestDrawFrame(t *testing.T) {
	require.Equal(t,
		cursorHome+"one"+clearLineEnd+"\r\n"+"two"+clearLineEnd+clearBelow,
		drawFrame("one\ntwo\n"))
}

// A watch enters the alternate screen, draws a frame at once and again on
// every tick, and restores the screen when it is stopped.
func TestWatchStatus_RedrawsUntilStopped(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	ctx, cancel := context.WithCancel(context.Background())
	frame := func() string {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 3 {
			cancel()
		}
		return "frame"
	}

	var out bytes.Buffer
	require.NoError(t, watchStatus(ctx, &out, time.Millisecond, frame))
	require.Equal(t, 3, calls)
	got := out.String()
	require.True(t, strings.HasPrefix(got, enterAltScreen))
	require.True(t, strings.HasSuffix(got, leaveAltScreen))
	require.Equal(t, 3, strings.Count(got, cursorHome+"frame"))
}

func TestStatus_WatchRefusals(t *testing.T) {
	stProject(t)

	stdout, _, code := runRootCmd(t, "status", "--watch", "--format", "json")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "status_watch_format_unsupported", er.Code)
	require.Contains(t, er.NextAction, "--watch")

	stdout, _, code = runRootCmd(t, "status", "--watch", "--interval", "10ms")
	require.Equal(t, 1, code)
	er = decodeError(t, stdout)
	require.Equal(t, "status_interval_invalid", er.Code)
	require.Contains(t, er.NextAction, "200ms")
}
