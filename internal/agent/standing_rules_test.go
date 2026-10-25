package agent

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// standingRuleHeadingsInOrder is the hand-maintained install order of the
// standing rules, by their exact AGENTS.md heading. Where-the-code-lives is
// first because it is placed at the top of the file.
var standingRuleHeadingsInOrder = []string{
	"## Where the Code Lives — Read This First",
	"## Spektacular's Files Are Reached Through Spektacular",
	"## Memory & Context",
	"## Knowledge-Worthy Discovery Recognition",
	"## Spec-Worthy Discussion Recognition",
	"## Design-Worthy Detail Recognition",
	"## Presenting Drafts and Confirmations",
	"## Historical Artifacts: Specs and Plans as Archaeology",
}

// standingRuleFixtureFS holds a tiny pair of templates for each of the eight
// standing rules, so an install's output can be pinned byte for byte by hand:
// the full guidance at agents/<slug>.md and the short AGENTS.md section at
// agents/short/<slug>.md, which links to the guidance file.
func standingRuleFixtureFS() fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, r := range []struct{ slug, heading string }{
		{"repo-sources", repoSourcesHeading},
		{"store-access", storeAccessHeading},
		{"memory-context", memoryContextHeading},
		{"knowledge-trigger", knowledgeTriggerHeading},
		{"spec-trigger", specTriggerHeading},
		{"design-trigger", designTriggerHeading},
		{"draft-presentation", draftPresentationHeading},
		{"historical-artifacts", historicalArtifactsHeading},
	} {
		fsys["agents/"+r.slug+".md"] = &fstest.MapFile{Data: []byte("# " + r.slug + " guidance\n\n" + r.slug + " full {{command}}\n")}
		fsys["agents/short/"+r.slug+".md"] = &fstest.MapFile{Data: []byte(r.heading + "\n\n" + r.slug + " body {{command}}\nsee {{guidance_path}}\n")}
	}
	return fsys
}

// fixtureGuidance is a fixture rule's full guidance rendered for cfg.Command =
// "go run .", as it lands in .spektacular/guidance/<slug>.md.
func fixtureGuidance(slug string) string {
	return "# " + slug + " guidance\n\n" + slug + " full go run .\n"
}

// standingRuleSlugsInOrder is the hand-maintained install order of the
// standing rules, by slug.
var standingRuleSlugsInOrder = []string{
	"repo-sources",
	"store-access",
	"memory-context",
	"knowledge-trigger",
	"spec-trigger",
	"design-trigger",
	"draft-presentation",
	"historical-artifacts",
}

// readGuidanceFile returns the installed guidance file of the rule named by
// slug under projectPath. The path is spelled out by hand rather than taken
// from guidancePath, so a change to where guidance lands fails here.
func readGuidanceFile(t *testing.T, projectPath, slug string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(projectPath, ".spektacular", "guidance", slug+".md"))
	require.NoError(t, err, "guidance file for %q", slug)
	return string(body)
}

// standingRuleGuidancePaths is the hand-maintained project-relative path of
// every standing rule's guidance file, in install order.
var standingRuleGuidancePaths = []string{
	".spektacular/guidance/repo-sources.md",
	".spektacular/guidance/store-access.md",
	".spektacular/guidance/memory-context.md",
	".spektacular/guidance/knowledge-trigger.md",
	".spektacular/guidance/spec-trigger.md",
	".spektacular/guidance/design-trigger.md",
	".spektacular/guidance/draft-presentation.md",
	".spektacular/guidance/historical-artifacts.md",
}

// requireGuidanceFilesInstalled asserts an agent's install wrote all eight
// guidance files.
func requireGuidanceFilesInstalled(t *testing.T, projectPath string) {
	t.Helper()
	for _, rel := range standingRuleGuidancePaths {
		require.FileExists(t, filepath.Join(projectPath, filepath.FromSlash(rel)))
	}
}

// requireFixtureGuidance asserts every fixture rule's guidance file holds its
// rendered full text.
func requireFixtureGuidance(t *testing.T, projectPath string) {
	t.Helper()
	for _, slug := range standingRuleSlugsInOrder {
		require.Equal(t, fixtureGuidance(slug), readGuidanceFile(t, projectPath, slug), "guidance for %q", slug)
	}
}

// standingRulesFixtureSections is the eight fixture short sections rendered
// for cfg.Command = "go run .", in install order, written out by hand.
const standingRulesFixtureSections = "## Where the Code Lives — Read This First\n" +
	"\n" +
	"repo-sources body go run .\n" +
	"see .spektacular/guidance/repo-sources.md\n" +
	"\n" +
	"## Spektacular's Files Are Reached Through Spektacular\n" +
	"\n" +
	"store-access body go run .\n" +
	"see .spektacular/guidance/store-access.md\n" +
	"\n" +
	"## Memory & Context\n" +
	"\n" +
	"memory-context body go run .\n" +
	"see .spektacular/guidance/memory-context.md\n" +
	"\n" +
	"## Knowledge-Worthy Discovery Recognition\n" +
	"\n" +
	"knowledge-trigger body go run .\n" +
	"see .spektacular/guidance/knowledge-trigger.md\n" +
	"\n" +
	"## Spec-Worthy Discussion Recognition\n" +
	"\n" +
	"spec-trigger body go run .\n" +
	"see .spektacular/guidance/spec-trigger.md\n" +
	"\n" +
	"## Design-Worthy Detail Recognition\n" +
	"\n" +
	"design-trigger body go run .\n" +
	"see .spektacular/guidance/design-trigger.md\n" +
	"\n" +
	"## Presenting Drafts and Confirmations\n" +
	"\n" +
	"draft-presentation body go run .\n" +
	"see .spektacular/guidance/draft-presentation.md\n" +
	"\n" +
	"## Historical Artifacts: Specs and Plans as Archaeology\n" +
	"\n" +
	"historical-artifacts body go run .\n" +
	"see .spektacular/guidance/historical-artifacts.md\n"

