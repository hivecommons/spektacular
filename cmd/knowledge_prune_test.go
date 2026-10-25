package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// prunedAddress is the tier, store and path of one entry in prune's output.
type prunedAddress struct {
	Tier string `json:"tier"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (a prunedAddress) key() string { return a.Name + ":" + a.Path }

// pruneRemovalRow mirrors one item of a prune result's "removed".
type pruneRemovalRow struct {
	prunedAddress
	Created       string  `json:"created"`
	TrackedSince  string  `json:"tracked_since"`
	LastRetrieval *string `json:"last_retrieval"`
	Discoveries   int     `json:"discoveries"`
}

// pruneReportRow mirrors one row of the final report's "entries".
type pruneReportRow struct {
	prunedAddress
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence"`
}

// pruneOutput mirrors every field `knowledge prune` and `knowledge prune
// verdict` may return: an entry to check, or the final report.
type pruneOutput struct {
	Removed     []pruneRemovalRow `json:"removed"`
	Remaining   int               `json:"remaining"`
	Entry       *prunedAddress    `json:"entry"`
	Instruction string            `json:"instruction"`
	Resumed     bool              `json:"resumed"`
	DryRun      bool              `json:"dry_run"`
	MaxAgeDays  int               `json:"max_age_days"`
	Entries     []pruneReportRow  `json:"entries"`
	Counts      map[string]int    `json:"counts"`
	Complete    bool              `json:"complete"`
}

// pruneTestNow is the fixed time every prune fixture is aged against.
var pruneTestNow = time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)

func daysAgo(d int) time.Time { return pruneTestNow.Add(-time.Duration(d) * 24 * time.Hour) }

// pruneProject is twoScopeProject with a 30-day prune age, more entries, and a
// usage index seeded in each store so every entry has a chosen age and usage.
// Both clocks are pinned to pruneTestNow. Against that age:
//
//   - repo testproj learnings/stale.md: 40 days old, never used: removed
//   - repo testproj learnings/surfaced.md: 40 days old, 4 discoveries, never read: removed
//   - project team old-team.md: 50 days old, never used: removed
//   - repo testproj readme.md: 40 days old, read 5 days ago: kept
//   - repo testproj architecture/initial-idea.md: 10 days old: kept
//   - repo testproj learnings/backfilled.md: dated 400 days ago, tracked 10: kept
//   - project team guidelines.md: 40 days old, read 3 days ago: kept
//
// gotchas/README.md is a category description, never an entry.
func pruneProject(t *testing.T) (root, repoLoc, teamLoc string) {
	t.Helper()
	root, repoLoc, teamLoc = twoScopeProject(t)
	cfg := "name: testproj\n" +
		"agent: claude\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: .\n" +
		"knowledge:\n" +
		"  sources:\n" +
		"    - name: team\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: " + teamLoc + "\n" +
		"  prune:\n" +
		"    max_age_days: 30\n"
	writeCurrentConfig(t, root, cfg)

	seedKnowledgeFile(t, repoLoc, "learnings/stale.md", "an old note nobody reads\n")
	seedKnowledgeFile(t, repoLoc, "learnings/surfaced.md", "a note searches keep finding\n")
	seedKnowledgeFile(t, repoLoc, "learnings/backfilled.md", "# Backfilled\n\nthe build runs make all\n")
	seedKnowledgeFile(t, repoLoc, "gotchas/README.md", "gotchas are traps\n")
	seedKnowledgeFile(t, teamLoc, "old-team.md", "a team note nobody reads\n")
	seedKnowledgeFile(t, teamLoc, "guidelines.md", "Every command publishes a --schema.\n")

	at := func(d int) *time.Time { v := daysAgo(d); return &v }
	rec := func(created, tracked int, disc, retr knowledge.UsageLog) *knowledge.UsageRecord {
		return &knowledge.UsageRecord{Created: daysAgo(created), TrackedSince: daysAgo(tracked), Discoveries: disc, Retrievals: retr}
	}
	none := knowledge.UsageLog{}
	seedUsageIndex(t, repoLoc, map[string]*knowledge.UsageRecord{
		"readme.md":                    rec(40, 40, none, knowledge.UsageLog{Count: 1, Last: at(5)}),
		"architecture/initial-idea.md": rec(10, 10, none, none),
		"learnings/stale.md":           rec(40, 40, none, none),
		"learnings/surfaced.md":        rec(40, 40, knowledge.UsageLog{Count: 4, Last: at(1)}, none),
		"learnings/backfilled.md":      rec(400, 10, knowledge.UsageLog{Count: 2, Last: at(2)}, knowledge.UsageLog{Count: 1, Last: at(8)}),
	})
	seedUsageIndex(t, teamLoc, map[string]*knowledge.UsageRecord{
		"guidelines.md": rec(40, 40, none, knowledge.UsageLog{Count: 3, Last: at(3)}),
		"old-team.md":   rec(50, 50, none, none),
	})

	fixKnowledgeClock(t, pruneTestNow)
	old := pruneNow
	pruneNow = func() time.Time { return pruneTestNow }
	t.Cleanup(func() { pruneNow = old })
	return root, repoLoc, teamLoc
}

