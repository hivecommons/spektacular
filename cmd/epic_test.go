package cmd

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file tests the `epic` command group in cmd/epic.go and the membership
// links cmd/epic_link.go keeps between an epic and its specs. The contract
// held here is that the two sides always agree: every spec an epic lists
// names it in its `epic` frontmatter, and every spec naming an epic is listed
// by it. A change that cannot keep them agreeing is refused before anything
// is written, and a write that fails part-way puts every document back
// exactly as it was.
//
// Every command is driven through resetRootCmd + runRootCmd, never by calling
// a RunE directly, because the cobra commands are package-level globals whose
// flag values would otherwise leak into whichever test `-shuffle=on` runs
// next. Fixtures are written with os.WriteFile and stored state is read back
// by scanning the bytes by hand, so the oracle is independent of the code
// under test.

// epicWriteResult mirrors the `epic write` JSON envelope.
type epicWriteResult struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Specs    []string `json:"specs"`
	Linked   []string `json:"linked"`
	Unlinked []string `json:"unlinked"`
}

// epicDeleteResult mirrors the `epic delete` JSON envelope.
type epicDeleteResult struct {
	Name     string   `json:"name"`
	Deleted  bool     `json:"deleted"`
	Unlinked []string `json:"unlinked"`
}

// The epic name most tests use. It already carries a counter ID, so it is
// used exactly as given rather than being assigned a fresh one.
const testEpic = "000050_rollout"

// The epic body every write supplies, and the stored spec fixture: a valid
// frontmatter block and a body, kept separate so a spec carrying an `epic`
// field can be built from the same parts.
const (
	epicTestBody      = "## Overview\n\nRoll the feature out in stages.\n\n## Specs\n\nSee frontmatter.\n"
	epicTestSpecBody  = "# A spec\n\nThe spec's own prose.\n"
	epicTestSpecFixed = "---\ncreated_date: 2026-07-01\ndocument_status: draft\n---\n\n" + epicTestSpecBody
)

// specInEpicFixture is a stored spec whose frontmatter already names epic.
func specInEpicFixture(epic string) string {
	return "---\ncreated_date: 2026-07-01\ndocument_status: draft\nepic: " + epic + "\n---\n\n" + epicTestSpecBody
}

// epicProject lays out a project in a fresh temp dir (and chdirs into it)
// with spec.id_method pinned to counter, so every ID in this file is
// deterministic. It creates no epics folder.
func epicProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	writeSpecCommandConfig(t, root, "spec:\n  id_method: counter\n")
	return root
}

// epicFilePath is where the default config stores the named epic.
func epicFilePath(root, name string) string {
	return filepath.Join(root, ".spektacular", "epics", name+".md")
}

// epicBodyFile stages the epic body outside the project and returns its path.
func epicBodyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "epic-body.md")
	require.NoError(t, os.WriteFile(path, []byte(epicTestBody), 0o644))
	return path
}

// runEpic drives one `epic` invocation through the root command, asserting
// only that nothing reached stderr.
func runEpic(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	resetRootCmd(t)
	out, errOut, code := runRootCmd(t, append([]string{"epic"}, args...)...)
	require.Empty(t, errOut)
	return out, code
}

