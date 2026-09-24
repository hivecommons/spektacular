package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// forbiddenInstructionSubstrings is the closed list of literal patterns that
// must never appear in the agent-facing instruction surface (skill templates,
// step templates, and the dogfooded rendered skills under .claude/skills/).
// Most entries encode a piece of the old stdin/heredoc interface that the
// `--from <path>` flag replaced.
//
// The last entry is different in kind: it is the exact sentence that used to
// claim Spektacular adds no frontmatter to a design document, full stop. That
// is now false — `design author` stamps a lifecycle record — and only `design
// write`, for a document Spektacular did not author, leaves the bytes alone.
// The guard is deliberately the old literal rather than a pattern over the
// idea, because the corrected wording legitimately says "adding no frontmatter
// to it" about the verbatim command. It therefore catches that one sentence
// returning, not the general class of unqualified frontmatter claims.
var forbiddenInstructionSubstrings = []string{
	"cat .spektacular/tmp/",
	"| {{config.command}} spec file write",
	"| {{config.command}} plan file write",
	"| go run . spec file write",
	"| go run . plan file write",
	"reads stdin",
	"with no frontmatter added and nothing reformatted",
}

// TestEmbeddedTemplatesAvoidStdinInstructionSurface walks the embedded
// templates filesystem under skills/workflows/ and steps/ and asserts no
// markdown file contains a pattern from the old stdin/heredoc CLI surface.
func TestEmbeddedTemplatesAvoidStdinInstructionSurface(t *testing.T) {
	roots := []string{"skills/workflows", "steps"}
	for _, root := range roots {
		err := fs.WalkDir(templates.FS, root, func(path string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			body, err := fs.ReadFile(templates.FS, path)
			require.NoError(t, err)
			assertNoForbiddenSubstring(t, path, string(body))
			return nil
		})
		require.NoError(t, err)
	}
}

// TestRenderedSkillsAvoidStdinInstructionSurface renders every workflow skill
// into a freshly-created temp directory via the real install path and asserts
// no rendered SKILL.md contains a pattern from the old stdin/heredoc CLI
// surface. The test owns the directory it walks — it does not depend on any
// pre-existing on-disk state.
func TestRenderedSkillsAvoidStdinInstructionSurface(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.NewDefault()

	require.NoError(t, installWorkflowSkills(tmp, ".claude/skills", cfg, io.Discard))

	root := filepath.Join(tmp, ".claude", "skills")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		assertNoForbiddenSubstring(t, path, string(body))
		return nil
	})
	require.NoError(t, err)
}

// TestRenderedSpekKnowledgeBodyContainsCRUDInvocations renders the workflow
// skills into a fresh temp directory and asserts the rendered spek-knowledge
// SKILL.md contains every CRUD entry point its prose orchestrates. This is a
// regression guard against a future edit accidentally dropping a load-bearing
// CLI reference; the expected substrings are hand-maintained as a literal Go
// slice rather than derived from the file. The test owns the directory it
// reads — it does not depend on any pre-existing on-disk state.
func TestRenderedSpekKnowledgeBodyContainsCRUDInvocations(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	expectedCRUDInvocations := []string{
		"knowledge search",
		"knowledge read",
		"knowledge sources",
		"knowledge write",
		"knowledge delete",
		"knowledge tags",
		"knowledge list",
	}
	for _, needle := range expectedCRUDInvocations {
		require.Contains(t, rendered, needle, "the rendered spek-knowledge skill is missing load-bearing CRUD invocation %q", needle)
	}
}

// TestRenderedWorkflowSkillsCarryCrossRepoNotes renders the workflow skills
// through the real install path and asserts the spek-plan and spek-implement
// SKILL.md files carry their Phase 4.2 cross-repo notes — roster-driven repo
// attribution for planning (criterion 2), attributed-repo execution with
// per-repo derived changelog entries for implementation (criterion 3) — with
// the {{command}} placeholder rendered away. The test owns the directory it
// reads — it does not depend on any pre-existing on-disk state.
func TestRenderedWorkflowSkillsCarryCrossRepoNotes(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.NewDefault()

	require.NoError(t, installWorkflowSkills(tmp, ".claude/skills", cfg, io.Discard))

	readSkill := func(t *testing.T, name string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(tmp, ".claude", "skills", name, "SKILL.md"))
		require.NoError(t, err)
		return string(body)
	}

	t.Run("spek-plan", func(t *testing.T) {
		body := readSkill(t, "spek-plan")

		// Criterion 2: planning attributes every requirement to its repo.
		require.Contains(t, body, "Cross-repo planning",
			"spek-plan must carry the cross-repo planning note")
		require.Contains(t, body, "attribute every requirement to the repo",
			"spek-plan must direct attributing every requirement to a repo")
		require.Contains(t, body, "spektacular repo list",
			"the {{command}} placeholder must render to the configured command")
		require.NotContains(t, body, "{{command}}",
			"the rendered skill must not leak the {{command}} placeholder")
		// Plan 000046: research happens in each repo's source, never in the
		// directory the agent is running in.
		require.Contains(t, body, "in the `root` reported for each",
			"spek-plan must direct research into the root repo list reports for each repo")
		require.NotContains(t, body, "running in",
			"spek-plan must not use the running directory as a stand-in for a repo")
	})

	t.Run("spek-implement", func(t *testing.T) {
		body := readSkill(t, "spek-implement")

		// Criterion 3: work runs in the attributed repo's resolved root and
		// derived changelog entries follow each affected repo.
		require.Contains(t, body, "Cross-repo implementation",
			"spek-implement must carry the cross-repo implementation note")
		require.Contains(t, body, "attributed repo's code",
			"spek-implement must direct work into the attributed repo's code")
		require.Contains(t, body, "reports where it lives as `root`",
			"spek-implement must say repo list reports where the code lives as root")
		require.NotContains(t, body, "resolved root",
			"spek-implement must not describe the working directory as a resolved root")
		require.Contains(t, body, "one derived entry per affected repo",
			"spek-implement must direct one derived changelog entry per affected repo")
		require.Contains(t, body, "--repo <name>",
			"derived entries must be written via `changelog file write ... --repo <name>`")
		require.NotContains(t, body, "{{command}}",
			"the rendered skill must not leak the {{command}} placeholder")
	})
}