// The table is the single list of standing rules: exactly these eight, in
// this order, with only where-the-code-lives placed at the top.
func TestStandingRuleTable_ListsEightRulesInInstallOrder(t *testing.T) {
	type entry struct {
		slug      string
		heading   string
		placement sectionPlacement
	}
	want := []entry{
		{"repo-sources", "## Where the Code Lives — Read This First", placeAtTop},
		{"store-access", "## Spektacular's Files Are Reached Through Spektacular", placeAtEnd},
		{"memory-context", "## Memory & Context", placeAtEnd},
		{"knowledge-trigger", "## Knowledge-Worthy Discovery Recognition", placeAtEnd},
		{"spec-trigger", "## Spec-Worthy Discussion Recognition", placeAtEnd},
		{"design-trigger", "## Design-Worthy Detail Recognition", placeAtEnd},
		{"draft-presentation", "## Presenting Drafts and Confirmations", placeAtEnd},
		{"historical-artifacts", "## Historical Artifacts: Specs and Plans as Archaeology", placeAtEnd},
	}

	got := make([]entry, 0, len(standingRules))
	for _, r := range standingRules {
		got = append(got, entry{r.slug, r.heading, r.placement})
	}
	require.Equal(t, want, got)
}

// Each rule renders its guidance from agents/<slug>.md and its AGENTS.md
// section from agents/short/<slug>.md, and both templates ship in the real
// embedded templates.
func TestStandingRuleTable_TemplatesExistInEmbeddedTemplates(t *testing.T) {
	want := map[string]string{
		"repo-sources":         "agents/repo-sources.md",
		"store-access":         "agents/store-access.md",
		"memory-context":       "agents/memory-context.md",
		"knowledge-trigger":    "agents/knowledge-trigger.md",
		"spec-trigger":         "agents/spec-trigger.md",
		"design-trigger":       "agents/design-trigger.md",
		"draft-presentation":   "agents/draft-presentation.md",
		"historical-artifacts": "agents/historical-artifacts.md",
	}
	wantShort := map[string]string{
		"repo-sources":         "agents/short/repo-sources.md",
		"store-access":         "agents/short/store-access.md",
		"memory-context":       "agents/short/memory-context.md",
		"knowledge-trigger":    "agents/short/knowledge-trigger.md",
		"spec-trigger":         "agents/short/spec-trigger.md",
		"design-trigger":       "agents/short/design-trigger.md",
		"draft-presentation":   "agents/short/draft-presentation.md",
		"historical-artifacts": "agents/short/historical-artifacts.md",
	}
	require.Len(t, standingRules, len(want))
	for _, r := range standingRules {
		require.Equal(t, want[r.slug], r.templatePath(), "template path for %q", r.slug)
		_, err := fs.Stat(templates.FS, r.templatePath())
		require.NoError(t, err, "embedded template for %q", r.slug)

		require.Equal(t, wantShort[r.slug], r.shortTemplatePath(), "short template path for %q", r.slug)
		_, err = fs.Stat(templates.FS, r.shortTemplatePath())
		require.NoError(t, err, "embedded short template for %q", r.slug)
	}
}

// An unknown slug is an error that tells the developer where to add the rule.
func TestInstallStandingRule_UnknownSlugNamesTheTable(t *testing.T) {
	withSourceFS(t, standingRuleFixtureFS())
	tmp := t.TempDir()

	err := installStandingRule(tmp, config.Config{Command: "go run ."}, io.Discard, "no-such-rule")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-rule")
	require.Contains(t, err.Error(), "standing_rules.go")

	_, statErr := os.Stat(filepath.Join(tmp, "AGENTS.md"))
	require.True(t, os.IsNotExist(statErr), "an unknown rule must not write AGENTS.md")
}