// epicWrite runs an `epic write` expected to succeed. An empty data omits
// --data entirely.
func epicWrite(t *testing.T, name, data string) epicWriteResult {
	t.Helper()
	args := []string{"write", name, "--from", epicBodyFile(t)}
	if data != "" {
		args = append(args, "--data", data)
	}
	stdout, code := runEpic(t, args...)
	require.Equalf(t, 0, code, "epic write failed: %s", stdout)
	var got epicWriteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// refuseEpic runs an `epic` invocation expected to be refused and returns the
// failure envelope, asserting every refusal states its problem and gives a
// next step.
func refuseEpic(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	stdout, code := runEpic(t, args...)
	require.Equalf(t, 1, code, "expected a refusal, got: %s", stdout)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.NotEmpty(t, er.Message, "a refusal must state the problem")
	require.NotEmpty(t, er.NextAction, "a refusal must give a runnable next step")
	return er
}

// specsData builds a --data payload listing names with no dependencies.
func specsData(names ...string) string {
	entries := make([]string, len(names))
	for i, n := range names {
		entries[i] = `{"name":"` + n + `","depends_on":[]}`
	}
	return `{"specs":[` + strings.Join(entries, ",") + `]}`
}

// frontmatterOf returns the lines of path's leading frontmatter block.
func frontmatterOf(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(raw), "\n")
	require.Equal(t, "---", lines[0], "%s must open with a frontmatter fence", path)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return lines[1:i]
		}
	}
	t.Fatalf("%s has no closing frontmatter fence", path)
	return nil
}

// specEpicOf scans a stored spec's frontmatter by hand for its `epic` field,
// returning "" when it has none.
func specEpicOf(t *testing.T, path string) string {
	t.Helper()
	for _, line := range frontmatterOf(t, path) {
		if v, ok := strings.CutPrefix(line, "epic: "); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// epicSpecsOf scans a stored epic's frontmatter by hand for its members.
func epicSpecsOf(t *testing.T, path string) []string {
	t.Helper()
	out := []string{}
	for _, line := range frontmatterOf(t, path) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "- name: "); ok {
			out = append(out, v)
		}
	}
	return out
}

// snapshotTree reads every regular file under root into a map keyed by its
// path relative to root, so a refusal can be shown to have written nothing.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		snap[rel] = string(raw)
		return nil
	}))
	return snap
}

// requireAgreement asserts both directions of the membership invariant for
// one epic over the given specs: each spec listed names the epic, and each
// spec naming the epic is listed.
func requireAgreement(t *testing.T, root, epic string, specs ...string) {
	t.Helper()
	listed := map[string]bool{}
	for _, n := range epicSpecsOf(t, epicFilePath(root, epic)) {
		listed[n] = true
	}
	for _, n := range specs {
		names := specEpicOf(t, filepath.Join(root, ".spektacular", "specs", n+".md")) == epic
		require.Equalf(t, listed[n], names,
			"spec %q: listed by the epic=%v but names the epic=%v", n, listed[n], names)
	}
}

// Criterion: after an epic is written, every spec it lists names it, every
// spec that names it is listed, and the specs' bodies are untouched.
func TestEpicWrite_LinksEverySpecItLists(t *testing.T) {
	root := epicProject(t)
	pa := writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	pb := writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	pc := writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)

	got := epicWrite(t, testEpic, `{"specs":[{"name":"000010_a","depends_on":[]},{"name":"000011_b","depends_on":["000010_a"]}]}`)
	require.Equal(t, epicWriteResult{
		Name:     testEpic,
		Path:     "epics/" + testEpic + ".md",
		Specs:    []string{"000010_a", "000011_b"},
		Linked:   []string{"000010_a", "000011_b"},
		Unlinked: []string{},
	}, got)

	require.Equal(t, []string{"000010_a", "000011_b"}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	require.Equal(t, testEpic, specEpicOf(t, pa))
	require.Equal(t, testEpic, specEpicOf(t, pb))
	require.Equal(t, "", specEpicOf(t, pc), "a spec the epic does not list must not name it")
	requireAgreement(t, root, testEpic, "000010_a", "000011_b", "000012_c")

	for _, p := range []string{pa, pb} {
		raw, err := os.ReadFile(p)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(string(raw), epicTestSpecBody),
			"linking must rewrite only the spec's frontmatter, never its body")
	}

	stored, err := os.ReadFile(epicFilePath(root, testEpic))
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(stored), epicTestBody), "the epic body is stored as supplied")
	require.Contains(t, string(stored), "depends_on:\n        - 000010_a")
}

