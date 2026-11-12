package voice

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// mcpProtocolVersion is the MCP revision spoken as a stdio client.
const mcpProtocolVersion = "2024-11-05"

// mcpMaxLineBytes caps one JSON-RPC line read from an MCP server; a long
// transcript arrives as one line.
const mcpMaxLineBytes = 32 << 20

// MCPClient is one connected MCP session: Call invokes a tool and returns
// the concatenated text content of its result.
type MCPClient interface {
	Call(ctx context.Context, tool string, args map[string]any) (string, error)
	Close() error
}

// Dialer opens an MCP session.
type Dialer func(ctx context.Context) (MCPClient, error)

// mcpSession speaks newline-delimited JSON-RPC 2.0 over a reader/writer
// pair, the same shape hive's knowledge connectors use. Requests are
// serialised; notifications are dropped.
type mcpSession struct {
	w      io.Writer
	lines  chan []byte
	done   chan struct{}
	mu     sync.Mutex
	nextID int64
	stderr *lockedBuffer
	closer func() error
}

// lockedBuffer is a goroutine-safe buffer for a child's stderr.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) tail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := strings.TrimSpace(b.buf.String())
	if len(s) > 2000 {
		s = s[len(s)-2000:]
	}
	return s
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewMCPSession wires a session over r/w and performs the MCP handshake.
// closer, when non-nil, is called by Close.
func NewMCPSession(ctx context.Context, r io.Reader, w io.Writer, closer func() error) (MCPClient, error) {
	return newSession(ctx, r, w, nil, closer)
}

func newSession(ctx context.Context, r io.Reader, w io.Writer, stderr *lockedBuffer, closer func() error) (MCPClient, error) {
	s := &mcpSession{w: w, lines: make(chan []byte, 64), done: make(chan struct{}), stderr: stderr, closer: closer}
	go s.readLoop(r)
	init := map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "spektacular-voice", "version": "1"},
	}
	if _, err := s.request(ctx, "initialize", init); err != nil {
		_ = s.Close()
		return nil, err
	}
	_ = s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return s, nil
}

func (s *mcpSession) readLoop(r io.Reader) {
	defer close(s.done)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), mcpMaxLineBytes)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			s.lines <- []byte(line)
		}
	}
}

func (s *mcpSession) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.w.Write(append(data, '\n'))
	return err
}

// request sends one request and waits for its reply.
func (s *mcpSession) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	if err := s.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, fmt.Errorf("mcp: sending %s: %w", method, err)
	}
	want := fmt.Sprintf("%d", id)
	for {
		select {
		case <-ctx.Done():
			return nil, s.withStderr(fmt.Errorf("mcp: %s: %w", method, ctx.Err()))
		case line := <-s.lines:
			if res, ok, err := s.match(line, want, method); ok {
				return res, err
			}
		case <-s.done:
			// Drain replies that arrived just before the server hung up.
			for {
				select {
				case line := <-s.lines:
					if res, ok, err := s.match(line, want, method); ok {
						return res, err
					}
				default:
					return nil, s.withStderr(fmt.Errorf("mcp: %s: server closed the connection", method))
				}
			}
		}
	}
}

// match decodes one line; ok is true when it is the reply to request id want.
func (s *mcpSession) match(line []byte, want, method string) (json.RawMessage, bool, error) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil || len(msg.ID) == 0 || msg.Method != "" {
		return nil, false, nil
	}
	if strings.Trim(string(msg.ID), `"`) != want {
		return nil, false, nil
	}
	if msg.Error != nil {
		return nil, true, fmt.Errorf("mcp: %s: %s (code %d)", method, msg.Error.Message, msg.Error.Code)
	}
	return msg.Result, true, nil
}

func (s *mcpSession) withStderr(err error) error {
	if s.stderr == nil {
		return err
	}
	if tail := s.stderr.tail(); tail != "" {
		return fmt.Errorf("%w (stderr: %s)", err, tail)
	}
	return err
}

// Call implements MCPClient: tools/call with the text content joined.
func (s *mcpSession) Call(ctx context.Context, tool string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := s.request(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return "", err
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("mcp: %s: decoding result: %w", tool, err)
	}
	var parts []string
	for _, c := range res.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if res.IsError {
		if text == "" {
			text = "tool reported an error"
		}
		return text, fmt.Errorf("mcp: %s: %s", tool, firstLine(text))
	}
	return text, nil
}

// Close implements MCPClient.
func (s *mcpSession) Close() error {
	if s.closer != nil {
		return s.closer()
	}
	return nil
}

// stdioDialer starts command as a child process speaking MCP on its
// stdin/stdout.
func stdioDialer(command string) Dialer {
	return func(ctx context.Context) (MCPClient, error) {
		argv := strings.Fields(command)
		if len(argv) == 0 {
			return nil, errors.New("mcp: command is empty")
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = os.Environ()
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		stderr := &lockedBuffer{}
		cmd.Stderr = stderr
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("mcp: starting %q: %w", argv[0], err)
		}
		closer := sync.OnceValue(func() error {
			_ = stdin.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return cmd.Wait()
		})
		return newSession(ctx, stdout, stdin, stderr, closer)
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