// A known slug installs that one rule, its guidance file and its section, and
// nothing else.
func TestInstallStandingRule_InstallsOnlyTheNamedRule(t *testing.T) {
	withSourceFS(t, standingRuleFixtureFS())
	tmp := t.TempDir()

	require.NoError(t, installStandingRule(tmp, config.Config{Command: "go run ."}, io.Discard, "spec-trigger"))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, "## Spec-Worthy Discussion Recognition\n\nspec-trigger body go run .\nsee .spektacular/guidance/spec-trigger.md\n", string(got))

	require.Equal(t, "# spec-trigger guidance\n\nspec-trigger full go run .\n", readGuidanceFile(t, tmp, "spec-trigger"))
	entries, err := os.ReadDir(filepath.Join(tmp, ".spektacular", "guidance"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the named rule's guidance file is written")
}

// Into an empty project the eight rules are written in table order, each
// guidance file before its section, and a second run changes nothing.
func TestInstallStandingRules_EmptyProjectByteExact(t *testing.T) {
	withSourceFS(t, standingRuleFixtureFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}
	agentsPath := filepath.Join(tmp, "AGENTS.md")

	var log bytes.Buffer
	require.NoError(t, installStandingRules(tmp, cfg, &log))

	guidance := func(slug string) string {
		return "  Guidance: " + filepath.Join(tmp, ".spektacular", "guidance", slug+".md") + "\n"
	}

	got, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	require.Equal(t, standingRulesFixtureSections, string(got))
	requireFixtureGuidance(t, tmp)
	require.Equal(t,
		guidance("repo-sources")+
			"  AGENTS.md: created "+agentsPath+"\n"+
			guidance("store-access")+
			"  AGENTS.md: appended Spektacular file access section to "+agentsPath+"\n"+
			guidance("memory-context")+
			"  AGENTS.md: appended Memory & Context section to "+agentsPath+"\n"+
			guidance("knowledge-trigger")+
			"  AGENTS.md: appended Knowledge-Worthy Discovery Recognition section to "+agentsPath+"\n"+
			guidance("spec-trigger")+
			"  AGENTS.md: appended Spec-Worthy Discussion Recognition section to "+agentsPath+"\n"+
			guidance("design-trigger")+
			"  AGENTS.md: appended Design-Worthy Detail Recognition section to "+agentsPath+"\n"+
			guidance("draft-presentation")+
			"  AGENTS.md: appended Presenting Drafts and Confirmations section to "+agentsPath+"\n"+
			guidance("historical-artifacts")+
			"  AGENTS.md: appended Historical Artifacts section to "+agentsPath+"\n",
		log.String())

	log.Reset()
	require.NoError(t, installStandingRules(tmp, cfg, &log))

	again, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	require.Equal(t, standingRulesFixtureSections, string(again))
	requireFixtureGuidance(t, tmp)
	require.Equal(t,
		guidance("repo-sources")+
			"  AGENTS.md: updated Where the Code Lives section in "+agentsPath+"\n"+
			guidance("store-access")+
			"  AGENTS.md: updated Spektacular file access section in "+agentsPath+"\n"+
			guidance("memory-context")+
			"  AGENTS.md: updated Memory & Context section in "+agentsPath+"\n"+
			guidance("knowledge-trigger")+
			"  AGENTS.md: updated Knowledge-Worthy Discovery Recognition section in "+agentsPath+"\n"+
			guidance("spec-trigger")+
			"  AGENTS.md: updated Spec-Worthy Discussion Recognition section in "+agentsPath+"\n"+
			guidance("design-trigger")+
			"  AGENTS.md: updated Design-Worthy Detail Recognition section in "+agentsPath+"\n"+
			guidance("draft-presentation")+
			"  AGENTS.md: updated Presenting Drafts and Confirmations section in "+agentsPath+"\n"+
			guidance("historical-artifacts")+
			"  AGENTS.md: updated Historical Artifacts section in "+agentsPath+"\n",
		log.String())
}

// Into a project with existing content the rules follow the project's own
// text, where-the-code-lives first, and a second run changes nothing.
func TestInstallStandingRules_ExistingContentByteExact(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		want     string
	}{
		{
			name:     "preamble only",
			existing: "# Team rules\n\nKeep A.\n",
			want:     "# Team rules\n\nKeep A.\n\n" + standingRulesFixtureSections,
		},
		{
			name:     "project section",
			existing: "# Team rules\n\n## House style\n\nKeep A.\n",
			want: "# Team rules\n" +
				"\n" +
				"## Where the Code Lives — Read This First\n" +
				"\n" +
				"repo-sources body go run .\n" +
				"see .spektacular/guidance/repo-sources.md\n" +
				"\n" +
				"## House style\n" +
				"\n" +
				"Keep A.\n" +
				"\n" +
				"## Spektacular's Files Are Reached Through Spektacular\n" +
				"\n" +
				"store-access body go run .\n" +
				"see .spektacular/guidance/store-access.md\n" +
				"\n" +
				"## Memory & Context\n" +
				"\n" +
				"memory-context body go run .\n" +
				"see .spektacular/guidance/memory-context.md\n" +
				"\n" +
				"## Knowledge-Worthy Discovery Recognition\n" +
				"\n" +
				"knowledge-trigger body go run .\n" +
				"see .spektacular/guidance/knowledge-trigger.md\n" +
				"\n" +
				"## Spec-Worthy Discussion Recognition\n" +
				"\n" +
				"spec-trigger body go run .\n" +
				"see .spektacular/guidance/spec-trigger.md\n" +
				"\n" +
				"## Design-Worthy Detail Recognition\n" +
				"\n" +
				"design-trigger body go run .\n" +
				"see .spektacular/guidance/design-trigger.md\n" +
				"\n" +
				"## Presenting Drafts and Confirmations\n" +
				"\n" +
				"draft-presentation body go run .\n" +
				"see .spektacular/guidance/draft-presentation.md\n" +
				"\n" +
				"## Historical Artifacts: Specs and Plans as Archaeology\n" +
				"\n" +
				"historical-artifacts body go run .\n" +
				"see .spektacular/guidance/historical-artifacts.md\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withSourceFS(t, standingRuleFixtureFS())
			tmp := t.TempDir()
			cfg := config.Config{Command: "go run ."}
			agentsPath := filepath.Join(tmp, "AGENTS.md")
			require.NoError(t, os.WriteFile(agentsPath, []byte(tc.existing), 0644))

			var log bytes.Buffer
			require.NoError(t, installStandingRules(tmp, cfg, &log))

			got, err := os.ReadFile(agentsPath)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
			requireFixtureGuidance(t, tmp)
			require.True(t, strings.HasPrefix(log.String(),
				"  Guidance: "+filepath.Join(tmp, ".spektacular", "guidance", "repo-sources.md")+"\n"+
					"  AGENTS.md: appended Where the Code Lives section to "+agentsPath+"\n"+
					"  Guidance: "+filepath.Join(tmp, ".spektacular", "guidance", "store-access.md")+"\n"+
					"  AGENTS.md: appended Spektacular file access section to "+agentsPath+"\n"),
				"log: %s", log.String())

			require.NoError(t, installStandingRules(tmp, cfg, io.Discard))
			again, err := os.ReadFile(agentsPath)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(again))
		})
	}
}