// renderSpekKnowledgeSkill installs the workflow skills through the production
// install path into a directory the test owns, and returns the rendered
// spek-knowledge SKILL.md.
//
// Rendering rather than reading the committed .claude/ or .bob/ copy is
// deliberate and load-bearing: a guard that read a committed copy would still
// pass when someone edited a template and never re-ran init, which is the
// exact drift this repo's generated-copy discipline exists to prevent.
func renderSpekKnowledgeSkill(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	require.NoError(t, installWorkflowSkills(tmp, ".claude/skills", config.NewDefault(), io.Discard))

	body, err := os.ReadFile(filepath.Join(tmp, ".claude", "skills", "spek-knowledge", "SKILL.md"))
	require.NoError(t, err)
	return string(body)
}

// staleBooleanRetrievalClaims is the closed, hand-maintained list of literal
// phrasings of the superseded retrieval rule — that a document matched only
// when every word of the query occurred somewhere in it. Retrieval is now a
// ranked OR, so any surface reintroducing one of these phrasings would be
// teaching agents a rule the search no longer implements: they would stop
// searching after one empty result, believing it proved nothing existed.
var staleBooleanRetrievalClaims = []string{
	"matches when every query word",
	"every query word occurs",
}

// TestRenderedSpekKnowledgeCaptureFlowProposesTags renders the spek-knowledge
// skill through the production install path and asserts the contribute intent
// carries the tag-proposal flow: the vocabulary is loaded before tags are
// chosen, an existing tag is preferred over a near-duplicate, and the single
// confirmation gate names the proposed tags alongside the tier, store name and
// path — so nothing reaches a store until the user approves the tags too.
// Expected substrings are a hand-maintained literal slice, never derived from
// the file under test.
func TestRenderedSpekKnowledgeCaptureFlowProposesTags(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	// The vocabulary in use must be loaded before any tag is chosen, and the
	// {{command}} placeholder must have been rendered away.
	require.Contains(t, rendered, "spektacular knowledge tags",
		"the contribute intent must load the tag vocabulary with `knowledge tags`")
	require.Contains(t, rendered, "tag vocabulary already in use",
		"the skill must say what `knowledge tags` is for")

	// Criterion 3: converge on the vocabulary rather than minting a variant.
	require.Contains(t, rendered, "Choose the entry's tags",
		"the contribute intent must carry an explicit tag-choosing step")
	require.Contains(t, rendered, "Prefer a tag already in the vocabulary",
		"the skill must direct preferring an existing tag over a near-duplicate")
	require.Contains(t, rendered, "is tagged `http`, never `HTTP` or `http-api`",
		"the skill must give the worked near-duplicate example")

	// Criteria 1 and 2: one gate, showing tags with tier, store name and path,
	// before anything is written.
	require.Contains(t, rendered, "Show the user the destination, the proposed tags, and the body before writing",
		"the confirmation step must show the proposed tags before the write")
	require.Contains(t, rendered, "This is one gate, not two",
		"tags must be confirmed with everything else, not in a second gate")
	require.Contains(t, rendered, "Only after explicit confirmation",
		"no entry may reach a store before explicit confirmation")
}

// TestRenderedSpekKnowledgeCarriesTagFormRules asserts the three tag-form
// rules survive in the rendered skill, keyed on their distinctive worked
// examples rather than on full sentences.
//
// These rules are load-bearing rather than stylistic. Tags are what retrieve
// an entry, so a badly chosen tag form is not merely untidy — the entry is
// simply never found again, which is the failure the whole tagging feature
// exists to fix. Collapsing `https` into `http` loses a distinct subject;
// splitting `apple`/`apples` clutters the vocabulary for nothing; and missing
// the `route`/`routing` case leaves a genuinely likely search word unmatched.
func TestRenderedSpekKnowledgeCarriesTagFormRules(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	tagFormRules := []struct {
		needle string
		why    string
	}{
		{
			needle: "One form per subject",
			why:    "a singular and its plural must not be proposed as two tags",
		},
		{
			needle: "Add a second tag only where a form differs by more than its ending",
			why:    "a second tag is only warranted beyond a differing ending",
		},
		{
			needle: "`route` and `routing` do not find each other",
			why:    "the route/routing example is what makes the differing-ending rule concrete",
		},
		{
			needle: "Do not collapse distinct subjects into one tag",
			why:    "a merely similar subject must keep its own tag",
		},
		{
			needle: "an entry about HTTPS is tagged `https` even though `http` is already in the vocabulary",
			why:    "the https/http example is what stops convergence swallowing a distinct subject",
		},
	}
	for _, rule := range tagFormRules {
		require.Contains(t, rendered, rule.needle,
			"the rendered spek-knowledge skill lost a tag-form rule (%s): %q", rule.why, rule.needle)
	}
}

// TestRenderedSpekKnowledgeUpdateIntentPreservesTags asserts the update intent
// carries the entry's existing frontmatter through unchanged unless the
// revision is itself about the tags. A write replaces the whole file, so an
// update that drops the block silently strips the entry's tags.
func TestRenderedSpekKnowledgeUpdateIntentPreservesTags(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	require.Contains(t, rendered, "Carry the entry's existing frontmatter block through unchanged",
		"the update intent must preserve the existing frontmatter block")
	require.Contains(t, rendered, "unless the revision is itself about the tags",
		"the update intent must carve out revisions that are about the tags")
	require.Contains(t, rendered, "dropping the block silently strips the entry's tags",
		"the update intent must say why dropping the block is harmful")
	require.Contains(t, rendered, "Show the user the tier, store name, path, tags, and proposed new body",
		"an update must also show the tags before writing")
}