// Criterion: removing a spec from an epic's list clears that spec's `epic`.
func TestEpicWrite_RemovingASpecClearsItsEpic(t *testing.T) {
	root := epicProject(t)
	pa := writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	pb := writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	pc := writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)
	epicWrite(t, testEpic, specsData("000010_a", "000011_b"))

	got := epicWrite(t, testEpic, specsData("000010_a", "000012_c"))
	require.Equal(t, []string{"000010_a", "000012_c"}, got.Specs)
	require.Equal(t, []string{"000012_c"}, got.Linked)
	require.Equal(t, []string{"000011_b"}, got.Unlinked)

	require.Equal(t, testEpic, specEpicOf(t, pa))
	require.Equal(t, "", specEpicOf(t, pb), "a spec dropped from the list must no longer name the epic")
	require.Equal(t, testEpic, specEpicOf(t, pc))
	requireAgreement(t, root, testEpic, "000010_a", "000011_b", "000012_c")
}

// Criterion: deleting an epic leaves no spec naming it, and the epic is gone.
func TestEpicDelete_LeavesNoSpecNamingIt(t *testing.T) {
	root := epicProject(t)
	pa := writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	pb := writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, specsData("000010_a", "000011_b"))

	stdout, code := runEpic(t, "delete", testEpic)
	require.Equalf(t, 0, code, "delete failed: %s", stdout)
	var got epicDeleteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, epicDeleteResult{Name: testEpic, Deleted: true, Unlinked: []string{"000010_a", "000011_b"}}, got)

	require.NoFileExists(t, epicFilePath(root, testEpic))
	require.Equal(t, "", specEpicOf(t, pa))
	require.Equal(t, "", specEpicOf(t, pb))

	er := refuseEpic(t, "read", testEpic)
	require.Equal(t, "epic_not_found", er.Code)
}

// Criterion: adding a spec that already belongs to another epic is refused,
// naming the other epic and how to take the spec out of it, and nothing is
// written anywhere.
func TestEpicWrite_MembershipConflictIsRefusedAndWritesNothing(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)
	epicWrite(t, "000060_other", specsData("000011_b"))
	epicWrite(t, testEpic, specsData("000010_a"))
	before := snapshotTree(t, root)

	er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000012_c", "000011_b"))
	require.Equal(t, "epic_membership_conflict", er.Code)
	require.Equal(t, "000011_b", er.Resource)
	require.Contains(t, er.Message, `"000011_b"`)
	require.Contains(t, er.Message, `"000060_other"`)
	require.Contains(t, er.NextAction, "spektacular epic write 000060_other --from")
	require.Contains(t, er.NextAction, "or leave it out of this epic")

	require.Equal(t, before, snapshotTree(t, root),
		"a refused membership change must leave every document byte-for-byte unchanged")
}

// Criterion: listing an epic as a member — another epic or the epic itself —
// is refused, and nothing is written.
func TestEpicWrite_NestingIsRefusedAndWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member string
	}{
		{"another epic", "000060_other"},
		{"the epic itself", testEpic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := epicProject(t)
			writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
			writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
			epicWrite(t, "000060_other", specsData("000011_b"))
			before := snapshotTree(t, root)

			er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", tc.member))
			require.Equal(t, "epic_nested", er.Code)
			require.Equal(t, tc.member, er.Resource)
			require.Contains(t, er.Message, "epics do not nest")
			require.Contains(t, er.NextAction, "remove \""+tc.member+"\" from the specs list")
			require.Contains(t, er.NextAction, "spektacular epic read "+tc.member)

			require.Equal(t, before, snapshotTree(t, root))
			require.NoFileExists(t, epicFilePath(root, testEpic))
		})
	}
}

// A spec that is not stored is refused before anything is written, pointing
// at the listing of stored specs.
func TestEpicWrite_UnknownSpecIsRefusedAndWritesNothing(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	before := snapshotTree(t, root)

	er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000099_missing"))
	require.Equal(t, "epic_spec_not_found", er.Code)
	require.Equal(t, "000099_missing", er.Resource)
	require.Equal(t, "run `spektacular spec file list` to see the stored specs, and list only those in the epic", er.NextAction)
	require.Equal(t, before, snapshotTree(t, root))
}

