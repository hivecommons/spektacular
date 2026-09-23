package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file asserts the repo-root documentation shipped with Phase 4.2:
// README.md's project/repo configuration split and CHANGELOG.md's
// breaking-change entry. These two files ARE the artifacts under test, so
// they are read read-only from the repository root (relative to this package
// directory) rather than rendered into a temp dir.

// readRepoRootFile reads a file at the repository root, relative to this
// package's directory. Content is treated strictly read-only.
func readRepoRootFile(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", name))
	require.NoError(t, err, "%s must exist at the repository root", name)
	return string(body)
}

// Criterion 5: the README describes the project-level config.yaml vs per-repo
// repo.yaml split, the repos registry, the ignore file, and the
// breaking-change re-initialization notice.
func TestREADMEDescribesProjectRepoConfigurationSplit(t *testing.T) {
	readme := readRepoRootFile(t, "README.md")

	require.Contains(t, readme, "repo.yaml",
		"README must describe the per-repo repo.yaml configuration")
	require.Contains(t, readme, "repos:",
		"README must show the project config's repos registry")
	require.Contains(t, readme, ".spektacular_ignore",
		"README must document the .spektacular_ignore exclusion file")
	require.Contains(t, readme, "Breaking change",
		"README must carry the breaking-change notice")
	require.Contains(t, readme, "re-initialize",
		"README must tell existing setups to re-initialize")
}

// Criterion 5: CHANGELOG.md records the 000039 project-level-capabilities
// entry's breaking change. The implement workflow no longer writes to this
// file (plan 000046 removed the root release-note step), but entries were
// prepended above 000039 before that, so this only checks that the 000039
// section itself still carries the notice, not that it remains the file's
// top entry.
func TestChangelogTopSectionRecordsBreakingChange(t *testing.T) {
	changelog := strings.TrimSpace(readRepoRootFile(t, "CHANGELOG.md"))

	const heading = "## 000039_project-level-capabilities"
	start := strings.Index(changelog, heading)
	require.GreaterOrEqual(t, start, 0, "CHANGELOG.md must retain the %s entry", heading)

	// The section is everything between its heading and the next `## ` heading.
	rest := changelog[start+len(heading):]
	next := strings.Index(rest, "\n## ")
	require.Greater(t, next, 0, "CHANGELOG.md must retain the earlier entries below the 000039 section")
	section := rest[:next]

	require.Contains(t, section, "Breaking change",
		"the 000039 changelog section must record the breaking change")
	require.Contains(t, section, "re-initialize",
		"the 000039 changelog section must direct existing setups to re-initialize")
}

// ---------------------------------------------------------------------------
// Phase 3.5: the command repository's own documentation of the knowledge
// addressing model (README.md, docs/knowledge-base.md, CHANGELOG.md).
// ---------------------------------------------------------------------------

// knowledgeBaseDoc is the in-repo knowledge base document, read relative to
// this package's directory like the other repo-root documents above.
const knowledgeBaseDoc = "docs/knowledge-base.md"

// docSection returns the body of the markdown section introduced by heading,
// ending at the next heading of the same or a higher level. Fenced code blocks
// are tracked so a `#` comment inside YAML is never mistaken for a heading.
func docSection(t *testing.T, doc, heading string) string {
	t.Helper()

	start := strings.Index(doc, heading)
	require.GreaterOrEqual(t, start, 0, "document must contain the %q heading", heading)

	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	rest := doc[start+len(heading):]

	var body strings.Builder
	inFence := false
	for _, line := range strings.Split(rest, "\n")[1:] {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "#") {
			if h := len(line) - len(strings.TrimLeft(line, "#")); h <= level {
				break
			}
		}
		body.WriteString(line)
		body.WriteString("\n")
	}

	return body.String()
}

// requireDocLacks fails when doc contains needle (case-insensitively), naming
// the document and quoting only the offending line. These documents are long,
// so a plain require.NotContains would dump the whole file into the failure.
func requireDocLacks(t *testing.T, name, doc, needle, why string) {
	t.Helper()

	lowered := strings.ToLower(needle)
	for i, line := range strings.Split(doc, "\n") {
		if strings.Contains(strings.ToLower(line), lowered) {
			t.Errorf("%s:%d still contains %q — %s\n\t%s",
				name, i+1, needle, why, strings.TrimSpace(line))
			return
		}
	}
}

