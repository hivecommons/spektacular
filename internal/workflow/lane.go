package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LaneDir is the folder, under the project's .spektacular directory, holding
// the state and working notes of orchestrated workflows. Each orchestrated
// plan or implement run is a lane: a state file and a notes file of its own,
// keyed by kind and spec name, so several can be in progress at once without
// touching the shared state.json.
const LaneDir = "workflows"

// LaneStatePath is the state file of the kind workflow for name, under
// dataDir (the project's .spektacular directory). It has exactly the format
// of state.json.
func LaneStatePath(dataDir, kind, name string) string {
	return filepath.Join(dataDir, LaneDir, kind+"-"+name+".json")
}

// LaneNotesPath is the working-notes file of the kind workflow for name: the
// lane's counterpart of the shared working-context.md.
func LaneNotesPath(dataDir, kind, name string) string {
	return filepath.Join(dataDir, LaneDir, kind+"-"+name+".md")
}

// LaneStateRel is LaneStatePath relative to the project root, the form
// messages print.
func LaneStateRel(kind, name string) string {
	return filepath.ToSlash(filepath.Join(".spektacular", LaneDir, kind+"-"+name+".json"))
}

// LaneNotesRel is LaneNotesPath relative to the project root, the form step
// instructions print.
func LaneNotesRel(kind, name string) string {
	return filepath.ToSlash(filepath.Join(".spektacular", LaneDir, kind+"-"+name+".md"))
}

// ReadLane loads the kind lane for name. It returns (nil, nil) when the lane
// has no state file.
func ReadLane(dataDir, kind, name string) (*State, error) {
	s, err := loadState(LaneStatePath(dataDir, kind, name))
	if err != nil {
		if _, statErr := os.Stat(LaneStatePath(dataDir, kind, name)); os.IsNotExist(statErr) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// LaneNames lists the spec names that have a kind lane under dataDir, sorted.
// A missing lane folder is no lanes.
func LaneNames(dataDir, kind string) []string {
	matches, _ := filepath.Glob(filepath.Join(dataDir, LaneDir, kind+"-*.json"))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		base := strings.TrimSuffix(filepath.Base(m), ".json")
		names = append(names, strings.TrimPrefix(base, kind+"-"))
	}
	sort.Strings(names)
	return names
}