// seedUsageIndex writes a store's usage index directly on disk.
func seedUsageIndex(t *testing.T, loc string, entries map[string]*knowledge.UsageRecord) {
	t.Helper()
	raw, err := json.MarshalIndent(knowledge.UsageIndex{Version: 1, Entries: entries}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(loc, knowledge.UsageIndexFile), append(raw, '\n'), 0o644))
}

// The removals the fixture's age rule makes, and the entries left to check.
var (
	pruneFixtureRemoved   = []string{"testproj:learnings/stale.md", "testproj:learnings/surfaced.md", "team:old-team.md"}
	pruneFixtureRemaining = []string{"testproj:readme.md", "testproj:architecture/initial-idea.md", "testproj:learnings/backfilled.md", "team:guidelines.md"}
)

// runPrune runs `knowledge prune` with args and decodes its result.
func runPrune(t *testing.T, args ...string) pruneOutput {
	t.Helper()
	stdout, _, err := runKnowledge(t, append([]string{"prune"}, args...)...)
	require.NoError(t, err)
	var res pruneOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	return res
}

// giveVerdict runs `knowledge prune verdict` for addr.
func giveVerdict(t *testing.T, addr prunedAddress, verdict, evidence string) (pruneOutput, error) {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"tier": addr.Tier, "name": addr.Name, "path": addr.Path, "verdict": verdict, "evidence": evidence,
	})
	require.NoError(t, err)
	stdout, _, err := runKnowledge(t, "prune", "verdict", "--data", string(data))
	if err != nil {
		return pruneOutput{}, err
	}
	var res pruneOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	return res, nil
}

// requirePruneRefusal asserts err is a refusal with the given code and a
// remediation, and returns it.
func requirePruneRefusal(t *testing.T, err error, code string) *output.ErrorResponse {
	t.Helper()
	var refusal *output.ErrorResponse
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, code, refusal.Code, "message: %s", refusal.Message)
	require.NotEmpty(t, refusal.NextAction, "every refusal carries a remediation")
	return refusal
}

// pruneLoop answers every entry prune hands out, starting from first, with
// decide's verdict and evidence (after any change decide makes), and returns
// the final report and the keys of the entries handed out, in order.
func pruneLoop(t *testing.T, first pruneOutput, decide func(res pruneOutput) (verdict, evidence string)) (pruneOutput, []string) {
	t.Helper()
	res := first
	var seen []string
	for !res.Complete {
		require.NotNil(t, res.Entry, "an unfinished prune hands out an entry")
		require.Less(t, len(seen), 20, "prune never finishes")
		seen = append(seen, res.Entry.key())
		verdict, evidence := decide(res)
		var err error
		res, err = giveVerdict(t, *res.Entry, verdict, evidence)
		require.NoError(t, err, "verdict %s for %s", verdict, seen[len(seen)-1])
	}
	return res, seen
}

func removalKeys(rows []pruneRemovalRow) []string {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.key())
	}
	return keys
}