// TestRenderedRetrievalSurfacesDescribeRankedMatching asserts every rendered
// surface that tells an agent how knowledge search behaves describes it as a
// ranked OR, and that none of them still carries the superseded boolean rule.
// The NotContains half is the more valuable one: it is what catches a future
// edit reintroducing the old claim.
//
// Three surfaces describe retrieval, and each is reached through its own
// production path: the installed spek-knowledge skill, the plan discovery step
// template, and the spawn-planning-agents library skill.
func TestRenderedRetrievalSurfacesDescribeRankedMatching(t *testing.T) {
	t.Run("spek-knowledge skill", func(t *testing.T) {
		rendered := renderSpekKnowledgeSkill(t)

		require.Contains(t, rendered, "does not have to contain every word of your query",
			"the skill must describe retrieval as a ranked OR, not a boolean AND")
		require.Contains(t, rendered, "ranked on how much of the query it covers",
			"the skill must say results are ranked on query coverage")
		require.Contains(t, rendered, "A tag match counts far more heavily",
			"the skill must say a tag match outweighs the same word in the prose")
		require.Contains(t, rendered, "an empty result is **not** proof that nothing on the subject exists",
			"the skill must spell out that an empty result proves nothing")
		require.Contains(t, rendered, "--tag <tag>",
			"the skill must document narrowing results with --tag")
		assertNoStaleRetrievalClaim(t, "rendered spek-knowledge SKILL.md", rendered)
	})

	t.Run("plan discovery step", func(t *testing.T) {
		rendered := renderPlanDiscoveryStep(t)

		require.Contains(t, rendered, "does not have to contain every word of your query",
			"the discovery step must describe retrieval as a ranked OR, not a boolean AND")
		require.Contains(t, rendered, "ranked on how much of the query it covers",
			"the discovery step must say results are ranked on query coverage")
		require.Contains(t, rendered, "an empty result is not proof that nothing on the subject exists",
			"the discovery step must spell out that an empty result proves nothing")
		require.Contains(t, rendered, "narrow with a repeatable `--tag <tag>`",
			"the discovery step must document narrowing results with --tag")
		require.NotContains(t, rendered, "{{config.command}}",
			"the rendered step must not leak the {{config.command}} placeholder")
		assertNoStaleRetrievalClaim(t, "rendered steps/plan/02-discovery.md", rendered)
	})

	t.Run("spawn-planning-agents skill", func(t *testing.T) {
		served := servedSpawnPlanningAgentsSkill(t)

		require.Contains(t, served, "A document need not contain every query word",
			"the planning skill must describe retrieval as a ranked OR, not a boolean AND")
		require.Contains(t, served, "it is returned if it carries evidence for any of them",
			"the planning skill must describe the OR semantics explicitly")
		require.Contains(t, served, "an empty result is not proof that nothing on the subject exists",
			"the planning skill must spell out that an empty result proves nothing")
		assertNoStaleRetrievalClaim(t, "skills/skill_spawn-planning-agents.md", served)
	})
}

// TestRenderedSpekKnowledgeLeavesNoUnrenderedPlaceholder asserts the installed
// spek-knowledge skill carries no mustache placeholder the renderer did not
// substitute. A template edit that introduced an unknown placeholder would
// otherwise ship a literal `{{...}}` into the agent's instructions.
func TestRenderedSpekKnowledgeLeavesNoUnrenderedPlaceholder(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	require.Contains(t, rendered, "spektacular knowledge search",
		"the {{command}} placeholder must render to the configured command")
	for _, placeholder := range []string{"{{command}}", "{{config.command}}"} {
		require.NotContains(t, rendered, placeholder,
			"the rendered spek-knowledge skill must not leak the %s placeholder", placeholder)
	}
}

// renderPlanDiscoveryStep renders the plan workflow's discovery step through
// stepkit.RenderTemplate — the same production renderer stepkit.WriteStepResult
// drives when the workflow serves the step — supplying the standard template
// variables the plan strategy would. The step templates have no installed
// on-disk copy; this is the surface an agent is actually handed.
func renderPlanDiscoveryStep(t *testing.T) string {
	t.Helper()

	rendered, err := stepkit.RenderTemplate("steps/plan/02-discovery.md", map[string]any{
		"step":      "discovery",
		"title":     stepkit.StepTitle("discovery"),
		"next_step": "architecture",
		"plan_name": "000001_test",
		"config":    map[string]any{"command": config.NewDefault().Command},
	})
	require.NoError(t, err)
	return rendered
}

// servedSpawnPlanningAgentsSkill returns the spawn-planning-agents library
// skill exactly as production serves it. Library skills have no render or
// install step at all: `skill <name>` reads templates.FS and returns those
// bytes verbatim as the served instructions (see cmd/skill.go), so the
// embedded template *is* the rendered surface here — there is no generated
// copy that could go stale against it.
func servedSpawnPlanningAgentsSkill(t *testing.T) string {
	t.Helper()

	body, err := fs.ReadFile(templates.FS, "skills/skill_spawn-planning-agents.md")
	require.NoError(t, err)
	return string(body)
}

// spekKnowledgeAuditSection returns the body of the rendered skill's
// `# Intent: audit` section — from its heading up to the next top-level
// heading. Scoping assertions to the section is what makes the negative half
// of the "composes only existing primitives" guard meaningful: a stray
// `knowledge <verb>` belonging to another intent must not satisfy or break it.
func spekKnowledgeAuditSection(t *testing.T, rendered string) string {
	t.Helper()

	const heading = "# Intent: audit"
	start := strings.Index(rendered, heading)
	require.NotEqual(t, -1, start, "the rendered spek-knowledge skill has no %q section", heading)

	section := rendered[start+len(heading):]
	if end := strings.Index(section, "\n# "); end != -1 {
		section = section[:end]
	}
	return section
}

// knowledgeSubcommands is the closed set of verbs registered on the
// `knowledge` command by cmd/knowledge.go's AddCommand call. It is
// hand-maintained rather than read from that package: cmd imports
// internal/agent, so importing cmd from this in-package test would be an
// import cycle. Keep it in step with cmd/knowledge.go.
var knowledgeSubcommands = map[string]bool{
	"search":         true,
	"read":           true,
	"list":           true,
	"write":          true,
	"delete":         true,
	"sources":        true,
	"conventions":    true,
	"categories":     true,
	"always-applied": true,
	"tags":           true,
}