// Every agent installs the eight rules from the real templates, each exactly
// once, in the same order, with where-the-code-lives at the top.
func TestInstallStandingRules_EveryAgentInstallsEightRulesInOrder(t *testing.T) {
	for _, name := range []string{"claude", "codex", "bob", "copilot", "omp"} {
		t.Run(name, func(t *testing.T) {
			a, err := Lookup(name)
			require.NoError(t, err)

			for _, existing := range []string{"", "# Team rules\n\nKeep A.\n"} {
				tmp := t.TempDir()
				if existing != "" {
					require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))
				}
				require.NoError(t, a.Install(tmp, config.Config{Command: "go run ."}, io.Discard))

				got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
				require.NoError(t, err)

				var headings []string
				for _, line := range strings.Split(string(got), "\n") {
					if strings.HasPrefix(line, "## ") {
						headings = append(headings, line)
					}
				}
				require.Equal(t, standingRuleHeadingsInOrder, headings,
					"agent %q (existing %q) must install each standing rule once, in order", name, existing)
				if existing != "" {
					require.True(t, strings.HasPrefix(string(got), existing+"\n"+standingRuleHeadingsInOrder[0]+"\n"),
						"agent %q must keep the project's text first and put where-the-code-lives right after it", name)
				}
			}
		})
	}
}

// omp writes one always-applied rule file per standing rule, named
// spek-<slug>.md, each equal to its section of AGENTS.md.
func TestInstallStandingRules_OMPWritesOneRuleFilePerSection(t *testing.T) {
	tmp := t.TempDir()
	a, err := Lookup("omp")
	require.NoError(t, err)
	require.NoError(t, a.Install(tmp, config.Config{Command: "go run ."}, io.Discard))

	agents, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	// Cut AGENTS.md at the hand-listed headings: in an empty project it holds
	// only the eight standing rules, back to back.
	body := string(agents)
	sections := make([]string, len(standingRuleHeadingsInOrder))
	for i, h := range standingRuleHeadingsInOrder {
		start := strings.Index(body, h+"\n")
		require.GreaterOrEqual(t, start, 0, "heading %q", h)
		end := len(body)
		if i+1 < len(standingRuleHeadingsInOrder) {
			end = strings.Index(body, standingRuleHeadingsInOrder[i+1]+"\n")
			require.Greater(t, end, start)
		}
		sections[i] = strings.TrimSpace(body[start:end])
	}

	files := []string{
		"spek-repo-sources.md",
		"spek-store-access.md",
		"spek-memory-context.md",
		"spek-knowledge-trigger.md",
		"spek-spec-trigger.md",
		"spek-design-trigger.md",
		"spek-draft-presentation.md",
		"spek-historical-artifacts.md",
	}

	entries, err := os.ReadDir(filepath.Join(tmp, ".omp", "rules"))
	require.NoError(t, err)
	var onDisk []string
	for _, e := range entries {
		onDisk = append(onDisk, e.Name())
	}
	wantSorted := append([]string(nil), files...)
	sort.Strings(wantSorted)
	require.Equal(t, wantSorted, onDisk)

	for i, f := range files {
		got, err := os.ReadFile(filepath.Join(tmp, ".omp", "rules", f))
		require.NoError(t, err)
		require.Equal(t, "---\nalwaysApply: true\n---\n\n"+sections[i]+"\n", string(got), f)

		// The rule file is the short statement, ending in its guidance link,
		// never the full guidance.
		slug := strings.TrimSuffix(strings.TrimPrefix(f, "spek-"), ".md")
		link := ".spektacular/guidance/" + slug + ".md"
		require.True(t, strings.HasSuffix(string(got), "\nManaged by `go run . init`; full guidance: ["+link+"]("+link+").\n"),
			"%s must end with its guidance link line, got:\n%s", f, got)
		require.Equal(t, slug, standingRuleExpectations[i].slug)
		require.NotContains(t, string(got), standingRuleExpectations[i].fullOnly, "%s must not carry the full guidance", f)
	}
}

// Adding a rule is one table entry plus its two templates: the guidance
// writer, the AGENTS.md installer and the omp rule writer all pick it up with
// no other change.
func TestStandingRuleTable_NewEntryIsInstalledEverywhere(t *testing.T) {
	orig := standingRules
	standingRules = append(append([]standingRule(nil), orig...), standingRule{
		slug:      "extra-rule",
		heading:   "## Extra Rule",
		placement: placeAtEnd,
		noun:      "Extra Rule section",
	})
	t.Cleanup(func() { standingRules = orig })

	fsys := standingRuleFixtureFS()
	fsys["agents/extra-rule.md"] = &fstest.MapFile{Data: []byte("# Extra Rule\n\nextra-rule full {{command}}\n")}
	fsys["agents/short/extra-rule.md"] = &fstest.MapFile{Data: []byte("## Extra Rule\n\nextra-rule body {{command}}\nsee {{guidance_path}}\n")}
	withSourceFS(t, fsys)

	tmp := t.TempDir()
	agentsPath := filepath.Join(tmp, "AGENTS.md")
	var log bytes.Buffer
	require.NoError(t, installStandingRules(tmp, config.Config{Command: "go run ."}, &log))

	got, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	require.Equal(t, standingRulesFixtureSections+"\n## Extra Rule\n\nextra-rule body go run .\nsee .spektacular/guidance/extra-rule.md\n", string(got))
	require.Equal(t, "# Extra Rule\n\nextra-rule full go run .\n", readGuidanceFile(t, tmp, "extra-rule"))
	require.True(t, strings.HasSuffix(log.String(),
		"  Guidance: "+filepath.Join(tmp, ".spektacular", "guidance", "extra-rule.md")+"\n"+
			"  AGENTS.md: appended Extra Rule section to "+agentsPath+"\n"), "log: %s", log.String())

	require.NoError(t, installOMPRules(tmp, io.Discard))
	rule, err := os.ReadFile(filepath.Join(tmp, ".omp", "rules", "spek-extra-rule.md"))
	require.NoError(t, err)
	require.Equal(t, "---\nalwaysApply: true\n---\n\n## Extra Rule\n\nextra-rule body go run .\nsee .spektacular/guidance/extra-rule.md\n", string(rule))

	entries, err := os.ReadDir(filepath.Join(tmp, ".omp", "rules"))
	require.NoError(t, err)
	require.Len(t, entries, 9)
}

