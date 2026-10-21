package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
)

// standingRule describes one rule Spektacular installs for every agent. Each
// rule is installed as two pieces: a short section in the project's AGENTS.md,
// stating the obligation that always applies and when to read further, and a
// guidance file under .spektacular/guidance/ holding the rule's full text,
// which the short section links to and agents open only when the rule
// applies. The table below is the single list of rules: the standing-rule
// installer and the omp rule writer both walk it, so a new rule is one entry
// here plus its two templates.
type standingRule struct {
	// slug names the rule's templates (agents/<slug>.md for the full
	// guidance, agents/short/<slug>.md for the AGENTS.md section), its
	// guidance file (.spektacular/guidance/<slug>.md) and its omp rule file
	// (.omp/rules/spek-<slug>.md).
	slug string
	// heading is the rule's exact AGENTS.md heading. The installer finds an
	// installed section by it, so it must never change.
	heading string
	// placement is where a section that is not yet installed goes.
	placement sectionPlacement
	// noun is the phrase logged for the section, e.g. "Memory & Context section".
	noun string
}

// templatePath is the embedded template of the rule's full guidance.
func (r standingRule) templatePath() string {
	return "agents/" + r.slug + ".md"
}

// shortTemplatePath is the embedded template of the rule's AGENTS.md section.
func (r standingRule) shortTemplatePath() string {
	return "agents/short/" + r.slug + ".md"
}

// guidanceDir is the project-root-relative folder holding every rule's full
// guidance. It is an install surface agents read, not a store.
var guidanceDir = filepath.Join(config.ProjectConfigDirName, "guidance")

// guidancePath is the project-root-relative path of a rule's guidance file,
// with forward slashes, as AGENTS.md links to it.
func guidancePath(slug string) string {
	return filepath.ToSlash(filepath.Join(guidanceDir, slug+".md"))
}

// standingRules lists every rule in install order. Where-the-code-lives goes
// at the top, ahead of everything else, because an agent that reads it after
// opening the wrong tree has read it too late; the rest are appended in turn.
var standingRules = []standingRule{
	{slug: "repo-sources", heading: repoSourcesHeading, placement: placeAtTop, noun: "Where the Code Lives section"},
	{slug: "store-access", heading: storeAccessHeading, placement: placeAtEnd, noun: "Spektacular file access section"},
	{slug: "memory-context", heading: memoryContextHeading, placement: placeAtEnd, noun: "Memory & Context section"},
	{slug: "knowledge-trigger", heading: knowledgeTriggerHeading, placement: placeAtEnd, noun: "Knowledge-Worthy Discovery Recognition section"},
	{slug: "spec-trigger", heading: specTriggerHeading, placement: placeAtEnd, noun: "Spec-Worthy Discussion Recognition section"},
	{slug: "design-trigger", heading: designTriggerHeading, placement: placeAtEnd, noun: "Design-Worthy Detail Recognition section"},
	{slug: "draft-presentation", heading: draftPresentationHeading, placement: placeAtEnd, noun: "Presenting Drafts and Confirmations section"},
	{slug: "historical-artifacts", heading: historicalArtifactsHeading, placement: placeAtEnd, noun: "Historical Artifacts section"},
}

// installStandingRules installs every standing rule, in table order: its
// guidance file under <projectPath>/.spektacular/guidance/ and its short
// section in <projectPath>/AGENTS.md. Every agent calls it, so each gets the
// same rules in the same order. Both init and migrate reach it, so it also
// converts a project holding the full sections (replaced in place by heading)
// and refreshes outdated guidance. Idempotent: guidance files are overwritten
// wholesale and each section is replaced in place.
func installStandingRules(projectPath string, cfg config.Config, out io.Writer) error {
	for _, rule := range standingRules {
		if err := installRule(projectPath, cfg, out, rule); err != nil {
			return err
		}
	}
	return nil
}

// installStandingRule installs the one standing rule named by slug.
func installStandingRule(projectPath string, cfg config.Config, out io.Writer, slug string) error {
	for _, rule := range standingRules {
		if rule.slug == slug {
			return installRule(projectPath, cfg, out, rule)
		}
	}
	return fmt.Errorf("no standing rule %q: add it to standingRules in internal/agent/standing_rules.go", slug)
}

// installRule writes the rule's guidance file first, so an interrupted
// install never leaves AGENTS.md linking to a file that is not there, then
// its short section.
func installRule(projectPath string, cfg config.Config, out io.Writer, rule standingRule) error {
	if err := installGuidance(projectPath, cfg, out, rule); err != nil {
		return err
	}
	return installManagedSectionWith(projectPath, out, rule.shortTemplatePath(), rule.heading, rule.noun, rule.placement, map[string]string{
		"command":       cfg.Command,
		"guidance_path": guidancePath(rule.slug),
	})
}

// installGuidance renders the rule's full guidance into its file under
// .spektacular/guidance/, replacing whatever is there.
func installGuidance(projectPath string, cfg config.Config, out io.Writer, rule standingRule) error {
	rendered, err := renderTemplate(rule.templatePath(), map[string]string{"command": cfg.Command})
	if err != nil {
		return err
	}
	dir := filepath.Join(projectPath, guidanceDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating guidance folder %s: %w; make sure the project directory is writable, then re-run `%s init`", dir, err, cfg.Command)
	}
	path := filepath.Join(projectPath, filepath.FromSlash(guidancePath(rule.slug)))
	if err := writeFileAtomic(path, []byte(rendered)); err != nil {
		return fmt.Errorf("writing guidance file: %w; make sure %s is writable, then re-run `%s init`", err, dir, cfg.Command)
	}
	fmt.Fprintf(out, "  Guidance: %s\n", path)
	return nil
}