// fencedBlocks returns the bodies of every ``` fenced code block in body.
func fencedBlocks(body string) []string {
	var blocks []string
	var current strings.Builder
	inFence := false

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inFence {
				blocks = append(blocks, current.String())
				current.Reset()
			}
			inFence = !inFence
			continue
		}
		if inFence {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}

	return blocks
}

// Criterion 1: the README explains the two addressing tiers, how a store gets
// its name, how a write states its destination, and how retrieval is narrowed.
// Each assertion targets a short distinctive phrase rather than a whole
// paragraph, so ordinary copy-editing does not break the test.
func TestREADMEExplainsKnowledgeTiersAndAddressing(t *testing.T) {
	readme := readRepoRootFile(t, "README.md")

	// The two addressing tiers, and that they are a different axis from the
	// per-category retrieval tiers.
	require.Contains(t, readme, "Knowledge lives in one of two tiers",
		"README must state that knowledge lives in one of two tiers")
	require.Contains(t, readme, "retrieval tier",
		"README must keep the per-category retrieval tier distinct from the addressing tier")

	// How a store is named: one store per repo, named by the registry; the
	// project's shared stores each declare their own name.
	require.Contains(t, readme, "contributes exactly one store",
		"README must say each registered repo contributes exactly one store")
	require.Contains(t, readme, "the name the project registered it under",
		"README must say a repo's store is addressed by its registry name")
	require.Contains(t, readme, "knowledge.sources",
		"README must say the project declares its shared stores under knowledge.sources")

	// A write states its destination: the three-part address.
	require.Contains(t, readme, `--data '{"tier"`,
		"README must show the addressed --data payload for read/write")
	require.Contains(t, readme, "Reading and writing name exactly one store",
		"README must say a read or write addresses exactly one store")

	// How retrieval is narrowed.
	require.Contains(t, readme, "`--tier <project|repo|all>`",
		"README must document the --tier narrowing flag and its values")
	require.Contains(t, readme, "`--filter <name>`",
		"README must document the repeatable --filter narrowing flag")
}

// Criterion 2: neither the README nor the in-repo knowledge base document shows
// the superseded configuration shape or an unaddressed command example.
//
// Distinguishing the legitimate `knowledge.sources` from the superseded one:
// a *project* config (`config.yaml`) still declares its shared stores as a
// `knowledge:`/`sources:` list, so the string cannot be banned outright — the
// README's project example and both documents' prose legitimately use it. What
// was removed is the same list inside a *repo* config (`repo.yaml`), which now
// carries a single `provider`/`config` block. The test therefore does not grep
// the whole document: it extracts the section headed for `repo.yaml`, takes the
// fenced YAML examples inside just that section, and asserts no `sources:` key
// appears there — while separately asserting the project section's example
// still has one.
func TestKnowledgeDocsDropSupersededConfigAndCommandShapes(t *testing.T) {
	docs := map[string]string{
		"README.md":      readRepoRootFile(t, "README.md"),
		knowledgeBaseDoc: readRepoRootFile(t, knowledgeBaseDoc),
	}

	for name, doc := range docs {
		requireDocLacks(t, name, doc, "- scope:",
			"the superseded per-source `scope:` config key is gone")
		requireDocLacks(t, name, doc, `"scope":`,
			"a --data payload is keyed on tier and name, not scope")
	}

	// The repo.yaml example declares a single store, not a `sources:` list.
	repoSection := docSection(t, docs["README.md"], "### Repo configuration (`repo.yaml`)")
	repoBlocks := fencedBlocks(repoSection)
	require.NotEmpty(t, repoBlocks, "README must keep a repo.yaml configuration example")
	for _, block := range repoBlocks {
		require.Contains(t, block, "knowledge:",
			"the repo.yaml example must still declare the repo's knowledge store")
		require.NotContains(t, block, "sources:",
			"the repo.yaml example must declare one provider block, not a sources list")
		require.Contains(t, block, "provider: file",
			"the repo.yaml example must declare its store as a provider block")
	}

	// The project config.yaml example still declares named shared stores — the
	// legitimate use of `knowledge.sources` that must survive this check.
	projectSection := docSection(t, docs["README.md"], "### Project configuration (`config.yaml`)")
	projectBlocks := fencedBlocks(projectSection)
	require.NotEmpty(t, projectBlocks, "README must keep a project config.yaml example")
	joinedProject := strings.Join(projectBlocks, "\n")
	require.Contains(t, joinedProject, "sources:",
		"the project config.yaml example must keep its knowledge.sources list")
	require.Contains(t, joinedProject, "- name: team",
		"the project config.yaml example must name each shared store with `name:`")

	// Whatever YAML either document shows must use the addressed shape. The
	// knowledge base document carries no config examples today; the loop still
	// guards it should one be added later.
	for name, doc := range docs {
		for _, block := range fencedBlocks(doc) {
			requireDocLacks(t, name, block, "scope:",
				"no example may show a `scope:` key")
		}
	}
}

