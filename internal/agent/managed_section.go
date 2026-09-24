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

// sectionPlacement says where a managed section goes in an AGENTS.md that
// does not already carry it at that position.
type sectionPlacement int

const (
	// placeAtEnd puts the section after all existing content.
	placeAtEnd sectionPlacement = iota
	// placeAtTop puts the section ahead of every other section, after only
	// the leading preamble (a title and any import lines) that precedes the
	// file's first `## ` heading. A section found lower in the file is
	// hoisted to that position. Use it for a rule that must be read before
	// the agent acts on anything else.
	placeAtTop
)

// installManagedSection writes (or updates in place) a named managed section
// at the end of <projectPath>/AGENTS.md, rendering the embedded template at
// templatePath against cfg.Command. Idempotent: re-running for the same
// projectPath leaves a single section and does not duplicate. actionNoun is
// the human-readable phrase logged to out (e.g. "Memory & Context section").
func installManagedSection(projectPath string, cfg config.Config, out io.Writer, templatePath, heading, actionNoun string) error {
	return installManagedSectionAt(projectPath, cfg, out, templatePath, heading, actionNoun, placeAtEnd)
}

// installManagedSectionAt is installManagedSection with an explicit placement.
func installManagedSectionAt(projectPath string, cfg config.Config, out io.Writer, templatePath, heading, actionNoun string, placement sectionPlacement) error {
	tmplBytes, err := fs.ReadFile(sourceFS, templatePath)
	if err != nil {
		return fmt.Errorf("reading embedded template %s: %w", templatePath, err)
	}

	rendered, err := mustache.Render(string(tmplBytes), map[string]string{"command": cfg.Command})
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", templatePath, err)
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

	startIdx, endIdx, found := locateManagedSection(existing, heading)

	// A top-placed section is always cut and re-inserted at the top, so a
	// section that has drifted down the file (or was written there by an
	// older init) is hoisted rather than updated where it sits.
	if placement == placeAtTop {
		body := existing
		if found {
			body = stripManagedSection(existing, startIdx, endIdx)
		}
		action := "appended " + actionNoun + " to"
		if found {
			action = "updated " + actionNoun + " in"
		}
		return writeAGENTSAtomic(agentsPath, prependManagedSection(body, rendered), out, action)
	}

	if !found {
		body := appendManagedSection(existing, rendered)
		return writeAGENTSAtomic(agentsPath, body, out, "appended "+actionNoun+" to")
	}

	body := replaceManagedSection(existing, startIdx, endIdx, rendered)
	return writeAGENTSAtomic(agentsPath, body, out, "updated "+actionNoun+" in")
}

// locateManagedSection returns the byte range [startIdx, endIdx) covering the
// managed section identified by heading in body. The range starts at the
// first byte of the heading line and ends at the first byte of the next `# `
// or `## ` heading line (or len(body) if there is none). Returns
// found=false if the heading is absent.
func locateManagedSection(body []byte, heading string) (startIdx, endIdx int, found bool) {
	lines := strings.SplitAfter(string(body), "\n")
	offset := 0
	start := -1
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\n\r \t")
		switch {
		case start == -1 && trimmed == heading:
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

// appendManagedSection appends the rendered section to body, ensuring
// exactly one blank line separates any prior content from the section and
// that no trailing-whitespace accumulation occurs across repeated runs.
func appendManagedSection(body []byte, rendered string) []byte {
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

// prependManagedSection inserts rendered ahead of every other section in
// body: after the leading preamble (a title and any import lines) that
// precedes the first `## ` heading, and at the very start when there is no
// preamble. A body that is all preamble gets the section appended to it,
// matching appendManagedSection.
func prependManagedSection(body []byte, rendered string) []byte {
	preamble, rest := splitLeadingPreamble(body)

	var buf bytes.Buffer
	if len(preamble) > 0 {
		buf.Write(preamble)
		buf.WriteString("\n\n")
	}
	buf.WriteString(rendered)
	if len(rest) > 0 {
		buf.WriteByte('\n')
		buf.Write(rest)
	}
	return buf.Bytes()
}

// splitLeadingPreamble splits body at its first `## ` heading, returning the
// content before it and the rest, each trimmed of the whitespace that
// separated them. A body with no `## ` heading is entirely preamble.
func splitLeadingPreamble(body []byte) (preamble, rest []byte) {
	lines := strings.SplitAfter(string(body), "\n")
	offset := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimRight(line, "\n\r \t"), "## ") {
			return bytes.TrimRight(body[:offset], "\n\r \t"), bytes.TrimLeft(body[offset:], "\n\r \t")
		}
		offset += len(line)
	}
	return bytes.TrimRight(body, "\n\r \t"), nil
}

// stripManagedSection removes body[startIdx:endIdx], leaving exactly one
// blank line between the content that surrounded it.
func stripManagedSection(body []byte, startIdx, endIdx int) []byte {
	prefixTrimmed := bytes.TrimRight(body[:startIdx], "\n\r \t")
	suffixTrimmed := bytes.TrimLeft(body[endIdx:], "\n\r \t")

	var buf bytes.Buffer
	if len(prefixTrimmed) > 0 {
		buf.Write(prefixTrimmed)
		buf.WriteByte('\n')
		if len(suffixTrimmed) > 0 {
			buf.WriteByte('\n')
		}
	}
	buf.Write(suffixTrimmed)
	return buf.Bytes()
}

// replaceManagedSection replaces body[startIdx:endIdx] with rendered,
// preserving exactly one blank line of separation between the section and
// any non-blank content immediately before or after it.
func replaceManagedSection(body []byte, startIdx, endIdx int, rendered string) []byte {
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

// writeAGENTSAtomic writes content to path atomically. Emits one line to out
// describing the action, matching the format used by installWorkflowSkills.
func writeAGENTSAtomic(path string, content []byte, out io.Writer, action string) error {
	if err := writeFileAtomic(path, content); err != nil {
		return err
	}
	if action == "created" {
		fmt.Fprintf(out, "  AGENTS.md: created %s\n", path)
	} else {
		fmt.Fprintf(out, "  AGENTS.md: %s %s\n", action, path)
	}
	return nil
}

// writeFileAtomic writes content to path via a sibling temp file + rename so a
// mid-write failure cannot truncate an existing file.
func writeFileAtomic(path string, content []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