// standingRuleExpectation is the hand-maintained contract for one rule's two
// installed pieces, rendered for cfg.Command = "go run .".
type standingRuleExpectation struct {
	slug string
	// heading is the rule's AGENTS.md heading.
	heading string
	// guidanceTitle is the first line of the rule's guidance file.
	guidanceTitle string
	// compact are phrases the short AGENTS.md section must carry inline: the
	// binding obligation an agent must follow without opening the guidance.
	compact []string
	// trigger is the phrase in the short section naming when to read further.
	trigger string
	// link is the markdown link from the short section to the guidance file.
	link string
	// fullOnly is a sentence that appears only in the rule's full guidance,
	// never in any installed instruction file.
	fullOnly string
}

// standingRuleExpectations lists every rule's contract, in install order.
// memory-context and draft-presentation have no read-further sentence: the
// whole rule fits in the short section, so their trigger is the link line.
var standingRuleExpectations = []standingRuleExpectation{
	{
		slug:          "repo-sources",
		heading:       "## Where the Code Lives — Read This First",
		guidanceTitle: "# Where the Code Lives — Read This First",
		compact:       []string{"go run . repo list", "root", "sub-agent", "STOP"},
		trigger:       "**Before any code-touching work**",
		link:          "[.spektacular/guidance/repo-sources.md](.spektacular/guidance/repo-sources.md)",
		fullOnly:      "This rule outranks your default assumptions about working directories",
	},
	{
		slug:          "store-access",
		heading:       "## Spektacular's Files Are Reached Through Spektacular",
		guidanceTitle: "# Spektacular's Files Are Reached Through Spektacular",
		compact: []string{
			"never build a store path by hand",
			".spektacular/tmp/",
			".spektacular/work/<name>/",
			".spektacular/working-context.md",
			"sub-agent",
			".spektacular/guidance/",
		},
		trigger:  "Before touching `.spektacular/`, read the full guidance.",
		link:     "[.spektacular/guidance/store-access.md](.spektacular/guidance/store-access.md)",
		fullOnly: "Removal is a CLI verb too",
	},
	{
		slug:          "memory-context",
		heading:       "## Memory & Context",
		guidanceTitle: "# Memory & Context",
		compact:       []string{"spek-knowledge"},
		trigger:       "Managed by `go run . init`; full guidance: [.spektacular/guidance/memory-context.md]",
		link:          "[.spektacular/guidance/memory-context.md](.spektacular/guidance/memory-context.md)",
		fullOnly:      "The skill handles tier and store selection",
	},
	{
		slug:          "knowledge-trigger",
		heading:       "## Knowledge-Worthy Discovery Recognition",
		guidanceTitle: "# Knowledge-Worthy Discovery Recognition",
		compact:       []string{"write it via `spek-knowledge` yourself", "tell the user which entry, where", "Do not ask first."},
		trigger:       "Before writing, read the full guidance.",
		link:          "[.spektacular/guidance/knowledge-trigger.md](.spektacular/guidance/knowledge-trigger.md)",
		fullOnly:      "This is the recognition trigger only",
	},
	{
		slug:          "spec-trigger",
		heading:       "## Spec-Worthy Discussion Recognition",
		guidanceTitle: "# Spec-Worthy Discussion Recognition",
		compact:       []string{"offer", "never start"},
		trigger:       "Before offering, read the full guidance.",
		link:          "[.spektacular/guidance/spec-trigger.md](.spektacular/guidance/spec-trigger.md)",
		fullOnly:      "Reading a well-specified issue and going straight to implementation",
	},
	{
		slug:          "design-trigger",
		heading:       "## Design-Worthy Detail Recognition",
		guidanceTitle: "# Design-Worthy Detail Recognition",
		compact:       []string{"offer", "spek-design", "never write"},
		trigger:       "Before offering, read the full",
		link:          "[.spektacular/guidance/design-trigger.md](.spektacular/guidance/design-trigger.md)",
		fullOnly:      "Silence or deflection is not acceptance.",
	},
	{
		slug:          "draft-presentation",
		heading:       "## Presenting Drafts and Confirmations",
		guidanceTitle: "# Presenting Drafts and Confirmations",
		compact:       []string{"plain chat text"},
		trigger:       "Managed by `go run . init`; full guidance: [.spektacular/guidance/draft-presentation.md]",
		link:          "[.spektacular/guidance/draft-presentation.md](.spektacular/guidance/draft-presentation.md)",
		fullOnly:      "that kind of UI truncates or compresses long text",
	},
	{
		slug:          "historical-artifacts",
		heading:       "## Historical Artifacts: Specs and Plans as Archaeology",
		guidanceTitle: "# Historical Artifacts: Specs and Plans as Archaeology",
		compact:       []string{"past intent", "Knowledge entries are binding"},
		trigger:       "Before reading a spec or plan, read the full guidance.",
		link:          "[.spektacular/guidance/historical-artifacts.md](.spektacular/guidance/historical-artifacts.md)",
		fullOnly:      "Archaeology is the only allowed reason to open these",
	},
}