func reportVerdicts(rows []pruneReportRow) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.key()] = r.Verdict
	}
	return out
}

// hashStore hashes every file under dir, keyed by its relative path.
func hashStore(t *testing.T, dir string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		sums[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	}))
	return sums
}

// indexPaths returns the paths a store's usage index holds a record for.
func indexPaths(t *testing.T, loc string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(loc, knowledge.UsageIndexFile))
	require.NoError(t, err)
	var idx knowledge.UsageIndex
	require.NoError(t, json.Unmarshal(raw, &idx))
	paths := map[string]bool{}
	for p := range idx.Entries {
		paths[p] = true
	}
	return paths
}

func pruneRunFilePath(root string) string {
	return filepath.Join(root, ".spektacular", pruneRunFile)
}

// Prune criteria 1 and 2: with a 30-day age, the first pass removes exactly
// the entries created and tracked for longer than that and not retrieved
// within it, including one searches surfaced but nobody read, with their
// files and their index records; an entry read in the window, a young entry
// and one backfilled with an old date but tracked for less than the age are
// kept and handed on to be checked, and a category description is untouched.
func TestKnowledgePrune_AgeRuleRemovesOldUnreadEntries(t *testing.T) {
	root, repoLoc, teamLoc := pruneProject(t)

	res := runPrune(t)
	require.False(t, res.Complete)
	require.False(t, res.DryRun)
	require.Equal(t, 30, res.MaxAgeDays)
	require.ElementsMatch(t, pruneFixtureRemoved, removalKeys(res.Removed))
	for _, r := range res.Removed {
		require.Nil(t, r.LastRetrieval, r.key())
		requireUsageTime(t, daysAgo(map[string]int{"testproj:learnings/stale.md": 40, "testproj:learnings/surfaced.md": 40, "team:old-team.md": 50}[r.key()]), r.Created)
		if r.key() == "testproj:learnings/surfaced.md" {
			require.Equal(t, 4, r.Discoveries, "the removal carries the discoveries that did not save it")
		} else {
			require.Zero(t, r.Discoveries, r.key())
		}
	}
	require.Equal(t, len(pruneFixtureRemaining), res.Remaining)
	require.NotNil(t, res.Entry)
	require.Contains(t, pruneFixtureRemaining, res.Entry.key())
	require.FileExists(t, pruneRunFilePath(root))

	for _, p := range []string{"learnings/stale.md", "learnings/surfaced.md"} {
		require.NoFileExists(t, filepath.Join(repoLoc, filepath.FromSlash(p)))
	}
	require.NoFileExists(t, filepath.Join(teamLoc, "old-team.md"))
	for _, p := range []string{"readme.md", "architecture/initial-idea.md", "learnings/backfilled.md", "gotchas/README.md"} {
		require.FileExists(t, filepath.Join(repoLoc, filepath.FromSlash(p)))
	}
	require.FileExists(t, filepath.Join(teamLoc, "guidelines.md"))

	repoIdx := indexPaths(t, repoLoc)
	require.False(t, repoIdx["learnings/stale.md"], "a removed entry leaves no record")
	require.False(t, repoIdx["learnings/surfaced.md"], "a removed entry leaves no record")
	require.True(t, repoIdx["readme.md"])
	require.True(t, repoIdx["learnings/backfilled.md"])
	teamIdx := indexPaths(t, teamLoc)
	require.False(t, teamIdx["old-team.md"], "a removed entry leaves no record")
	require.True(t, teamIdx["guidelines.md"])
}

