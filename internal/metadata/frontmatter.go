package metadata

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Split scans raw for a leading `---\n…\n---\n` frontmatter block. When one is
// present, it returns the parsed Metadata and the remaining body bytes. When
// raw has no leading block, it returns (nil, raw, nil) — a bare artifact is
// not an error. When a `---` opener is present but the block is unterminated
// or the YAML fails to unmarshal, Split returns a wrapped error so callers can
// distinguish malformed frontmatter from a bare artifact.
func Split(raw []byte) (*Metadata, []byte, error) {
	fmYAML, body, ok, err := SplitRaw(raw)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, raw, nil
	}
	var m Metadata
	if err := yaml.Unmarshal(fmYAML, &m); err != nil {
		return nil, nil, fmt.Errorf("malformed frontmatter: %w", err)
	}
	return &m, body, nil
}

// SplitRaw finds raw's leading frontmatter fence without decoding it, for a
// document kind with its own frontmatter type (an epic). It returns the YAML
// between the fences, the body after them (less one separating blank line),
// and ok=false with a nil error when raw has no leading block. An unterminated
// block, or a closing fence not on its own line, is an error.
func SplitRaw(raw []byte) (yamlBytes, body []byte, ok bool, err error) {
	opener := []byte("---\n")
	if !bytes.HasPrefix(raw, opener) {
		return nil, raw, false, nil
	}
	rest := raw[len(opener):]

	// The closing delimiter is a line containing only `---`. Find `\n---`.
	closeIdx := bytes.Index(rest, []byte("\n---"))
	if closeIdx < 0 {
		return nil, nil, false, fmt.Errorf("malformed frontmatter: unterminated block")
	}

	fmYAML := rest[:closeIdx]
	afterClose := rest[closeIdx+len("\n---"):]

	// The closing `---` must be followed by end-of-input or a newline.
	switch {
	case len(afterClose) == 0:
		body = nil
	case afterClose[0] == '\n':
		body = afterClose[1:]
	default:
		return nil, nil, false, fmt.Errorf("malformed frontmatter: closing --- must be on its own line")
	}

	// Drop a single blank line separating frontmatter from body, if present.
	if bytes.HasPrefix(body, []byte("\n")) {
		body = body[1:]
	}
	return fmYAML, body, true, nil
}

// Render writes m as a fenced YAML frontmatter block, a blank line, and then
// body. The output is deterministic — Render(Split(x))'s idempotency is only
// guaranteed for the fields Metadata knows about; unknown YAML keys in the
// original block are not preserved.
func Render(m Metadata, body []byte) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(yamlBytes)
	out.WriteString("---\n\n")
	out.Write(body)
	return out.Bytes(), nil
}