// An invalid dependency graph is refused with epic_invalid and nothing is
// written.
func TestEpicWrite_InvalidGraphIsRefusedAndWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name, data, msg string
	}{
		{"cycle", `{"specs":[{"name":"000010_a","depends_on":["000011_b"]},{"name":"000011_b","depends_on":["000010_a"]}]}`, "dependency cycle"},
		{"unknown dependency", `{"specs":[{"name":"000010_a","depends_on":["000099_x"]}]}`, "not a spec in this epic"},
		{"missing depends_on", `{"specs":[{"name":"000010_a"}]}`, "has no depends_on list"},
		{"duplicate", specsData("000010_a", "000010_a"), "listed more than once"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := epicProject(t)
			writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
			writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
			before := snapshotTree(t, root)

			er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", tc.data)
			require.Equal(t, "epic_invalid", er.Code)
			require.Contains(t, er.Message, tc.msg)
			require.Contains(t, er.NextAction, "fix the specs list")
			require.Equal(t, before, snapshotTree(t, root))
		})
	}
}

// errForcedEpicLink is the failure the writeEpicLinkFn substitutes return.
var errForcedEpicLink = errors.New("forced epic link failure")

// failEpicLinkOnCall substitutes writeEpicLinkFn so the nth call (1-based)
// runs before() and then fails, restoring the real function when t ends.
func failEpicLinkOnCall(t *testing.T, n int, before func()) {
	t.Helper()
	original := writeEpicLinkFn
	t.Cleanup(func() { writeEpicLinkFn = original })
	calls := 0
	writeEpicLinkFn = func(txn *docTxn, spec specFile, epic string) error {
		calls++
		if calls == n {
			if before != nil {
				before()
			}
			return errForcedEpicLink
		}
		return original(txn, spec, epic)
	}
}

// Criterion: if a write fails part-way, the epic and every spec involved are
// restored to their previous content, and a new epic does not remain.
func TestEpicWrite_FailurePartWayRestoresEveryDocument(t *testing.T) {
	t.Run("new epic: the second spec's link write fails", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		before := snapshotTree(t, root)
		failEpicLinkOnCall(t, 2, nil)

		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000011_b"))
		require.Equal(t, "epic_link_failed", er.Code)
		require.Contains(t, er.Message, errForcedEpicLink.Error())
		require.Contains(t, er.Message, "every document was restored")
		require.Contains(t, er.NextAction, "reissue the same command")

		require.Equal(t, before, snapshotTree(t, root),
			"the first spec's link must be undone, and no epic file may remain")
		require.NoFileExists(t, epicFilePath(root, testEpic))
	})

	t.Run("existing epic: the second spec's link write fails", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)
		epicWrite(t, testEpic, specsData("000010_a"))
		before := snapshotTree(t, root)
		failEpicLinkOnCall(t, 2, nil)

		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000011_b", "000012_c"))
		require.Equal(t, "epic_link_failed", er.Code)
		require.Equal(t, before, snapshotTree(t, root),
			"the epic and every spec involved must be back exactly as they were")
	})

	t.Run("a spec path is unusable on disk", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		pb := writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)
		epicWrite(t, testEpic, specsData("000010_a", "000011_b"))
		// A directory in place of the file, not chmod: CI runs as root, which
		// bypasses permission bits. Joining c succeeds first; unlinking b then
		// fails, after the epic and c have been written.
		replaceWithDirectory(t, pb)
		before := snapshotTree(t, root)

		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000012_c"))
		require.Equal(t, "epic_link_failed", er.Code)
		require.Contains(t, er.NextAction, "reissue the same command")
		require.Equal(t, before, snapshotTree(t, root),
			"the epic rewrite and c's new link must both be undone")
	})

	t.Run("a new epic whose spec path is unusable on disk leaves no epic", func(t *testing.T) {
		root := epicProject(t)
		pa := writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		pb := writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		// b becomes unusable between the pre-write check and its own link
		// write, the only window in which the filesystem alone can fail a
		// link write that the check let through.
		failEpicLinkOnCall(t, 2, func() { replaceWithDirectory(t, pb) })

		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000011_b"))
		require.Equal(t, "epic_link_failed", er.Code)
		require.NoFileExists(t, epicFilePath(root, testEpic), "a new epic must not remain after a failed write")
		raw, err := os.ReadFile(pa)
		require.NoError(t, err)
		require.Equal(t, epicTestSpecFixed, string(raw))
	})

	t.Run("restoring afterwards also fails", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		epicWrite(t, testEpic, specsData("000010_a"))
		ep := epicFilePath(root, testEpic)
		failEpicLinkOnCall(t, 1, func() { replaceWithDirectory(t, ep) })

		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", specsData("000010_a", "000011_b"))
		require.Equal(t, "epic_link_rollback_failed", er.Code)
		require.Contains(t, er.Message, errForcedEpicLink.Error())
		require.Contains(t, er.NextAction, "reconcile these documents by hand")
		require.Contains(t, er.NextAction, filepath.Join(".spektacular", "epics", testEpic+".md"))
	})
}