// TestRenderedSpekKnowledgeAdvertisesAuditIntent asserts the audit intent is
// both present and reachable. Presence alone is not enough: an intent whose
// own preamble still tells the agent it has three branches would exist in the
// file and never be chosen, so the branch count and the `# When to invoke`
// trigger are guarded alongside the heading itself.
func TestRenderedSpekKnowledgeAdvertisesAuditIntent(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	require.Contains(t, rendered, "# Intent: audit",
		"the rendered spek-knowledge skill must carry an audit intent section")
	require.Contains(t, rendered, "picks one of five branches (lookup / contribute / update / audit / maintenance)",
		"the preamble must advertise five branches, or the audit intent is never reached")
	require.Contains(t, rendered, "One skill handles all five intents",
		"`# When to invoke` must say one skill handles all five intents")
	require.Contains(t, rendered, "Are our knowledge entries tagged properly?",
		"`# When to invoke` must carry a natural-language trigger for the audit")

	// Every superseded branch-count phrasing must be gone, not merely joined
	// by the current one. Each count this preamble has ever advertised is
	// banned by name, so a later intent cannot leave a stale sentence behind.
	for _, stale := range []string{
		"one of three branches", "all three intents",
		"one of four branches", "all four intents",
	} {
		require.NotContains(t, rendered, stale,
			"the rendered skill still advertises a superseded branch count (%q)", stale)
	}
}

// TestRenderedSpekKnowledgeAuditReportsUnsupportedAndMissingTags asserts the
// audit's two reported outcomes survive: a tag the content does not bear out
// is reported for removal, and a subject the entry is clearly about but
// carries no tag for gets one proposed, preferring the vocabulary already in
// use over a freshly minted tag.
func TestRenderedSpekKnowledgeAuditReportsUnsupportedAndMissingTags(t *testing.T) {
	section := spekKnowledgeAuditSection(t, renderSpekKnowledgeSkill(t))

	auditReports := []struct {
		needle string
		why    string
	}{
		{
			needle: "Unsupported tags",
			why:    "the audit must name the unsupported-tag outcome",
		},
		{
			needle: "a tag the entry's content does not bear out",
			why:    "the audit must define an unsupported tag by its content",
		},
		{
			needle: "Missing tags",
			why:    "the audit must name the missing-tag outcome",
		},
		{
			needle: "a subject the entry is clearly about but carries no tag for",
			why:    "the audit must define a missing tag by the entry's subject",
		},
		{
			needle: "preferring a tag already in the vocabulary over a new one",
			why:    "a proposed tag must converge on the vocabulary rather than mint a variant",
		},
		{
			needle: "rules from the contribute intent unchanged",
			why:    "the audit must defer to the contribute intent's tag-form rules",
		},
		{
			needle: "do not restate or reinterpret them here",
			why:    "the tag-form rules must have exactly one home",
		},
	}
	for _, report := range auditReports {
		require.Contains(t, section, report.needle,
			"the audit intent lost a reported outcome (%s): %q", report.why, report.needle)
	}
}

// TestRenderedSpekKnowledgeAuditCarriesTagAuditFailureModes asserts the two
// auditing-specific failure modes survive, keyed on their worked examples.
//
// These two rules are opposite errors, and auditing is where both are most
// likely: an agent sweeping a whole store's tags at once is exactly the agent
// tempted to tidy `https` into `http`, and exactly the one that leaves
// `apples` sitting beside `apple` because each entry looked fine on its own.
// A future edit trimming either rule would leave the audit either collapsing
// distinct subjects into one tag or letting the vocabulary silently double.
func TestRenderedSpekKnowledgeAuditCarriesTagAuditFailureModes(t *testing.T) {
	section := spekKnowledgeAuditSection(t, renderSpekKnowledgeSkill(t))

	failureModes := []struct {
		needle string
		why    string
	}{
		{
			needle: "Do not over-merge",
			why:    "the audit must name the over-merging failure mode",
		},
		{
			needle: "An entry tagged `https` must not be told to use `http` instead",
			why:    "the https/http example is what stops an audit collapsing a distinct subject",
		},
		{
			needle: "prefix matching already relates them at reduced strength",
			why:    "the audit must say why https and http need not be merged",
		},
		{
			needle: "Do prune what prefix matching already reaches",
			why:    "the audit must name the redundant-tag failure mode",
		},
		{
			needle: "`apples` sitting beside `apple`",
			why:    "the apples/apple example is what makes the pruning rule concrete",
		},
		{
			needle: "should be reported as removable",
			why:    "a tag prefix matching already reaches must be reported, not left to accumulate",
		},
	}
	for _, mode := range failureModes {
		require.Contains(t, section, mode.needle,
			"the audit intent lost a tag-auditing failure mode (%s): %q", mode.why, mode.needle)
	}
}

// TestRenderedSpekKnowledgeAuditConfirmsPerEntry asserts the audit gates every
// write behind that entry's own confirmation, and that `# Decline handling`
// carries the per-entry rule — a decline on one entry does not carry to the
// next, and approval of one is never approval of another.
func TestRenderedSpekKnowledgeAuditConfirmsPerEntry(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)
	section := spekKnowledgeAuditSection(t, rendered)

	require.Contains(t, section, "Propose per entry, and confirm per entry",
		"the audit must propose and confirm one entry at a time")
	require.Contains(t, section, "Wait for **explicit confirmation for that entry**",
		"the audit must require confirmation for the entry being changed")
	require.Contains(t, section, "Only after explicit confirmation for that entry",
		"no entry may be written before its own confirmation")
	require.Contains(t, section, "Accepting one entry's changes never applies another's",
		"accepting one entry must not silently apply another's changes")
	require.Contains(t, section, "at the entry's **original** tier, name and path",
		"an audited entry must be written back where it came from")
	require.Contains(t, section, "is a bug in the skill's execution",
		"reaching a write without the per-entry gate must be called a bug")

	// The per-entry carve-out lives in `# Decline handling`, outside the audit
	// section, so it is asserted against the whole rendered skill.
	require.Contains(t, rendered, "In the audit and maintenance intents the same rule applies **per entry**",
		"decline handling must scope the audit's and maintenance's declines per entry")
	require.Contains(t, rendered, "A decline on one entry stops that entry's change and nothing else",
		"a decline on one entry must not stop the whole review")
	require.Contains(t, rendered, "never treat approval of an earlier entry as approval of a later one",
		"approval of one entry must never carry to a later entry")
}