// installedStandingRuleSections installs the real standing rules into a fresh
// project and returns it with AGENTS.md cut into its sections, keyed by the
// hand-listed headings.
func installedStandingRuleSections(t *testing.T) (string, map[string]string) {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, installStandingRules(tmp, config.Config{Command: "go run ."}, io.Discard))

	agents, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	body := string(agents)
	sections := map[string]string{}
	for i, h := range standingRuleHeadingsInOrder {
		start := strings.Index(body, h+"\n")
		require.GreaterOrEqual(t, start, 0, "heading %q", h)
		end := len(body)
		if i+1 < len(standingRuleHeadingsInOrder) {
			end = strings.Index(body, standingRuleHeadingsInOrder[i+1]+"\n")
			require.Greater(t, end, start)
		}
		sections[h] = body[start:end]
	}
	return tmp, sections
}

// Each rule's short AGENTS.md section states the binding obligation inline,
// names when to read further, and links to a guidance file that exists.
func TestStandingRules_CompactSectionsStateTheBindingRule(t *testing.T) {
	tmp, sections := installedStandingRuleSections(t)
	require.Len(t, standingRuleExpectations, len(standingRules))

	for _, want := range standingRuleExpectations {
		section, ok := sections[want.heading]
		require.True(t, ok, "AGENTS.md has no section %q", want.heading)

		for _, needle := range want.compact {
			require.Containsf(t, section, needle,
				"the short %s section must state %q inline", want.slug, needle)
		}
		require.Containsf(t, section, want.trigger,
			"the short %s section must say when to read the full guidance", want.slug)
		require.Containsf(t, section, want.link,
			"the short %s section must link to its guidance file", want.slug)
		require.Containsf(t, section, "Managed by `go run . init`; full guidance:",
			"the short %s section must end with the managed-by link line", want.slug)
		require.NotContainsf(t, section, want.fullOnly,
			"the short %s section must not carry the full guidance", want.slug)
		require.FileExists(t, filepath.Join(tmp, ".spektacular", "guidance", want.slug+".md"))
	}
}

// Each rule's guidance file opens with the rule's title as a top-level
// heading, carries the managed-by banner, and holds the full text.
func TestStandingRules_GuidanceFilesHoldTheFullText(t *testing.T) {
	tmp, _ := installedStandingRuleSections(t)

	for _, want := range standingRuleExpectations {
		guidance := readGuidanceFile(t, tmp, want.slug)
		require.Truef(t, strings.HasPrefix(guidance, want.guidanceTitle+"\n\n"),
			"the %s guidance file must open with %q, got:\n%s", want.slug, want.guidanceTitle, guidance)
		require.Containsf(t, guidance, "> Managed by `go run . init` and refreshed by `go run . migrate`",
			"the %s guidance file must carry the managed-by banner", want.slug)
		require.Containsf(t, guidance, want.fullOnly,
			"the %s guidance file must hold the rule's full text", want.slug)
		require.NotContainsf(t, guidance, "{{",
			"the %s guidance file must not leak a mustache placeholder", want.slug)
	}
}

// No instruction file an agent loads at session start imports a guidance
// file or carries a guidance file's body: the full text is read on demand,
// through the short section's link, and never loaded up front.
func TestStandingRules_NoInstructionFileLoadsGuidance(t *testing.T) {
	for _, name := range []string{"claude", "codex", "bob", "copilot", "omp"} {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			a, err := Lookup(name)
			require.NoError(t, err)
			require.NoError(t, a.Install(tmp, config.NewDefault(), io.Discard))
			requireGuidanceFilesInstalled(t, tmp)

			instructionFiles := []string{"AGENTS.md"}
			if name == "claude" {
				instructionFiles = append(instructionFiles, "CLAUDE.md")
			}
			if name == "omp" {
				rules, err := os.ReadDir(filepath.Join(tmp, ".omp", "rules"))
				require.NoError(t, err)
				require.Len(t, rules, 8)
				for _, r := range rules {
					instructionFiles = append(instructionFiles, filepath.Join(".omp", "rules", r.Name()))
				}
			}

			for _, rel := range instructionFiles {
				body, err := os.ReadFile(filepath.Join(tmp, rel))
				require.NoError(t, err)
				require.NotContainsf(t, string(body), "@.spektacular/guidance",
					"%s must not import a guidance file", rel)
				for _, want := range standingRuleExpectations {
					require.NotContainsf(t, string(body), want.fullOnly,
						"%s must not carry the %s guidance body", rel, want.slug)
				}
			}

			if name == "claude" {
				claude, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
				require.NoError(t, err)
				require.Equal(t, 1, strings.Count(string(claude), "@AGENTS.md"),
					"CLAUDE.md must still import AGENTS.md, exactly once")
			}
		})
	}
}

// Guidance files are read with the agent's own tools, so the store-access
// rule must exempt them on both surfaces: the short section an agent always
// sees, and the full guidance it reads before touching .spektacular/.
func TestStandingRules_StoreAccessExemptsGuidanceOnBothSurfaces(t *testing.T) {
	tmp, sections := installedStandingRuleSections(t)

	section := sections["## Spektacular's Files Are Reached Through Spektacular"]
	require.Contains(t, section, "yours to read, never edit: `.spektacular/guidance/`",
		"the short store-access section must exempt guidance files for reading only")

	guidance := readGuidanceFile(t, tmp, "store-access")
	for _, needle := range []string{
		"The guidance files under `.spektacular/guidance/`, this one included, are not a\nstore either.",
		"Read them with your own file tools",
		"Never edit, move or delete them",
	} {
		require.Containsf(t, guidance, needle,
			"the store-access guidance must exempt guidance files: %q", needle)
	}
}