// Deleting an epic is one transaction too: a failure part-way puts the
// already-unlinked specs back and keeps the epic.
func TestEpicDelete_FailurePartWayRestoresEveryDocument(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, specsData("000010_a", "000011_b"))
	before := snapshotTree(t, root)
	failEpicLinkOnCall(t, 2, nil)

	er := refuseEpic(t, "delete", testEpic)
	require.Equal(t, "epic_link_failed", er.Code)
	require.Equal(t, before, snapshotTree(t, root))
}

// Criterion: writing the first epic in a project with no epics folder creates
// the folder and succeeds.
func TestEpicWrite_FirstEpicCreatesTheFolder(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	require.NoDirExists(t, filepath.Join(root, ".spektacular", "epics"))

	stdout, code := runEpic(t, "list")
	require.Equal(t, 0, code)
	require.JSONEq(t, `{"error":false,"files":[]}`, stdout, "listing with no epics folder is an empty list")

	epicWrite(t, testEpic, specsData("000010_a"))
	require.DirExists(t, filepath.Join(root, ".spektacular", "epics"))
	require.FileExists(t, epicFilePath(root, testEpic))
}

// A bare new name is given an ID with spec.id_method, run against the epic
// folder; a name already carrying a matching ID is used as given.
func TestEpicWrite_BareNameGetsACounterID(t *testing.T) {
	root := epicProject(t)
	// A high-numbered spec proves the counter is run against the epic
	// folder, not the spec folder.
	writeSpecFixture(t, root, "000040_a", epicTestSpecFixed)

	got := epicWrite(t, "rollout", specsData("000040_a"))
	require.Equal(t, "000001_rollout", got.Name)
	require.Equal(t, "epics/000001_rollout.md", got.Path)
	require.FileExists(t, epicFilePath(root, "000001_rollout"))
	require.Equal(t, "000001_rollout", specEpicOf(t, filepath.Join(root, ".spektacular", "specs", "000040_a.md")))

	got = epicWrite(t, "migration", "")
	require.Equal(t, "000002_migration", got.Name)

	got = epicWrite(t, "000007_given", "")
	require.Equal(t, "000007_given", got.Name, "a name already carrying a counter ID is used as given")
	require.FileExists(t, epicFilePath(root, "000007_given"))
}