// TestRenderedSpekKnowledgeAuditComposesExistingPrimitives asserts the audit
// is built from the CRUD surface the other intents already use — `knowledge
// list`, `knowledge tags`, `knowledge read` and the confirmed `knowledge
// write` — and introduces no new command of its own. The negative half scans
// every `<command> knowledge <verb>` invocation in the audit section and
// requires each verb to be one cmd/knowledge.go actually registers, so an
// audit prescribing an invented bulk-retag command fails here rather than
// shipping instructions for a CLI that does not exist.
func TestRenderedSpekKnowledgeAuditComposesExistingPrimitives(t *testing.T) {
	command := config.NewDefault().Command
	section := spekKnowledgeAuditSection(t, renderSpekKnowledgeSkill(t))

	for _, primitive := range []string{"list", "tags", "read", "write"} {
		require.Contains(t, section, command+" knowledge "+primitive,
			"the audit intent must compose the existing `knowledge %s` primitive", primitive)
	}
	require.Contains(t, section, "adds no new command",
		"the audit must state that it introduces no new command")

	invocation := regexp.MustCompile(regexp.QuoteMeta(command) + ` knowledge ([a-z][a-z-]*)`)
	matches := invocation.FindAllStringSubmatch(section, -1)
	require.NotEmpty(t, matches, "the audit intent invokes no knowledge command at all")
	for _, match := range matches {
		require.True(t, knowledgeSubcommands[match[1]],
			"the audit intent invokes %q, which cmd/knowledge.go does not register as a knowledge subcommand", match[1])
	}
}

// spekKnowledgeMaintenanceSection returns the body of the rendered skill's
// `# Intent: maintenance` section — from its heading up to the next top-level
// heading. It mirrors spekKnowledgeAuditSection exactly; scoping assertions to
// the section is what keeps the maintenance guards honest, since the audit
// intent immediately above it uses much of the same vocabulary (per-entry
// confirmation, `knowledge list`, `knowledge read`) and would otherwise satisfy
// them from outside.
func spekKnowledgeMaintenanceSection(t *testing.T, rendered string) string {
	t.Helper()

	const heading = "# Intent: maintenance"
	start := strings.Index(rendered, heading)
	require.NotEqual(t, -1, start, "the rendered spek-knowledge skill has no %q section", heading)

	section := rendered[start+len(heading):]
	if end := strings.Index(section, "\n# "); end != -1 {
		section = section[:end]
	}
	return section
}

// TestSpekKnowledgeSectionSlicingSeparatesAuditAndMaintenance asserts the two
// section helpers each return their own intent and nothing else.
//
// This is not a tautology about string slicing: the audit section used to be
// the last `# Intent:` heading in the file and so ran to the end of the
// document. A new heading now follows it, and every pre-existing audit
// assertion silently depends on the slice still stopping where it should. If
// the boundary were wrong in either direction — audit swallowing maintenance,
// or maintenance starting mid-audit — the two families of tests would pass
// while guarding the wrong prose, so the boundary is asserted rather than
// assumed.
func TestSpekKnowledgeSectionSlicingSeparatesAuditAndMaintenance(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	audit := spekKnowledgeAuditSection(t, rendered)
	maintenance := spekKnowledgeMaintenanceSection(t, rendered)

	// The audit section still holds its own prose, front to back: its opening
	// read-and-propose claim, its middle failure modes, and its closing
	// bug-in-execution sentence, which names only the two intents that predate
	// it.
	require.Contains(t, audit, "The audit **reads and proposes only**",
		"the audit section lost its opening after another heading was added below it")
	require.Contains(t, audit, "Unsupported tags",
		"the audit section lost its middle after another heading was added below it")
	require.Contains(t, audit, "exactly as it would be in the contribute and update intents",
		"the audit section lost its closing sentence, so the slice stops too early")

	// ...and none of the maintenance intent's.
	require.NotContains(t, audit, "# Intent: maintenance",
		"the audit slice must stop at the next top-level heading")
	require.NotContains(t, audit, "unverifiable",
		"the audit slice must not reach into the maintenance intent's verdicts")

	// The maintenance section holds its own prose and neither neighbour's.
	require.Contains(t, maintenance, "Maintenance **reads and proposes only**",
		"the maintenance slice must start at its own heading")
	require.NotContains(t, maintenance, "Unsupported tags",
		"the maintenance slice must not reach back into the audit intent")
	require.NotContains(t, maintenance, "# Decline handling",
		"the maintenance slice must stop at the next top-level heading")
}

// TestRenderedSpekKnowledgeAdvertisesMaintenanceIntent asserts the maintenance
// intent is both present and reachable. An intent the preamble never mentions
// exists in the file and is never chosen, so the natural-language triggers and
// the "what this skill does" summary are guarded alongside the heading itself.
// The branch-count contract is already guarded by the audit intent's test and
// is deliberately not repeated here.
func TestRenderedSpekKnowledgeAdvertisesMaintenanceIntent(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)

	require.Contains(t, rendered, "# Intent: maintenance",
		"the rendered spek-knowledge skill must carry a maintenance intent section")
	require.Contains(t, rendered, "read, contribute, update, audit, and maintenance operations",
		"`# What this skill does` must list maintenance among the operations the skill orchestrates")

	for _, trigger := range []string{
		"Is the knowledge base still accurate?",
		"Are these entries still true?",
		"Review the knowledge base for anything out of date.",
	} {
		require.Contains(t, rendered, trigger,
			"`# When to invoke` must carry the natural-language maintenance trigger %q", trigger)
	}

	// The intent must distinguish itself from the audit sitting directly above
	// it, or an agent handed "review the knowledge base" picks the wrong branch.
	section := spekKnowledgeMaintenanceSection(t, rendered)
	require.Contains(t, section, "not whether it is well labelled, which is the audit intent one section above",
		"the maintenance intent must distinguish itself from the audit intent")
}