// Prune criteria 3 and 4: the agent loop. Every claim the store contradicts,
// a verdict for an entry other than the one being checked, a change without
// evidence and a malformed verdict are refused with a remediation, leaving the
// entry at the head of the queue. A corrected entry keeps its usage counts, a
// deleted entry leaves no record in the index, an entry stating a standard
// the code has not yet met is kept current byte for byte, and the final
// report lists every entry prune accounted for before the run file is
// removed.
func TestKnowledgePrune_VerdictLoopChecksEveryClaimAgainstTheStore(t *testing.T) {
	root, repoLoc, teamLoc := pruneProject(t)
	guidelinesBefore, err := os.ReadFile(filepath.Join(teamLoc, "guidelines.md"))
	require.NoError(t, err)

	first := runPrune(t)
	head := *first.Entry
	require.Contains(t, first.Instruction, "knowledge prune verdict")
	require.Contains(t, first.Instruction, `"path":"`+head.Path+`"`)
	require.Contains(t, first.Instruction, "knowledge write --data")
	require.Contains(t, first.Instruction, "knowledge delete --data")
	headFile := filepath.Join(map[string]string{"testproj": repoLoc, "team": teamLoc}[head.Name], filepath.FromSlash(head.Path))
	headBytes, err := os.ReadFile(headFile)
	require.NoError(t, err)
	require.Contains(t, first.Instruction, string(headBytes[:len(headBytes)-1]), "the entry's text is inlined")

	// A verdict for an entry other than the head.
	var other prunedAddress
	for _, k := range pruneFixtureRemaining {
		if k != head.key() {
			name, path, _ := strings.Cut(k, ":")
			other = prunedAddress{Tier: map[string]string{"testproj": "repo", "team": "project"}[name], Name: name, Path: path}
			break
		}
	}
	_, err = giveVerdict(t, other, "current", "")
	refusal := requirePruneRefusal(t, err, "knowledge_prune_wrong_entry")
	require.Contains(t, refusal.NextAction, "knowledge prune verdict")
	require.Contains(t, refusal.NextAction, `"path":"`+head.Path+`"`)

	// Changes without evidence.
	_, err = giveVerdict(t, head, "corrected", "   ")
	requirePruneRefusal(t, err, "knowledge_prune_evidence_required")
	_, err = giveVerdict(t, head, "deleted", "")
	requirePruneRefusal(t, err, "knowledge_prune_evidence_required")

	// Claims the store contradicts.
	_, err = giveVerdict(t, head, "corrected", "the flag was renamed")
	requirePruneRefusal(t, err, "knowledge_prune_claim_mismatch")
	_, err = giveVerdict(t, head, "deleted", "the package is gone")
	refusal = requirePruneRefusal(t, err, "knowledge_prune_claim_mismatch")
	require.Contains(t, refusal.NextAction, "knowledge delete --data")
	require.NoError(t, os.WriteFile(headFile, []byte("changed behind prune's back\n"), 0o644))
	_, err = giveVerdict(t, head, "current", "")
	requirePruneRefusal(t, err, "knowledge_prune_claim_mismatch")
	require.NoError(t, os.WriteFile(headFile, headBytes, 0o644))

	// A verdict that is none of the three, and a call with no data.
	_, err = giveVerdict(t, head, "stale", "it looks old")
	requirePruneRefusal(t, err, "knowledge_prune_verdict_invalid")
	_, _, err = runKnowledge(t, "prune", "verdict")
	requirePruneRefusal(t, err, "knowledge_data_required")

	// Every refusal left the same entry at the head.
	resumed := runPrune(t)
	require.True(t, resumed.Resumed)
	require.Equal(t, head, *resumed.Entry)
	require.Equal(t, len(pruneFixtureRemaining), resumed.Remaining)

	usageBefore := readUsage(t)
	report, seen := pruneLoop(t, resumed, func(res pruneOutput) (string, string) {
		switch res.Entry.key() {
		case "testproj:learnings/backfilled.md":
			_, _, err := runKnowledge(t, "write", "--data", `{"tier":"repo","name":"testproj","path":"learnings/backfilled.md"}`,
				"--file", writeTemp(t, "# Backfilled\n\nthe build runs make build\n"))
			require.NoError(t, err)
			return "corrected", "Makefile has no all target; the build target is make build"
		case "testproj:architecture/initial-idea.md":
			_, _, err := runKnowledge(t, "delete", "--data", `{"tier":"repo","name":"testproj","path":"architecture/initial-idea.md"}`)
			require.NoError(t, err)
			return "deleted", "no widgets package exists in any registered repo"
		default:
			return "current", ""
		}
	})
	require.ElementsMatch(t, pruneFixtureRemaining, seen, "every remaining entry is handed out exactly once")

	require.True(t, report.Complete)
	require.False(t, report.DryRun)
	require.Equal(t, map[string]string{
		"testproj:learnings/stale.md":           "removed",
		"testproj:learnings/surfaced.md":        "removed",
		"team:old-team.md":                      "removed",
		"testproj:readme.md":                    "current",
		"testproj:architecture/initial-idea.md": "deleted",
		"testproj:learnings/backfilled.md":      "corrected",
		"team:guidelines.md":                    "current",
	}, reportVerdicts(report.Entries))
	require.Len(t, report.Entries, 7)
	require.Equal(t, map[string]int{"removed": 3, "current": 2, "deleted": 1, "corrected": 1}, report.Counts)
	for _, row := range report.Entries {
		switch row.key() {
		case "testproj:learnings/surfaced.md":
			require.Equal(t, "created 2026-09-14, tracked since 2026-09-14, last retrieved never, 4 discoveries; not retrieved within 30 days", row.Evidence)
		case "testproj:learnings/backfilled.md":
			require.Equal(t, "Makefile has no all target; the build target is make build", row.Evidence)
		}
	}
	require.NoFileExists(t, pruneRunFilePath(root), "the run file is removed with the final report")

	after := readUsage(t)
	corrected := after["testproj:learnings/backfilled.md"]
	require.Equal(t, 2, corrected.Discoveries.Count, "a corrected entry keeps its discoveries")
	require.Equal(t, 1, corrected.Retrievals.Count, "a corrected entry keeps its retrievals")
	requireUsageTime(t, daysAgo(400), corrected.Created)
	require.Equal(t, usageBefore["testproj:learnings/backfilled.md"].Discoveries.Count, corrected.Discoveries.Count)

	raw, err := os.ReadFile(filepath.Join(repoLoc, "learnings", "backfilled.md"))
	require.NoError(t, err)
	require.Equal(t, "# Backfilled\n\nthe build runs make build\n", string(raw))
	guidelinesAfter, err := os.ReadFile(filepath.Join(teamLoc, "guidelines.md"))
	require.NoError(t, err)
	require.Equal(t, guidelinesBefore, guidelinesAfter, "a current entry is left byte for byte")

	repoIdx := indexPaths(t, repoLoc)
	for _, p := range []string{"architecture/initial-idea.md", "learnings/stale.md", "learnings/surfaced.md"} {
		require.False(t, repoIdx[p], "no record for removed or deleted %s", p)
	}
	require.True(t, repoIdx["learnings/backfilled.md"])
	require.False(t, indexPaths(t, teamLoc)["old-team.md"])

	_, err = giveVerdict(t, head, "current", "")
	requirePruneRefusal(t, err, "knowledge_prune_not_running")
}

