package migrate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
)

// scanMetadata reads project files to infer appropriate repo.yaml metadata
// values. Uses best-effort heuristics with graceful fallback to defaults.
// Always returns a valid RepoConfig, never fails.
func scanMetadata(projectRoot string) config.RepoConfig {
	cfg := config.NewDefaultRepoConfig()

	// Try to extract description from README.md
	readmePath := filepath.Join(projectRoot, "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		content := string(data)
		// Try to find first H1 title
		if idx := strings.Index(content, "# "); idx != -1 {
			end := strings.Index(content[idx:], "\n")
			if end != -1 {
				cfg.Description = strings.TrimSpace(content[idx+2 : idx+end])
			}
		}
		// If no H1, try first paragraph
		if cfg.Description == "" {
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					cfg.Description = line
					break
				}
			}
		}
	}

	// Detect Go projects via go.mod
	goModPath := filepath.Join(projectRoot, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		cfg.Role = "application"
		cfg.Tags = append(cfg.Tags, "go")
	}

	// Detect Node.js projects via package.json
	packageJSONPath := filepath.Join(projectRoot, "package.json")
	if _, err := os.Stat(packageJSONPath); err == nil {
		if cfg.Role == "" {
			cfg.Role = "application"
		}
		cfg.Tags = append(cfg.Tags, "nodejs")
	}

	// Ensure defaults if nothing was detected
	if cfg.Description == "" {
		cfg.Description = "A Spektacular project"
	}
	if cfg.Role == "" {
		cfg.Role = "application"
	}
	if len(cfg.Tags) == 0 {
		cfg.Tags = []string{"general"}
	}

	return cfg
}