// A write with no --data on an existing epic keeps its specs, sources and
// provenance; only the body changes.
func TestEpicWrite_NoDataKeepsTheGraphAndSources(t *testing.T) {
	root := epicProject(t)
	pa := writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, `{"specs":[{"name":"000010_a","depends_on":[]},{"name":"000011_b","depends_on":["000010_a"]}],"sources":[{"uri":"https://example.com/issue/1","retrieved_date":"2026-01-02"}],"spec":"000010_a"}`)
	before := frontmatterOf(t, epicFilePath(root, testEpic))

	newBody := filepath.Join(t.TempDir(), "new.md")
	require.NoError(t, os.WriteFile(newBody, []byte("## Overview\n\nRewritten.\n"), 0o644))
	stdout, code := runEpic(t, "write", testEpic, "--from", newBody)
	require.Equalf(t, 0, code, "write failed: %s", stdout)
	var got epicWriteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []string{"000010_a", "000011_b"}, got.Specs)
	require.Equal(t, []string{}, got.Linked)
	require.Equal(t, []string{}, got.Unlinked)

	require.Equal(t, before, frontmatterOf(t, epicFilePath(root, testEpic)),
		"omitting --data must keep the specs, sources and spec exactly")
	raw, err := os.ReadFile(epicFilePath(root, testEpic))
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(raw), "## Overview\n\nRewritten.\n"))
	require.Equal(t, testEpic, specEpicOf(t, pa))
}

// Sources get retrieved_date stamped with today when omitted; an empty uri is
// refused with sources_invalid and nothing is written.
func TestEpicWrite_Sources(t *testing.T) {
	t.Run("retrieved_date is stamped when omitted and kept when given", func(t *testing.T) {
		root := epicProject(t)
		epicWrite(t, testEpic, `{"sources":[{"uri":"https://example.com/a"},{"uri":"https://example.com/b","retrieved_date":"2026-01-02"}]}`)
		fm := strings.Join(frontmatterOf(t, epicFilePath(root, testEpic)), "\n")
		today := time.Now().UTC().Format("2006-01-02")
		require.Contains(t, fm, "uri: https://example.com/a\n      retrieved_date: \""+today+"\"")
		require.Contains(t, fm, "uri: https://example.com/b\n      retrieved_date: \"2026-01-02\"")
	})

	t.Run("an empty uri is refused", func(t *testing.T) {
		root := epicProject(t)
		before := snapshotTree(t, root)
		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", `{"sources":[{"uri":"https://example.com/a"},{"uri":""}]}`)
		require.Equal(t, "sources_invalid", er.Code)
		require.Equal(t, "sources[1]", er.Resource)
		require.Contains(t, er.NextAction, `"sources":[{"uri":"https://`)
		require.Equal(t, before, snapshotTree(t, root))
	})

	t.Run("a malformed retrieved_date is refused", func(t *testing.T) {
		epicProject(t)
		er := refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", `{"sources":[{"uri":"https://example.com/a","retrieved_date":"yesterday"}]}`)
		require.Equal(t, "sources_invalid", er.Code)
		require.Contains(t, er.NextAction, "YYYY-MM-DD")
	})
}

// `epic read` writes the stored bytes unchanged; an absent epic is
// epic_not_found pointing at `epic list`.
func TestEpicRead_RoundTripAndNotFound(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	epicWrite(t, testEpic, specsData("000010_a"))

	stdout, code := runEpic(t, "read", testEpic)
	require.Equal(t, 0, code)
	stored, err := os.ReadFile(epicFilePath(root, testEpic))
	require.NoError(t, err)
	require.Equal(t, string(stored), stdout)

	er := refuseEpic(t, "read", "000099_absent")
	require.Equal(t, "epic_not_found", er.Code)
	require.Equal(t, "000099_absent", er.Resource)
	require.Equal(t, "run `spektacular epic list` to see the stored epics", er.NextAction)

	er = refuseEpic(t, "delete", "000099_absent")
	require.Equal(t, "epic_not_found", er.Code)
	require.Contains(t, er.NextAction, "spektacular epic list")
}

