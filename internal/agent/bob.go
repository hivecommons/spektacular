package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
)

type bobAgent struct{}

func (bobAgent) Name() string { return "bob" }

func (bobAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".bob/skills", cfg, out); err != nil {
		return err
	}
	if err := installCommandWrappers(projectPath, ".bob/commands", bobCommandFilename, cfg, out); err != nil {
		return err
	}
	if err := installRepoSourcesSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installStoreAccessSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installMemoryContextSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installKnowledgeTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installSpecTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installDesignTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installDraftPresentationSection(projectPath, cfg, out); err != nil {
		return err
	}
	return installHistoricalArtifactsSection(projectPath, cfg, out)
}

func bobCommandFilename(skillName string) string {
	return skillName + ".md"
}

func init() {
	register(bobAgent{})
}

// Bob Shell stops every task, sub-agents included, once it has run
// session.maxTurns turns (100 unless the user changes it), without telling
// the model, and hands the parent whatever the sub-agent said last. An epic
// run's children routinely need more than that, so they stall mid-step and
// have to be resumed. The limit lives only in the user's own Bob settings:
// Bob reads no per-project session settings.
const (
	// BobDefaultMaxTurns is Bob Shell's built-in session.maxTurns.
	BobDefaultMaxTurns = 100
	bobSettingsDir     = ".bob/settings"
	bobSettingsFile    = "settings.json"
)

// BobSettingsPath is the user's Bob Shell settings file under home.
func BobSettingsPath(home string) string {
	return filepath.Join(home, bobSettingsDir, bobSettingsFile)
}

// BobMaxTurns is the session.maxTurns Bob applies for the user whose home
// directory is home: the value in their settings file, or Bob's default when
// the file or the key is absent. 0 means no limit.
func BobMaxTurns(home string) (int, error) {
	settings, err := readBobSettings(BobSettingsPath(home))
	if err != nil {
		return 0, err
	}
	session, _ := settings["session"].(map[string]any)
	switch v := session["maxTurns"].(type) {
	case float64:
		return int(v), nil
	case nil:
		return BobDefaultMaxTurns, nil
	default:
		return 0, fmt.Errorf("%s: session.maxTurns is %v, not a number", BobSettingsPath(home), v)
	}
}

// RaiseBobTurnLimit sets session.maxTurns to 0, no limit, in the user's Bob
// settings, keeping every other setting as it was. A settings file that is
// not a JSON object is refused rather than overwritten.
func RaiseBobTurnLimit(home string) error {
	path := BobSettingsPath(home)
	settings, err := readBobSettings(path)
	if err != nil {
		return err
	}
	session, ok := settings["session"].(map[string]any)
	if !ok {
		if settings["session"] != nil {
			return fmt.Errorf("%s: session is not an object; set session.maxTurns to 0 yourself", path)
		}
		session = map[string]any{}
	}
	session["maxTurns"] = 0
	settings["session"] = session

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), bobSettingsFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readBobSettings decodes the settings file at path, an empty map when it
// does not exist.
func readBobSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	return settings, nil
}
