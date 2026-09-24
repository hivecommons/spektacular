package repo

import (
	"os"
	"strings"

	repodomain "github.com/hivecommons/spektacular/internal/repo"
)

// writable reports whether the guided add could create its folder inside dir.
//
// It answers by actually creating and removing an entry rather than by
// inspecting permission bits, which do not answer the question reliably
// across the platforms in play. The probe is transient and leaves nothing
// behind, and it is the only thing the flow touches in the target repo
// before the user has confirmed.
func writable(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	probe, err := os.MkdirTemp(dir, ".spektacular-probe-")
	if err != nil {
		return false
	}
	_ = os.RemoveAll(probe)
	return true
}

// evidenceVars renders the examination as template variables.
//
// The result is deliberately a lowercase-keyed map rather than the Evidence
// struct: workflow data is persisted as JSON and read back as a map, so
// caching the struct would make an instruction render one way in the session
// that gathered the evidence and another way after a resume. Normalising here
// means both paths see the identical shape.
func evidenceVars(ev repodomain.Evidence) map[string]any {
	return map[string]any{
		"readable": ev.Readable,
		"readme":   ev.Readme,
		"manifest": ev.Manifest,
		"identity": ev.Identity,
		"summary":  ev.Summary,
		// Both the list and a ready-joined line: a mustache section over a
		// list repeats everything inside it, so a template that wants one
		// labelled line needs the joining done here rather than repeating
		// the label once per entry.
		"top_level":      ev.TopLevel,
		"languages":      ev.Languages,
		"top_level_line": strings.Join(ev.TopLevel, ", "),
		"languages_line": strings.Join(ev.Languages, ", "),
	}
}