// Other input refusals each state the problem and give a runnable next step.
func TestEpic_InputRefusalsCarryNextAction(t *testing.T) {
	epicProject(t)

	er := refuseEpic(t, "read")
	require.Equal(t, "epic_name_required", er.Code)
	require.Contains(t, er.NextAction, "spektacular epic list")

	er = refuseEpic(t, "write", testEpic)
	require.Equal(t, "epic_from_required", er.Code)
	require.Contains(t, er.NextAction, "spektacular epic write "+testEpic+" --from")

	er = refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--data", `{not json`)
	require.Equal(t, "bad_input", er.Code)
	require.Contains(t, er.NextAction, `--data as a JSON object`)

	er = refuseEpic(t, "read", testEpic+".md")
	require.Equal(t, "unexpected_extension", er.Code)
	require.Contains(t, er.NextAction, `reissue with the bare name "`+testEpic+`"`)

	er = refuseEpic(t, "write", testEpic, "--from", epicBodyFile(t), "--document-status", "bogus")
	require.NotEmpty(t, er.Code)

	er = refuseEpic(t, "frobnicate")
	require.Equal(t, "unknown_subcommand", er.Code)
	require.Equal(t, "run one of: delete, list, merge, order, read, split, summary, worktree, write", er.NextAction)
}

// `epic list` reports each epic's name, location and lifecycle fields.
func TestEpicList_ReportsFields(t *testing.T) {
	epicProject(t)
	epicWrite(t, testEpic, "")
	epicWrite(t, "000060_other", "")

	stdout, code := runEpic(t, "list")
	require.Equal(t, 0, code)
	var got struct {
		Files []map[string]string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Files, 2)

	today := time.Now().UTC().Format("2006-01-02")
	byName := map[string]map[string]string{}
	for _, f := range got.Files {
		byName[f["name"]] = f
		require.NotEmpty(t, f["modified_at"])
		_, err := time.Parse(time.RFC3339, f["modified_at"])
		require.NoError(t, err)
		require.Equal(t, today, f["created_date"])
		require.Equal(t, "draft", f["document_status"])
		_, hasClosed := f["closed_date"]
		require.False(t, hasClosed, "an open epic carries no closed_date")
	}
	require.Equal(t, "epics/"+testEpic+".md", byName[testEpic]["path"])
	require.Equal(t, "epics/000060_other.md", byName["000060_other"]["path"])
}

// `--schema` on each verb, including `order` and the nested `summary` verbs, publishes an input and an output schema.
func TestEpicSchema_EachVerbPublishesInputAndOutput(t *testing.T) {
	epicProject(t)
	for verb, outKeys := range map[string][]string{
		"read":          {"content"},
		"write":         {"name", "path", "specs", "linked", "unlinked"},
		"list":          {"files"},
		"delete":        {"name", "deleted", "unlinked"},
		"split":         {"epic", "path", "created", "linked"},
		"order":         {"epic", "added", "unplanned", "removed"},
		"summary read":  {"content"},
		"summary write": {"epic", "section", "sections"},
	} {
		t.Run(verb, func(t *testing.T) {
			stdout, code := runEpic(t, append(strings.Fields(verb), "--schema")...)
			require.Equal(t, 0, code)
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
			require.Contains(t, raw, "input")
			require.Contains(t, raw, "output")

			var out struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(raw["output"], &out))
			keys := make([]string, 0, len(out.Properties))
			for k := range out.Properties {
				keys = append(keys, k)
			}
			require.ElementsMatch(t, outKeys, keys)

			if want, ok := map[string][]string{
				"write":         {"specs", "sources", "spec", "confirm_completed_epic"},
				"split":         {"spec", "overview", "sources", "specs", "confirm_completed_epic"},
				"summary write": {"section"},
				"order":         {"unorder"},
			}[verb]; ok {
				var in struct {
					Properties map[string]json.RawMessage `json:"properties"`
				}
				require.NoError(t, json.Unmarshal(raw["input"], &in))
				inKeys := make([]string, 0, len(in.Properties))
				for k := range in.Properties {
					inKeys = append(inKeys, k)
				}
				require.ElementsMatch(t, want, inKeys)
			} else {
				require.Equal(t, "null", string(raw["input"]))
			}
		})
	}
}