// TestRenderedSpekKnowledgeMaintenanceCarriesFourVerdicts asserts all four
// verdicts survive with their definitions. Three of the four are what stop the
// review collapsing into a binary keep/delete sweep: without `unverifiable` in
// particular an agent has nowhere to put a claim it cannot check from the code,
// and the pressure is to call it stale and propose deleting it.
func TestRenderedSpekKnowledgeMaintenanceCarriesFourVerdicts(t *testing.T) {
	section := spekKnowledgeMaintenanceSection(t, renderSpekKnowledgeSkill(t))

	require.Contains(t, section, "Classify each entry as exactly one of four verdicts",
		"the maintenance intent must classify each entry into exactly one of four verdicts")

	verdicts := []struct {
		needle string
		why    string
	}{
		{
			needle: "**current** — the entry still describes how the project works",
			why:    "the current verdict must be defined by the entry still describing the project",
		},
		{
			needle: "**stale** — the entry's subject no longer exists",
			why:    "the stale verdict must be defined by the subject's disappearance",
		},
		{
			needle: "**incorrect** — the subject still exists but the entry describes it wrongly",
			why:    "the incorrect verdict must be defined by a surviving subject described wrongly",
		},
		{
			needle: "**unverifiable** — the entry cannot be checked from the code",
			why:    "the unverifiable verdict must be defined by the entry being uncheckable",
		},
	}
	for _, verdict := range verdicts {
		require.Contains(t, section, verdict.needle,
			"the maintenance intent lost a verdict (%s): %q", verdict.why, verdict.needle)
	}
}

// TestRenderedSpekKnowledgeMaintenanceCarriesClassificationRule asserts the
// single rule that decides whether this review helps or harms, together with
// its worked failure mode.
//
// The rule is the whole safety property of the feature. This project's
// knowledge entries state the target and the code is what has yet to meet it,
// so an agent applying the naive "entry disagrees with code, therefore entry is
// stale" heuristic would propose deleting precisely the entries doing the most
// work. The worked example is asserted alongside the rule because the abstract
// statement on its own has repeatedly failed to stop that inference.
func TestRenderedSpekKnowledgeMaintenanceCarriesClassificationRule(t *testing.T) {
	section := spekKnowledgeMaintenanceSection(t, renderSpekKnowledgeSkill(t))

	rule := []struct {
		needle string
		why    string
	}{
		{
			needle: "An entry stating a standard the code has not yet met is **current**, not stale",
			why:    "an unmet standard must be classified current, never stale",
		},
		{
			needle: "an entry states the target and the code is what has yet to meet it",
			why:    "the rule must say which of the entry and the code states the target",
		},
		{
			needle: "Only an entry whose **subject no longer exists** is stale",
			why:    "stale must be bounded to a subject that is genuinely gone",
		},
		{
			needle: "an architecture entry says every store write goes through the CLI",
			why:    "the worked failure mode is what makes the rule concrete",
		},
		{
			needle: "Proposing its removal would delete the entry doing the most work",
			why:    "the example must name the consequence of getting the rule wrong",
		},
		{
			needle: "Say the code disagrees with it, and leave the entry alone",
			why:    "the example must say what to do instead of proposing removal",
		},
	}
	for _, part := range rule {
		require.Contains(t, section, part.needle,
			"the maintenance intent lost part of the classification rule (%s): %q", part.why, part.needle)
	}
}

// TestRenderedSpekKnowledgeMaintenanceRequiresEvidence asserts every stale or
// incorrect verdict has to name what actually changed, and that a verdict
// without evidence falls back to `unverifiable` rather than to stale. Without
// this the review degenerates into vibes about which entries look old, which is
// unreviewable by the user and exactly the failure that makes an automated
// knowledge sweep dangerous.
func TestRenderedSpekKnowledgeMaintenanceRequiresEvidence(t *testing.T) {
	section := spekKnowledgeMaintenanceSection(t, renderSpekKnowledgeSkill(t))

	for _, needle := range []string{
		"State the evidence for every stale or incorrect verdict",
		"Name the specific file, command or behaviour that changed",
		"so the user can check the finding rather than take it on trust",
		"A verdict you cannot attach evidence to is **unverifiable**, not stale",
		"never classify from age or tone",
		`"Looks old", "seems outdated" and "probably superseded" are not findings`,
	} {
		require.Contains(t, section, needle,
			"the maintenance intent lost part of the evidence requirement: %q", needle)
	}
}

// TestRenderedSpekKnowledgeMaintenanceReportsCategoryDrift asserts the drifted
// category-description step survives with the commands it composes and, more
// importantly, with its read-only boundary. Repairing drift means regenerating
// a store's category READMEs, which is a destructive rewrite the user runs
// deliberately; a review that silently did it would overwrite local edits on
// the strength of nothing but having noticed a mismatch.
func TestRenderedSpekKnowledgeMaintenanceReportsCategoryDrift(t *testing.T) {
	command := config.NewDefault().Command
	section := spekKnowledgeMaintenanceSection(t, renderSpekKnowledgeSkill(t))

	require.Contains(t, section, "Report drifted category descriptions",
		"the maintenance intent must carry the category-drift step")
	require.Contains(t, section, "generated from the project's definition of that category",
		"the drift step must say where a category README comes from")

	for _, verb := range []string{"list", "read", "categories"} {
		require.Contains(t, section, command+" knowledge "+verb,
			"the drift step must compose `knowledge %s`", verb)
	}
	require.Contains(t, section, command+" init <agent>",
		"the drift step must name `init <agent>` as the remedy")

	require.Contains(t, section, "reports drift and never repairs it",
		"the drift step must state that the review never repairs drift itself")
	require.Contains(t, section, "stays something the user runs deliberately",
		"the drift step must say why repair is left to the user")
}