// Prune criterion 4: a verdict with no prune running is refused, naming how
// to start one.
func TestKnowledgePruneVerdict_WithoutARunIsRefused(t *testing.T) {
	pruneProject(t)
	_, err := giveVerdict(t, prunedAddress{Tier: "repo", Name: "testproj", Path: "readme.md"}, "current", "")
	refusal := requirePruneRefusal(t, err, "knowledge_prune_not_running")
	require.Contains(t, refusal.NextAction, "knowledge prune")
}

// Running prune again resumes the run in progress at the entry being checked;
// --force discards the run, its verdicts with it, and starts again from the
// first entry.
func TestKnowledgePrune_ResumesAndForceRestarts(t *testing.T) {
	root, _, _ := pruneProject(t)

	first := runPrune(t)
	h1 := *first.Entry
	again := runPrune(t)
	require.True(t, again.Resumed)
	require.Equal(t, h1, *again.Entry)
	require.ElementsMatch(t, pruneFixtureRemoved, removalKeys(again.Removed), "a resumed run reports the removals it made")

	next, err := giveVerdict(t, h1, "current", "")
	require.NoError(t, err)
	h2 := *next.Entry
	require.NotEqual(t, h1, h2)
	again = runPrune(t)
	require.True(t, again.Resumed)
	require.Equal(t, h2, *again.Entry)
	require.Equal(t, len(pruneFixtureRemaining)-1, again.Remaining)

	forced := runPrune(t, "--force")
	require.False(t, forced.Resumed)
	require.Equal(t, h1, *forced.Entry, "a forced restart starts from the first entry")
	require.Equal(t, len(pruneFixtureRemaining), forced.Remaining)
	require.Empty(t, forced.Removed, "the first run already removed the old entries")

	report, seen := pruneLoop(t, forced, func(pruneOutput) (string, string) { return "current", "" })
	require.ElementsMatch(t, pruneFixtureRemaining, seen)
	require.Len(t, report.Entries, len(pruneFixtureRemaining), "the discarded run's verdict is not carried over")
	require.NoFileExists(t, pruneRunFilePath(root))
}