// Criterion 3: neither document still claims a precedence order between stores,
// and the knowledge base states positively that there is none — so the claim was
// removed rather than merely reworded.
func TestKnowledgeDocsClaimNoPrecedenceBetweenStores(t *testing.T) {
	docs := map[string]string{
		"README.md":      readRepoRootFile(t, "README.md"),
		knowledgeBaseDoc: readRepoRootFile(t, knowledgeBaseDoc),
	}

	precedenceClaims := []string{
		"most-specific",
		"most specific scope",
		"layered precedence",
		"Layered source precedence",
		"overrides a team entry",
	}

	for name, doc := range docs {
		for _, claim := range precedenceClaims {
			requireDocLacks(t, name, doc, claim,
				"the documents must not claim a precedence order between stores")
		}
	}

	// Positively assert the claim was removed rather than merely reworded: the
	// knowledge base says outright that stores do not rank against each other.
	kb := docs[knowledgeBaseDoc]
	require.True(t, strings.Contains(kb, "There is no precedence between stores"),
		"%s must state explicitly that there is no precedence between stores", knowledgeBaseDoc)
	require.True(t, strings.Contains(kb, "## Two tiers, and how a store is addressed"),
		"%s must carry the addressing section that replaced `## Layered source precedence`", knowledgeBaseDoc)
}

// Criterion 4: CHANGELOG.md records the addressing change as breaking and
// names both edits an existing project must make.
//
// The section is located by its heading rather than by position. The entry was
// top of the file when this was written, but a changelog grows: asserting on
// position would make every later release fail a test about an older one.
func TestChangelogRecordsKnowledgeAddressingBreakingChange(t *testing.T) {
	changelog := strings.TrimSpace(readRepoRootFile(t, "CHANGELOG.md"))

	const heading = "## 000047_repo-scoped-knowledge-addressing"
	start := strings.Index(changelog, heading)
	require.GreaterOrEqual(t, start, 0, "CHANGELOG.md must carry the %s entry", heading)

	rest := changelog[start+len(heading):]
	next := strings.Index(rest, "\n## ")
	require.Greater(t, next, 0, "CHANGELOG.md must retain the earlier entries below the %s section", heading)
	section := rest[:next]

	require.Contains(t, section, "**Breaking change**",
		"the %s changelog section must record the change as breaking", heading)
	require.Contains(t, section, "`provider`/`config` block",
		"the changelog must say a repo now declares a single provider/config block")
	require.Contains(t, section, "`scope:` key to `name:`",
		"the changelog must say a project-declared source's scope: key is renamed to name:")
}

// The repository's own markdown documentation must not carry an unaddressed
// `--data '{"scope"` example anywhere: a reader copying one would run a command
// the CLI now rejects.
func TestRepoMarkdownDocsCarryNoUnaddressedDataExample(t *testing.T) {
	paths := []string{filepath.Join("..", "README.md")}

	matches, err := filepath.Glob(filepath.Join("..", "docs", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "the repository must carry markdown documentation under docs/")
	paths = append(paths, matches...)

	for _, path := range paths {
		body, err := os.ReadFile(path)
		require.NoError(t, err)

		require.NotContains(t, string(body), `--data '{"scope"`,
			"%s must not show a --data example keyed on the superseded scope", path)

		for _, line := range strings.Split(string(body), "\n") {
			if strings.Contains(line, "--data") && strings.Contains(line, `"scope"`) {
				t.Errorf("%s: --data example still keyed on scope: %s", path, strings.TrimSpace(line))
			}
		}
	}
}

// Phase 3.2 criterion 3: the README's project configuration example shows the
// auto_commit key with its three values, so a reader setting the file up by
// hand sees what it accepts without leaving the README.
func TestREADMEDocumentsAutoCommitSetting(t *testing.T) {
	readme := readRepoRootFile(t, "README.md")

	require.Contains(t, readme, `auto_commit: "off"`,
		"the README config example must show auto_commit and its default")
	for _, value := range []string{"off", "workflow", "full"} {
		require.Containsf(t, readme, value,
			"the README must name the %q auto_commit value", value)
	}
	require.Contains(t, readme, "off | workflow | full",
		"the README must list the three accepted auto_commit values together")
}