// TestRenderedSpekKnowledgeMaintenanceConfirmsPerEntry asserts the review gates
// every write *and every delete* behind that entry's own confirmation, and that
// `# Decline handling` carries the removal-specific carve-out.
//
// The generic per-entry rules shared with the audit intent are already guarded
// by TestRenderedSpekKnowledgeAuditConfirmsPerEntry and are not repeated here.
// What is asserted below is what maintenance adds: it is the first intent that
// can *remove* an entry rather than only rewrite one, so a decline has to mean
// nothing is deleted, not merely that nothing is written.
func TestRenderedSpekKnowledgeMaintenanceConfirmsPerEntry(t *testing.T) {
	rendered := renderSpekKnowledgeSkill(t)
	section := spekKnowledgeMaintenanceSection(t, rendered)

	require.Contains(t, section, "Propose per entry, and confirm per entry",
		"the maintenance review must propose and confirm one entry at a time")
	require.Contains(t, section, "verdict and the evidence for it",
		"each proposal must show the verdict and its evidence")
	require.Contains(t, section, "leave it, correct it, or remove it",
		"each proposal must name the three outcomes the user is choosing between")
	require.Contains(t, section, "Wait for **explicit confirmation for that entry**",
		"the maintenance review must require confirmation for the entry being changed")
	require.Contains(t, section, "Only after explicit confirmation for that entry",
		"no entry may be written or deleted before its own confirmation")
	require.Contains(t, section, "Accepting one entry's outcome never applies another's",
		"accepting one entry must not silently apply another's outcome")
	require.Contains(t, section, "at the entry's **original** tier, name and path",
		"a corrected entry must be written back where it came from")
	require.Contains(t, section, "delete it through the tool and never with your own file tools",
		"a removal must go through the CLI rather than a raw file operation")
	require.Contains(t, section, "is a bug in the skill's execution",
		"reaching a write or delete without the per-entry gate must be called a bug")

	// The decline carve-out lives in `# Decline handling`, outside the section,
	// so it is asserted against the whole rendered skill.
	require.Contains(t, rendered, "Maintenance is the first intent that can **remove** an entry",
		"decline handling must call out that maintenance can remove an entry")
	require.Contains(t, rendered, "nothing is written and, above all, nothing is deleted",
		"a decline in maintenance must delete nothing")
	require.Contains(t, rendered, "agreement to remove one entry is never agreement to remove another",
		"agreement to remove one entry must never carry to another")
}

// TestRenderedSpekKnowledgeMaintenanceComposesExistingPrimitives asserts the
// maintenance review is built from the CRUD surface the other intents already
// use — `knowledge list`, `knowledge read`, `knowledge categories`, the
// confirmed `knowledge write`, and the one verb it adds, `knowledge delete` —
// and invents nothing. The negative half scans every `<command> knowledge
// <verb>` invocation in the section and requires each verb to be one
// cmd/knowledge.go actually registers, so a review prescribing an invented bulk
// prune fails here rather than shipping instructions for a CLI that does not
// exist.
func TestRenderedSpekKnowledgeMaintenanceComposesExistingPrimitives(t *testing.T) {
	command := config.NewDefault().Command
	section := spekKnowledgeMaintenanceSection(t, renderSpekKnowledgeSkill(t))

	for _, primitive := range []string{"list", "read", "categories", "write", "delete"} {
		require.Contains(t, section, command+" knowledge "+primitive,
			"the maintenance intent must compose the existing `knowledge %s` primitive", primitive)
	}
	require.Contains(t, section, "adds no bulk operation, no recursive operation and no second write path",
		"the maintenance intent must state the operations it does not introduce")

	invocation := regexp.MustCompile(regexp.QuoteMeta(command) + ` knowledge ([a-z][a-z-]*)`)
	matches := invocation.FindAllStringSubmatch(section, -1)
	require.NotEmpty(t, matches, "the maintenance intent invokes no knowledge command at all")
	for _, match := range matches {
		require.True(t, knowledgeSubcommands[match[1]],
			"the maintenance intent invokes %q, which cmd/knowledge.go does not register as a knowledge subcommand", match[1])
	}
}

// managedStoreRemovalTargets are the store directories a raw removal must never
// be aimed at in agent-facing prose. They are the default on-disk locations of
// the spec, plan, knowledge, changelog and design stores; a store relocated by
// config is reached by the same CLI verbs, so banning the default spellings is
// what catches an instruction being written in the first place.
//
// The scratch surfaces — `.spektacular/tmp/`, `.spektacular/work/` and
// `.spektacular/working-context.md` — are deliberately absent. They are the
// agent's own to create and delete, and several intents legitimately end by
// removing a staged file.
var managedStoreRemovalTargets = []string{
	".spektacular/specs",
	".spektacular/plans",
	".spektacular/knowledge",
	".spektacular/changelog",
	".spektacular/design",
}

// rawRemovalVerbs are the shell spellings of "delete this file yourself".
//
// A bare `rm ` is deliberately *not* banned on its own: every `rm` currently in
// the template corpus targets `.spektacular/tmp/`, and those lines are correct.
// Each verb is therefore only ever matched joined to a managed store path.
var rawRemovalVerbs = []string{"rm", "rm -f", "rm -r", "rm -rf", "unlink"}

// rawGoRemovalCalls are Go-level removals spelled out in instruction prose.
// Neither appears anywhere in the corpus today, and neither can false-positive
// in English, so they are matched as plain literals.
var rawGoRemovalCalls = []string{"os.Remove(", "os.RemoveAll("}

// rawRemovalPatterns is the compiled ban list. Each verb/target pair is
// anchored on a non-letter so `confirm .spektacular/specs/...` — which ends in
// the letters `rm` followed by a space and a store path — is not read as an
// `rm` instruction.
var rawRemovalPatterns = compileRawRemovalPatterns()

func compileRawRemovalPatterns() []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, verb := range rawRemovalVerbs {
		for _, target := range managedStoreRemovalTargets {
			patterns = append(patterns, regexp.MustCompile(`(^|[^A-Za-z])`+regexp.QuoteMeta(verb+" "+target)))
		}
	}
	for _, call := range rawGoRemovalCalls {
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(call)))
	}
	return patterns
}

