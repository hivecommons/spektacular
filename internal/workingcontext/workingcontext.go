// Package workingcontext manages the agent's working context file,
// .spektacular/working-context.md.
//
// The file belongs entirely to the agent: it records the cross-cutting
// learnings and user answers a session leaves behind for a resumed session to
// read back. The CLI's only involvement is clearing it at the start of a
// fresh workflow.
//
// A file under the previous name, .spektacular/context.md, is deliberately
// ignored: it is never read, migrated, removed or reported as an error.
//
// Where each repo's code lives is deliberately not written here. It is
// reported live by `repo list`, and the "Where the Code Lives" section
// installed into AGENTS.md directs every agent to that command before it
// touches code — so the roster cannot go stale against the registry, and an
// agent gets it whether or not a workflow has ever run in the project.
package workingcontext

import (
	"fmt"
	"os"
	"path/filepath"
)

// RelPath is the working-context file's location relative to the project
// root.
const RelPath = ".spektacular/working-context.md"

// Path returns the working-context file's absolute path under root.
func Path(root string) string {
	return filepath.Join(root, filepath.FromSlash(RelPath))
}

// Reset empties the file at path, discarding the previous session's content
// so a fresh workflow starts with no carried-over context. A missing file is
// created empty; the directory is created if needed.
func Reset(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