// Prune criterion 6: a dry run reports exactly the removals a real run on the
// same knowledge base makes and a verdict for every entry, refuses a verdict
// once an entry has changed, and leaves every entry and each store's index
// byte for byte as it found them.
func TestKnowledgePrune_DryRunChangesNothingAndMatchesARealRun(t *testing.T) {
	root, repoLoc, teamLoc := pruneProject(t)
	repoBefore, teamBefore := hashStore(t, repoLoc), hashStore(t, teamLoc)
	require.Contains(t, repoBefore, knowledge.UsageIndexFile)
	require.Contains(t, teamBefore, knowledge.UsageIndexFile)

	first := runPrune(t, "--dry-run")
	require.True(t, first.DryRun)
	require.ElementsMatch(t, pruneFixtureRemoved, removalKeys(first.Removed))
	require.Equal(t, len(pruneFixtureRemaining), first.Remaining)
	require.Contains(t, first.Instruction, "Change nothing")
	require.NotContains(t, first.Instruction, "knowledge write")
	require.NotContains(t, first.Instruction, "knowledge delete")
	require.Equal(t, repoBefore, hashStore(t, repoLoc), "the first pass removes nothing in a dry run")

	// An entry changed during a dry run is refused whatever the verdict.
	head := *first.Entry
	headFile := filepath.Join(map[string]string{"testproj": repoLoc, "team": teamLoc}[head.Name], filepath.FromSlash(head.Path))
	headBytes, err := os.ReadFile(headFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(headFile, []byte("edited during a dry run\n"), 0o644))
	for _, v := range []string{"current", "corrected"} {
		_, err = giveVerdict(t, head, v, "the subject changed")
		requirePruneRefusal(t, err, "knowledge_prune_dry_run_changed")
	}
	require.NoError(t, os.WriteFile(headFile, headBytes, 0o644))

	report, seen := pruneLoop(t, first, func(res pruneOutput) (string, string) {
		require.True(t, res.DryRun)
		switch res.Entry.key() {
		case "testproj:learnings/backfilled.md":
			return "corrected", "Makefile has no all target"
		case "testproj:architecture/initial-idea.md":
			return "deleted", "no widgets package exists"
		default:
			return "current", ""
		}
	})
	require.ElementsMatch(t, pruneFixtureRemaining, seen)
	require.True(t, report.Complete)
	require.True(t, report.DryRun)
	require.Equal(t, map[string]string{
		"testproj:learnings/stale.md":           "would_remove",
		"testproj:learnings/surfaced.md":        "would_remove",
		"team:old-team.md":                      "would_remove",
		"testproj:readme.md":                    "current",
		"testproj:architecture/initial-idea.md": "would_delete",
		"testproj:learnings/backfilled.md":      "would_correct",
		"team:guidelines.md":                    "current",
	}, reportVerdicts(report.Entries))
	require.Len(t, report.Entries, 7)
	require.NoFileExists(t, pruneRunFilePath(root))

	require.Equal(t, repoBefore, hashStore(t, repoLoc), "a dry run leaves the repo store, its index included, byte for byte")
	require.Equal(t, teamBefore, hashStore(t, teamLoc), "a dry run leaves the team store, its index included, byte for byte")

	// The knowledge base is untouched, so a real run now sees what the dry
	// run saw.
	real := runPrune(t)
	require.False(t, real.DryRun)
	var wouldRemove []string
	for _, row := range report.Entries {
		if row.Verdict == "would_remove" {
			wouldRemove = append(wouldRemove, row.key())
		}
	}
	require.ElementsMatch(t, removalKeys(real.Removed), wouldRemove)
}

