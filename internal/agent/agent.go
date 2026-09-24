// Package agent defines the Agent interface and per-agent registry used by
// `spektacular init` to install workflow artefacts for a chosen AI coding
// agent (Claude, Bob, Codex, …).
package agent

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
)

// Agent is implemented by each supported AI coding agent integration.
// Name returns the canonical identifier used on the CLI and persisted to
// config. Install writes all skills (and commands, if supported) into
// projectPath, emitting one human-readable line per artefact to out.
type Agent interface {
	Name() string
	Install(projectPath string, cfg config.Config, out io.Writer) error
}

// ErrUnknownAgent is returned (wrapped) by Lookup when the requested agent
// name is not registered.
var ErrUnknownAgent = errors.New("unknown agent")

var registry = map[string]Agent{}

// register adds a to the package registry. Per-agent files call this from an
// init() function.
func register(a Agent) {
	registry[a.Name()] = a
}

// Lookup returns the Agent registered under name. If no such agent exists the
// returned error wraps ErrUnknownAgent and lists the supported agent names.
func Lookup(name string) (Agent, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%w %q: must be one of %s", ErrUnknownAgent, name, strings.Join(Supported(), ", "))
	}
	return a, nil
}

// Supported returns the sorted list of registered agent names.
func Supported() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