// rawManagedFileRemovalsIn returns every raw-removal instruction found in body,
// as the matched text. An empty result means the body instructs no raw removal
// of a managed file.
func rawManagedFileRemovalsIn(body string) []string {
	var found []string
	for _, pattern := range rawRemovalPatterns {
		if match := pattern.FindString(body); match != "" {
			found = append(found, strings.TrimSpace(match))
		}
	}
	return found
}

// scratchRemovalInstruction is the removal that must stay legal. Pinning it is
// what proves the sweep below is discriminating rather than simply matching
// nothing: if a future refactor made the ban patterns unsatisfiable, this
// assertion would still fail loudly when the legitimate line disappeared with
// them.
const scratchRemovalInstruction = "rm .spektacular/tmp/"

// TestRawManagedFileRemovalPredicateFires is the mutation check for the two
// corpus sweeps below. Those sweeps can only ever assert an absence, so on
// their own they would pass just as happily against a predicate that never
// matched anything. This exercises the predicate directly against synthetic
// offending and legitimate prose, without touching a template.
func TestRawManagedFileRemovalPredicateFires(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		offends bool
	}{
		{
			name:    "rm on a knowledge entry",
			body:    "Remove the entry with `rm .spektacular/knowledge/gotchas/stale.md`.",
			offends: true,
		},
		{
			name:    "rm -rf on the spec store",
			body:    "Clean up afterwards: rm -rf .spektacular/specs/000001_thing",
			offends: true,
		},
		{
			name:    "rm on the plan store at the start of a line",
			body:    "rm .spektacular/plans/000001_thing/plan.md\n",
			offends: true,
		},
		{
			name:    "unlink on a design document",
			body:    "Drop it with unlink .spektacular/design/api-shape.md when done.",
			offends: true,
		},
		{
			name:    "rm on a changelog record",
			body:    "rm .spektacular/changelog/000001.md",
			offends: true,
		},
		{
			name:    "a Go-level removal in prose",
			body:    "Call os.Remove(path) to drop the record.",
			offends: true,
		},
		{
			name:    "removing a staged scratch file is allowed",
			body:    "Remove the scratch file after a successful write: `rm .spektacular/tmp/<slug>.md`.",
			offends: false,
		},
		{
			name:    "removing a workflow working directory is allowed",
			body:    "rm -rf .spektacular/work/000001_thing",
			offends: false,
		},
		{
			name:    "a word merely ending in rm is not a removal verb",
			body:    "Afterwards confirm .spektacular/specs/000001.md still reads correctly.",
			offends: false,
		},
		{
			name:    "the CLI delete verb is allowed",
			body:    "spektacular knowledge delete --data '{\"tier\":\"project\"}'",
			offends: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := rawManagedFileRemovalsIn(tc.body)
			if tc.offends {
				require.NotEmpty(t, found,
					"the raw-removal sweep would not fire on %q", tc.body)
				return
			}
			require.Empty(t, found,
				"the raw-removal sweep false-positives on legitimate prose %q", tc.body)
		})
	}
}

// TestEmbeddedTemplatesNeverRemoveManagedFilesDirectly walks the embedded
// templates under skills/workflows/ and steps/ and asserts no instruction tells
// an agent to remove a file a store owns with a raw file operation. Removal is
// a CLI verb; going around it leaves a spec pointing at a design that is not
// there, and stops working outright the moment a store is backed by something
// other than a local directory.
func TestEmbeddedTemplatesNeverRemoveManagedFilesDirectly(t *testing.T) {
	scratchRemovalSeen := false

	for _, root := range []string{"skills/workflows", "steps"} {
		err := fs.WalkDir(templates.FS, root, func(path string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			body, err := fs.ReadFile(templates.FS, path)
			require.NoError(t, err)

			require.Emptyf(t, rawManagedFileRemovalsIn(string(body)),
				"%s instructs a raw removal of a managed file; removal is a CLI verb", path)

			if strings.Contains(string(body), scratchRemovalInstruction) {
				scratchRemovalSeen = true
			}
			return nil
		})
		require.NoError(t, err)
	}

	require.True(t, scratchRemovalSeen,
		"no template removes a staged scratch file any more, so the sweep above is no longer discriminating between a store path and %q", scratchRemovalInstruction)
}

// TestRenderedSkillsNeverRemoveManagedFilesDirectly is the same sweep against
// the surface an agent is actually handed: every workflow skill rendered
// through the production install path into a directory the test owns. A
// template can be clean while a partial or a placeholder substitution
// reintroduces the instruction, and the rendered copy is what ships.
func TestRenderedSkillsNeverRemoveManagedFilesDirectly(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, installWorkflowSkills(tmp, ".claude/skills", config.NewDefault(), io.Discard))

	scratchRemovalSeen := false
	root := filepath.Join(tmp, ".claude", "skills")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		require.NoError(t, err)

		require.Emptyf(t, rawManagedFileRemovalsIn(string(body)),
			"%s instructs a raw removal of a managed file; removal is a CLI verb", path)

		if strings.Contains(string(body), scratchRemovalInstruction) {
			scratchRemovalSeen = true
		}
		return nil
	})
	require.NoError(t, err)

	require.True(t, scratchRemovalSeen,
		"no rendered skill removes a staged scratch file any more, so the sweep above is no longer discriminating between a store path and %q", scratchRemovalInstruction)
}

func assertNoStaleRetrievalClaim(t *testing.T, surface, body string) {
	t.Helper()
	for _, needle := range staleBooleanRetrievalClaims {
		require.NotContains(t, body, needle,
			"%s reintroduced the superseded boolean retrieval rule %q", surface, needle)
	}
}

func assertNoForbiddenSubstring(t *testing.T, path, body string) {
	t.Helper()
	for _, needle := range forbiddenInstructionSubstrings {
		require.NotContains(t, body, needle, "%s contains forbidden instruction-surface pattern %q", path, needle)
	}
}