// standingRuleAgents is the hand-maintained list of every agent Spektacular
// installs the standing rules for.
var standingRuleAgents = []string{"claude", "codex", "bob", "copilot", "omp"}

// legacyAgentsMDFixture is the AGENTS.md the pre-compact binary wrote, holding
// every rule's full text, with the project's own blocks before, between and
// after Spektacular's sections.
const legacyAgentsMDFixture = "testdata/agents-md-legacy.md"

// The project's own blocks in the legacy fixture, written out by hand.
const (
	legacyUserBlockBefore  = "# Team rules\n\nKeep A.\n"
	legacyUserBlockBetween = "## Our section\n\nKeep B.\n"
	legacyUserBlockAfter   = "## Trailing\n\nKeep C.\n"
)

// legacyFullBanner opens every full section the pre-compact binary wrote into
// AGENTS.md. A converted file carries none of it.
const legacyFullBanner = "> Managed by `spektacular init` — edit `templates/agents/"

// compactBudgetBytes is the most AGENTS.md bytes Spektacular's sections may
// take together, and preCompactBaselineBytes the size of those sections in a
// real install before the change; the compact form must be under a quarter
// of it.
const (
	compactBudgetBytes      = 5120
	preCompactBaselineBytes = 21722
)

// standingRuleLineLimit is the most non-blank lines a short section may hold
// below its heading, the link line included.
func standingRuleLineLimit(slug string) int {
	if slug == "repo-sources" {
		return 15
	}
	return 6
}

// markdownLink matches one markdown link, capturing its text and target.
var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)

// shippedGuidance renders a rule's embedded full template for the default
// command, newline-terminated, exactly as a guidance file must hold it. It is
// the "nothing is lost" oracle: the guidance file is the full template.
func shippedGuidance(t *testing.T, slug string) string {
	t.Helper()
	tmpl, err := fs.ReadFile(templates.FS, "agents/"+slug+".md")
	require.NoError(t, err)
	rendered, err := mustache.Render(string(tmpl), map[string]string{"command": "spektacular"})
	require.NoError(t, err)
	if !strings.HasSuffix(rendered, "\n") {
		rendered += "\n"
	}
	return rendered
}

// installAgent installs the named agent into projectPath with the default
// config.
func installAgent(t *testing.T, name, projectPath string) {
	t.Helper()
	a, err := Lookup(name)
	require.NoError(t, err)
	require.NoError(t, a.Install(projectPath, config.NewDefault(), io.Discard))
}

// copyLegacyFixture writes the legacy AGENTS.md fixture into projectPath.
func copyLegacyFixture(t *testing.T, projectPath string) {
	t.Helper()
	legacy, err := os.ReadFile(legacyAgentsMDFixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "AGENTS.md"), legacy, 0644))
}

// instructionFiles returns every file an agent loads at session start, as
// project-relative paths.
func instructionFiles(t *testing.T, name, projectPath string) []string {
	t.Helper()
	files := []string{"AGENTS.md"}
	if name == "claude" {
		files = append(files, "CLAUDE.md")
	}
	if name == "omp" {
		rules, err := os.ReadDir(filepath.Join(projectPath, ".omp", "rules"))
		require.NoError(t, err)
		for _, r := range rules {
			files = append(files, filepath.Join(".omp", "rules", r.Name()))
		}
	}
	return files
}

// requireCompactSection asserts one installed section is the short form: at
// most its line limit below the heading, every line but the link line at
// most 100 runes, exactly one markdown link, to the rule's guidance file,
// which exists.
func requireCompactSection(t *testing.T, projectPath, slug, section string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(section, "\n"), "\n")
	var body []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" {
			body = append(body, line)
		}
	}
	require.LessOrEqualf(t, len(body), standingRuleLineLimit(slug),
		"the %s section holds %d lines below its heading:\n%s", slug, len(body), section)
	for _, line := range body {
		if strings.HasPrefix(line, "Managed by") {
			continue
		}
		require.LessOrEqualf(t, utf8.RuneCountInString(line), 100,
			"the %s section has an over-long line: %q", slug, line)
	}

	links := markdownLink.FindAllStringSubmatch(section, -1)
	require.Lenf(t, links, 1, "the %s section must hold exactly one link:\n%s", slug, section)
	want := ".spektacular/guidance/" + slug + ".md"
	require.Equal(t, want, links[0][2], "the %s section's link target", slug)
	require.Equal(t, want, links[0][1], "the %s section's link text", slug)
	require.FileExists(t, filepath.Join(projectPath, ".spektacular", "guidance", slug+".md"))
	require.NotContains(t, section, "@.spektacular/guidance", "the %s section must not import its guidance", slug)
}

// For every agent, a fresh install keeps Spektacular's AGENTS.md sections
// under budget and each short, linking to a guidance file that exists, while
// no instruction file loads any of the full guidance.
func TestStandingRules_BudgetForEveryAgent(t *testing.T) {
	for _, name := range standingRuleAgents {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			installAgent(t, name, tmp)

			agents, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
			require.NoError(t, err)

			total := 0
			for _, want := range standingRuleExpectations {
				start, end, found := locateManagedSection(agents, want.heading)
				require.Truef(t, found, "AGENTS.md has no section %q", want.heading)
				section := string(agents[start:end])
				total += len(section)
				requireCompactSection(t, tmp, want.slug, section)
			}
			require.Less(t, total, compactBudgetBytes, "Spektacular's AGENTS.md sections total %d bytes", total)
			require.Less(t, total*4, preCompactBaselineBytes,
				"the sections (%d bytes) must be under a quarter of the %d-byte baseline", total, preCompactBaselineBytes)

			// No paragraph of any guidance body (below its title and banner)
			// is carried by an instruction file.
			for _, rel := range instructionFiles(t, name, tmp) {
				body, err := os.ReadFile(filepath.Join(tmp, rel))
				require.NoError(t, err)
				require.NotContainsf(t, string(body), "@.spektacular/guidance", "%s must not import guidance", rel)
				for _, want := range standingRuleExpectations {
					paragraphs := strings.Split(readGuidanceFile(t, tmp, want.slug), "\n\n")
					for _, p := range paragraphs[1:] {
						if strings.HasPrefix(p, "> Managed by") || len(p) < 120 {
							continue
						}
						require.NotContainsf(t, string(body), p,
							"%s carries a paragraph of the %s guidance", rel, want.slug)
					}
				}
			}
		})
	}
}

