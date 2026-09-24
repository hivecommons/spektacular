package agent

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/config"
)

const (
	historicalArtifactsTemplatePath = "agents/historical-artifacts.md"
	historicalArtifactsHeading      = "## Historical Artifacts: Specs and Plans as Archaeology"
)

// installHistoricalArtifactsSection writes (or updates in place) the managed
// "Historical Artifacts: Specs and Plans as Archaeology" section in
// <projectPath>/AGENTS.md, rendering the embedded template against
// cfg.Command. Idempotent: re-running for the same projectPath leaves a single
// section and does not duplicate.
func installHistoricalArtifactsSection(projectPath string, cfg config.Config, out io.Writer) error {
	tmplBytes, err := fs.ReadFile(sourceFS, historicalArtifactsTemplatePath)
	if err != nil {
		return fmt.Errorf("reading embedded template %s: %w", historicalArtifactsTemplatePath, err)
	}

	rendered, err := mustache.Render(string(tmplBytes), map[string]string{"command": cfg.Command})
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", historicalArtifactsTemplatePath, err)
	}
	if !strings.HasSuffix(rendered, "\n") {
		rendered += "\n"
	}

	agentsPath := filepath.Join(projectPath, "AGENTS.md")
	existing, err := os.ReadFile(agentsPath)
	switch {
	case os.IsNotExist(err):
		return writeAGENTSAtomic(agentsPath, []byte(rendered), out, "created")
	case err != nil:
		return fmt.Errorf("reading %s: %w", agentsPath, err)
	}

	startIdx, endIdx, found := locateHistoricalArtifactsSection(existing)
	if !found {
		body := appendHistoricalArtifactsSection(existing, rendered)
		return writeAGENTSAtomic(agentsPath, body, out, "appended Historical Artifacts section to")
	}

	body := replaceHistoricalArtifactsSection(existing, startIdx, endIdx, rendered)
	return writeAGENTSAtomic(agentsPath, body, out, "updated Historical Artifacts section in")
}

// locateHistoricalArtifactsSection returns the byte range [startIdx, endIdx)
// covering the managed section in body. The range starts at the first byte of
// the "## Historical Artifacts: Specs and Plans as Archaeology" heading line
// and ends at the first byte of the next `# ` or `## ` heading line (or
// len(body) if there is none). Returns found=false if the heading is absent.
func locateHistoricalArtifactsSection(body []byte) (startIdx, endIdx int, found bool) {
	lines := strings.SplitAfter(string(body), "\n")
	offset := 0
	start := -1
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\n\r \t")
		switch {
		case start == -1 && trimmed == historicalArtifactsHeading:
			start = offset
		case start != -1 && (strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ")):
			return start, offset, true
		}
		offset += len(line)
	}
	if start == -1 {
		return 0, 0, false
	}
	return start, len(body), true
}

// appendHistoricalArtifactsSection appends the rendered section to body,
// ensuring exactly one blank line separates any prior content from the
// section and that no trailing-whitespace accumulation occurs across repeated
// runs.
func appendHistoricalArtifactsSection(body []byte, rendered string) []byte {
	trimmed := bytes.TrimRight(body, "\n\r \t")
	if len(trimmed) == 0 {
		return []byte(rendered)
	}
	var buf bytes.Buffer
	buf.Write(trimmed)
	buf.WriteString("\n\n")
	buf.WriteString(rendered)
	return buf.Bytes()
}

// replaceHistoricalArtifactsSection replaces body[startIdx:endIdx] with
// rendered, preserving exactly one blank line of separation between the
// section and any non-blank content immediately before or after it.
func replaceHistoricalArtifactsSection(body []byte, startIdx, endIdx int, rendered string) []byte {
	prefixTrimmed := bytes.TrimRight(body[:startIdx], "\n\r \t")
	suffixTrimmed := bytes.TrimLeft(body[endIdx:], "\n\r \t")

	var buf bytes.Buffer
	if len(prefixTrimmed) > 0 {
		buf.Write(prefixTrimmed)
		buf.WriteString("\n\n")
	}
	buf.WriteString(rendered)
	if len(suffixTrimmed) > 0 {
		buf.WriteByte('\n')
		buf.Write(suffixTrimmed)
	}
	return buf.Bytes()
}