// Prune criterion 7: prune's own reads, in a dry run and in a real run, add
// no retrieval and no discovery to any entry.
func TestKnowledgePrune_OwnReadsRecordNoUsage(t *testing.T) {
	pruneProject(t)
	before := readUsage(t)

	report, _ := pruneLoop(t, runPrune(t, "--dry-run"), func(pruneOutput) (string, string) { return "current", "" })
	require.True(t, report.DryRun)
	require.Equal(t, before, readUsage(t), "a dry run records no usage")

	// A real run, resumed once and then restarted with --force, reads every
	// remaining entry several times over.
	_ = runPrune(t)
	resumed := runPrune(t)
	require.True(t, resumed.Resumed)
	report, _ = pruneLoop(t, runPrune(t, "--force"), func(pruneOutput) (string, string) { return "current", "" })
	require.False(t, report.DryRun)

	after := readUsage(t)
	for _, k := range pruneFixtureRemaining {
		require.Equal(t, before[k].Retrievals.Count, after[k].Retrievals.Count, "%s retrievals", k)
		require.Equal(t, before[k].Discoveries.Count, after[k].Discoveries.Count, "%s discoveries", k)
		require.Equal(t, before[k].Retrievals.Last, after[k].Retrievals.Last, "%s last retrieval", k)
	}
	require.Equal(t, 1, after["testproj:readme.md"].Retrievals.Count)
	require.Equal(t, 3, after["team:guidelines.md"].Retrievals.Count)
}

// Both prune commands publish their schemas: prune its flags and output, the
// verdict command the input it requires and the verdicts it accepts.
func TestKnowledgePrune_Schemas(t *testing.T) {
	pruneProject(t)

	prune, _ := knowledgeSchema(t, "prune")
	require.Nil(t, prune.Input)
	for _, f := range []string{"dry-run", "force", "tier", "filter"} {
		require.Contains(t, prune.Flags, f)
	}
	for _, p := range []string{"removed", "remaining", "entry", "instruction", "resumed", "entries", "complete"} {
		require.Contains(t, prune.Output.Properties, p)
	}

	stdout, _, err := runKnowledge(t, "prune", "verdict", "--schema")
	require.NoError(t, err)
	var verdict commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &verdict))
	require.Equal(t, []string{"tier", "name", "path", "verdict"}, verdict.Input.Required)
	require.Equal(t, []string{"current", "corrected", "deleted"}, verdict.Input.Properties["verdict"].Enum)
	require.Contains(t, verdict.Input.Properties, "evidence")
	require.Contains(t, verdict.Output.Properties, "instruction")
	require.Contains(t, verdict.Output.Properties, "complete")
}

// A dry run asked for while a real prune is unfinished, or the reverse, is
// refused rather than resuming the other kind of run, and --force starts the
// one asked for.
func TestKnowledgePrune_ModeMismatchIsRefusedNotResumed(t *testing.T) {
	pruneProject(t)

	real := runPrune(t)
	require.False(t, real.DryRun)
	_, _, err := runKnowledge(t, "prune", "--dry-run")
	refusal := requirePruneRefusal(t, err, "knowledge_prune_in_progress")
	require.Contains(t, refusal.NextAction, "knowledge prune\"")
	require.Contains(t, refusal.NextAction, "--force")

	dry := runPrune(t, "--dry-run", "--force")
	require.True(t, dry.DryRun)
	require.False(t, dry.Resumed)
	_, _, err = runKnowledge(t, "prune")
	refusal = requirePruneRefusal(t, err, "knowledge_prune_in_progress")
	require.Contains(t, refusal.NextAction, "knowledge prune --dry-run")

	resumed := runPrune(t, "--dry-run")
	require.True(t, resumed.Resumed)
	require.True(t, resumed.DryRun)
}