// Every guidance file is the rule's full template, rendered and nothing lost.
func TestStandingRules_GuidanceHoldsFullTemplate(t *testing.T) {
	for _, name := range standingRuleAgents {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			installAgent(t, name, tmp)
			for _, slug := range standingRuleSlugsInOrder {
				require.Equal(t, shippedGuidance(t, slug), readGuidanceFile(t, tmp, slug), "guidance for %q", slug)
			}
			entries, err := os.ReadDir(filepath.Join(tmp, ".spektacular", "guidance"))
			require.NoError(t, err)
			require.Len(t, entries, len(standingRuleSlugsInOrder), "only the eight guidance files are written")
		})
	}
}

// A project initialised before the compact form, with its own content before,
// between and after Spektacular's full sections, converts in place: its own
// blocks stay byte for byte where they were, and every section is short.
func TestStandingRules_ConvertsLegacyFullSections(t *testing.T) {
	for _, name := range standingRuleAgents {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			copyLegacyFixture(t, tmp)
			installAgent(t, name, tmp)

			fresh := t.TempDir()
			installAgent(t, name, fresh)
			freshAgents, err := os.ReadFile(filepath.Join(fresh, "AGENTS.md"))
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
			require.NoError(t, err)
			body := string(got)

			for _, block := range []string{legacyUserBlockBefore, legacyUserBlockBetween, legacyUserBlockAfter} {
				require.Equal(t, 1, strings.Count(body, block), "the project's block %q must survive once", block)
			}
			require.NotContains(t, body, legacyFullBanner, "no legacy full section may remain")

			// Hand-written order: the project's blocks around the rules.
			order := []string{
				legacyUserBlockBefore,
				"## Where the Code Lives — Read This First\n",
				"## Spektacular's Files Are Reached Through Spektacular\n",
				legacyUserBlockBetween,
				"## Memory & Context\n",
				"## Knowledge-Worthy Discovery Recognition\n",
				"## Spec-Worthy Discussion Recognition\n",
				"## Design-Worthy Detail Recognition\n",
				"## Presenting Drafts and Confirmations\n",
				"## Historical Artifacts: Specs and Plans as Archaeology\n",
				legacyUserBlockAfter,
			}
			prev := -1
			for _, piece := range order {
				idx := strings.Index(body, piece)
				require.Greaterf(t, idx, prev, "%q must follow the piece before it", piece)
				prev = idx
			}

			rest := got
			for _, want := range standingRuleExpectations {
				require.Equal(t, 1, strings.Count(body, want.heading+"\n"), "heading %q", want.heading)
				start, end, found := locateManagedSection(got, want.heading)
				require.True(t, found)
				section := string(got[start:end])
				requireCompactSection(t, tmp, want.slug, section)

				fStart, fEnd, ok := locateManagedSection(freshAgents, want.heading)
				require.True(t, ok)
				require.Equal(t, strings.TrimSpace(string(freshAgents[fStart:fEnd])), strings.TrimSpace(section),
					"the converted %s section must be the shipped short section", want.slug)

				rs, re, ok := locateManagedSection(rest, want.heading)
				require.True(t, ok)
				rest = append(append([]byte(nil), rest[:rs]...), rest[re:]...)
			}
			require.Equal(t, legacyUserBlockBefore+"\n"+legacyUserBlockBetween+"\n"+legacyUserBlockAfter, string(rest),
				"outside Spektacular's sections, AGENTS.md must hold exactly the project's own blocks")
			requireGuidanceFilesInstalled(t, tmp)
		})
	}
}

// Installing twice leaves every file, AGENTS.md and the guidance included,
// identical to installing once, from an empty project and from a legacy one.
func TestStandingRules_Idempotent(t *testing.T) {
	for _, name := range standingRuleAgents {
		for _, start := range []string{"fresh", "legacy"} {
			t.Run(name+"/"+start, func(t *testing.T) {
				tmp := t.TempDir()
				if start == "legacy" {
					copyLegacyFixture(t, tmp)
				}
				installAgent(t, name, tmp)
				once := ompSnapshot(t, tmp)
				installAgent(t, name, tmp)
				require.Equal(t, once, ompSnapshot(t, tmp))
			})
		}
	}
}

// Reinstalling restores guidance files that were changed or deleted to the
// shipped content.
func TestStandingRules_RefreshesStaleGuidance(t *testing.T) {
	for _, name := range standingRuleAgents {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			installAgent(t, name, tmp)
			dir := filepath.Join(tmp, ".spektacular", "guidance")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "store-access.md"), []byte("stale\n"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "spec-trigger.md"), []byte("stale\n"), 0644))
			require.NoError(t, os.Remove(filepath.Join(dir, "design-trigger.md")))

			installAgent(t, name, tmp)

			for _, slug := range []string{"store-access", "spec-trigger", "design-trigger"} {
				require.Equal(t, shippedGuidance(t, slug), readGuidanceFile(t, tmp, slug), "guidance for %q", slug)
			}
			requireGuidanceFilesInstalled(t, tmp)
		})
	}
}
