package agentusage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkReadSession parses a generated session of about 10 MB: 2,000
// responses, each split over two records with a shell call, and a large
// tool result after each.
func BenchmarkReadSession(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "big.jsonl")
	big := strings.Repeat("x", 4000)
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		at := fmt.Sprintf("2026-10-25T10:%02d:%02d.000Z", (i/60)%60, i%60)
		usage := `"usage":{"input_tokens":3,"output_tokens":120,"cache_read_input_tokens":150000,"cache_creation_input_tokens":900}`
		fmt.Fprintf(&sb, `{"type":"assistant","message":{"model":"claude-opus-5-5","id":"msg_%d","content":[{"type":"text","text":"thinking about it"}],%s},"timestamp":"%s"}`+"\n", i, usage, at)
		fmt.Fprintf(&sb, `{"type":"assistant","message":{"model":"claude-opus-5-5","id":"msg_%d","content":[{"type":"tool_use","id":"toolu_%d","name":"Bash","input":{"command":"go test ./..."}}],%s},"timestamp":"%s"}`+"\n", i, i, usage, at)
		fmt.Fprintf(&sb, `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_%d","type":"tool_result","content":"%s"}]},"timestamp":"%s"}`+"\n", i, big, at)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(sb.Len()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := readSession(path); err != nil {
			b.Fatal(err)
		}
	}
}
