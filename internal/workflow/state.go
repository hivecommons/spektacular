package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// State is the persisted progress of a workflow.
// Name, ArtifactPath and other domain-specific values live in Data.
type State struct {
	Kind           string         `json:"kind,omitempty"` // "spec" | "plan" | "implement" | "workflow:<name>"
	CurrentStep    string         `json:"current_step"`
	CompletedSteps []string       `json:"completed_steps"`
	TerminalStep   string         `json:"terminal_step,omitempty"`
	TerminalSteps  []string       `json:"terminal_steps,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Data           map[string]any `json:"data"`
}

// InProgress reports whether the workflow has started but not yet finished.
// It is the single definition of "resumable" used across the resume feature.
func (s *State) InProgress() bool {
	if s.CurrentStep == "" || s.CurrentStep == "done" {
		return false
	}
	if len(s.TerminalSteps) > 0 {
		return !slices.Contains(s.TerminalSteps, s.CurrentStep)
	}
	if s.TerminalStep != "" {
		return s.CurrentStep != s.TerminalStep
	}
	return s.CurrentStep != "finished"
}

func (s *State) markCompleted(step string) {
	if !slices.Contains(s.CompletedSteps, step) {
		s.CompletedSteps = append(s.CompletedSteps, step)
	}
}

func loadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &s, nil
}

func saveState(path string, s *State) error {
	s.UpdatedAt = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
