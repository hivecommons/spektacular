package workflow

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
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
	Kind string `json:"kind,omitempty"` // "spec" | "plan" | "implement"
	// RunID identifies one run of the workflow: a 32-lowercase-hex
	// OpenTelemetry trace id minted when fresh state is built, kept across
	// interruption and resume, and new on every restart. It is the only
	// holder of the run identifier; the command log links records by it.
	RunID string `json:"run_id,omitempty"`
	// StepSpan is the open span of the current step: the workflow opens one
	// on every step it enters and closes it when it leaves. TaskSpan is the
	// open span of the plan task an implement run is working on. Both are
	// absent in state written before spans existed.
	StepSpan       *OpenSpan      `json:"step_span,omitempty"`
	TaskSpan       *OpenSpan      `json:"task_span,omitempty"`
	CurrentStep    string         `json:"current_step"`
	CompletedSteps []string       `json:"completed_steps"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Data           map[string]any `json:"data"`
}

// OpenSpan is a step or task span the workflow has opened and not yet
// closed, kept in state so the next move can close it.
type OpenSpan struct {
	// ID is the span id: 16 lowercase hex characters.
	ID string `json:"id"`
	// Name is the step name, or the task's title.
	Name string `json:"name"`
	// Task is a task span's task id (or phase label).
	Task string `json:"task,omitempty"`
	// Parent is the task span a step span inside a task nests under.
	Parent string `json:"parent,omitempty"`
	// Step is the step that opened a task span.
	Step  string    `json:"step,omitempty"`
	Start time.Time `json:"start"`
}

// RunRef identifies the run the state belongs to, for the span sink.
func (s *State) RunRef() RunRef {
	name, _ := s.Data["name"].(string)
	return RunRef{RunID: s.RunID, Kind: s.Kind, Name: name, StartedAt: s.CreatedAt}
}

// InProgress reports whether the workflow has started but not yet finished.
// It is the single definition of "resumable" used across the resume feature.
func (s *State) InProgress() bool {
	return s.CurrentStep != "" && s.CurrentStep != "finished"
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

// NewRunID mints a run identifier in the OpenTelemetry trace id format: 16
// random bytes as 32 lowercase hex characters, never all zeros. It mirrors
// tracelog.NewTraceID, duplicated so the workflow package stays independent
// of the logger.
func NewRunID() string {
	var b [16]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			// crypto/rand does not fail on supported platforms; fall back
			// to the clock so a run id is always produced.
			now := time.Now().UnixNano()
			for i := range 8 {
				b[i] = byte(now >> (8 * i))
			}
			b[15] |= 1
		}
		for _, c := range b {
			if c != 0 {
				return hex.EncodeToString(b[:])
			}
		}
	}
}

// NewSpanID mints a span id in the OpenTelemetry format: 8 random bytes as 16
// lowercase hex characters, never all zeros. Like NewRunID it mirrors the
// tracelog package's minting, so the workflow stays independent of the
// logger.
func NewSpanID() string {
	var b [8]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			now := time.Now().UnixNano()
			for i := range 8 {
				b[i] = byte(now >> (8 * i))
			}
			b[7] |= 1
		}
		for _, c := range b {
			if c != 0 {
				return hex.EncodeToString(b[:])
			}
		}
	}
}

// EnsureRunID gives the in-progress workflow state at path a run id when it
// has none, as state written before run ids existed has not, and returns the
// state's run id. A missing file, or a state that is not in progress, is
// left untouched and yields "". A state that already has an id is returned
// as is. The file is edited as a generic JSON object, so every other field,
// including keys this build does not know, is kept, and it is written
// through a temporary file and a rename so a crash cannot truncate it.
func EnsureRunID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading state file: %w", err)
	}

	// UseNumber keeps every number exactly as written.
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return "", fmt.Errorf("parsing state file: %w", err)
	}
	step, _ := m["current_step"].(string)
	if step == "" || step == "finished" {
		return "", nil
	}
	if id, ok := m["run_id"].(string); ok && id != "" {
		return id, nil
	}

	id := NewRunID()
	m["run_id"] = id
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("writing state file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("writing state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("writing state file: %w", err)
	}
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpName, info.Mode().Perm())
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("writing state file: %w", err)
	}
	return id, nil
}
