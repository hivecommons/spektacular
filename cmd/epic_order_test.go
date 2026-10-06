package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file tests `epic order` (cmd/epic_order.go): an epic's planned specs
// whose plans change the same file, and that nothing orders yet, are ordered
// later-listed after earlier-listed; each added dependency is logged in the
// planning summary; only the epic and its summary are written; and a
// dependency undone at review is never added back.
//
// Like cmd/epic_test.go, every command is driven through the root command
// against its own t.TempDir() project, plans are written by hand with
// os.WriteFile, and expected values are written out by hand.

// epicOrderResult mirrors the `epic order` JSON envelope when ordering.
type epicOrderResult struct {
	Epic      string           `json:"epic"`
	Added     []orderAddedJSON `json:"added"`
	Unplanned []string         `json:"unplanned"`
}

type orderAddedJSON struct {
	Spec      string         `json:"spec"`
	DependsOn string         `json:"depends_on"`
	Files     []orderFileRef `json:"files"`
}

type orderFileRef struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
}

// epicUnorderResult mirrors the `epic order` JSON envelope when unordering.
type epicUnorderResult struct {
	Epic    string            `json:"epic"`
	Removed map[string]string `json:"removed"`
}

// The members of the ordering fixture. A and B both change pkg/x.go; C
// changes pkg/y.go (also changed by A) and pkg/z.go (also changed by B), and
// already depends on B; D has no plan.
const (
	orderA = "000010_a"
	orderB = "000011_b"
	orderC = "000012_c"
	orderD = "000013_d"
)

// orderTaskFields are the required lines of an agent task in repo.
func orderTaskFields(id, repo string) []string {
	return []string{"**Id:** " + id, "**Repo:** " + repo, "**Depends on:** none", "**Execution:** agent"}
}

// writeOrderPlan stores a one-task plan for spec, in repo, and a context
// document whose section for that task names files (each written verbatim
// between backticks, so a "<repo>:" prefix may be given).
func writeOrderPlan(t *testing.T, root, spec, repo string, files ...string) {
	t.Helper()
	dir := filepath.Join(root, ".spektacular", "plans", spec)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	title := "Change " + spec
	plan := "---\ncreated_date: 2026-09-20\ndocument_status: final\nspec: " + spec + "\n---\n\n" +
		taskPlanDoc(taskBlock(title, false, orderTaskFields(idA, repo), "- [ ] it changes"))
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = "`" + f + "`"
	}
	context := "---\ncreated_date: 2026-09-19\ndocument_status: final\nspec: " + spec + "\n---\n\n" +
		"# Context: " + spec + "\n\n## Tasks\n\n### Task: " + title + "\n\nEdits " + strings.Join(quoted, ", ") + ".\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.md"), []byte(plan), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "context.md"), []byte(context), 0o644))
}

// orderEpicProject is the ordering fixture: testEpic listing A, B, C (which
// depends on B) and D, with plans for A, B and C in the single registered
// repo testproj.
func orderEpicProject(t *testing.T) string {
	t.Helper()
	root := epicProject(t)
	for _, n := range []string{orderA, orderB, orderC, orderD} {
		writeSpecFixture(t, root, n, epicTestSpecFixed)
	}
	epicWrite(t, testEpic, `{"specs":[`+
		`{"name":"`+orderA+`","depends_on":[]},`+
		`{"name":"`+orderB+`","depends_on":[]},`+
		`{"name":"`+orderC+`","depends_on":["`+orderB+`"]},`+
		`{"name":"`+orderD+`","depends_on":[]}]}`)
	writeOrderPlan(t, root, orderA, "testproj", "pkg/x.go", "pkg/y.go")
	writeOrderPlan(t, root, orderB, "testproj", "pkg/x.go", "pkg/z.go")
	writeOrderPlan(t, root, orderC, "testproj", "pkg/y.go", "pkg/z.go")
	return root
}

// runOrder runs `epic order` expected to succeed and decodes its result.
func runOrder(t *testing.T, args ...string) epicOrderResult {
	t.Helper()
	stdout, code := runEpic(t, append([]string{"order"}, args...)...)
	require.Equalf(t, 0, code, "epic order failed: %s", stdout)
	var got epicOrderResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// runUnorder undoes spec's dependency on dependsOn, expected to succeed.
func runUnorder(t *testing.T, spec, dependsOn string) epicUnorderResult {
	t.Helper()
	stdout, code := runEpic(t, "order", testEpic,
		"--data", `{"unorder":{"spec":"`+spec+`","depends_on":"`+dependsOn+`"}}`)
	require.Equalf(t, 0, code, "epic order unorder failed: %s", stdout)
	var got epicUnorderResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// epicSpecsBlock returns the `specs:` block of the stored epic's frontmatter.
func epicSpecsBlock(t *testing.T, root string) string {
	t.Helper()
	fm := frontmatterOf(t, epicFilePath(root, testEpic))
	for i, line := range fm {
		if line == "specs:" {
			end := i + 1
			for end < len(fm) && strings.HasPrefix(fm[end], " ") {
				end++
			}
			return strings.Join(fm[i:end], "\n") + "\n"
		}
	}
	t.Fatalf("epic %s has no specs block", testEpic)
	return ""
}

// readSummaryFile returns the stored summary's bytes.
func readSummaryFile(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(summaryFilePath(root, testEpic))
	require.NoError(t, err)
	return string(raw)
}

// withoutEpicAndSummary drops the epic and its summary from a snapshot,
// leaving every spec and plan document.
func withoutEpicAndSummary(snap map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range snap {
		if k == filepath.Join(".spektacular", "epics", testEpic+".md") ||
			k == filepath.Join(".spektacular", "epics", testEpic, "summary.md") {
			continue
		}
		out[k] = v
	}
	return out
}

// Criterion: Two independent planned specs that change the same file end up
// with the later-listed one depending on the earlier one, and the user is not
// asked.
func TestEpicOrder_SharedFileOrdersLaterAfterEarlier(t *testing.T) {
	root := orderEpicProject(t)
	body := bodyOf(t, epicFilePath(root, testEpic))

	got := runOrder(t, testEpic)
	require.Equal(t, testEpic, got.Epic)
	require.Equal(t, []orderAddedJSON{
		{Spec: orderB, DependsOn: orderA, Files: []orderFileRef{{Repo: "testproj", Path: "pkg/x.go"}}},
	}, got.Added)

	require.Equal(t, "specs:\n"+
		"    - name: 000010_a\n"+
		"      depends_on: []\n"+
		"    - name: 000011_b\n"+
		"      depends_on:\n"+
		"        - 000010_a\n"+
		"    - name: 000012_c\n"+
		"      depends_on:\n"+
		"        - 000011_b\n"+
		"    - name: 000013_d\n"+
		"      depends_on: []\n", epicSpecsBlock(t, root))
	require.Equal(t, body, bodyOf(t, epicFilePath(root, testEpic)), "the epic's body is untouched")
}

// Criterion: Specs already ordered directly or through another spec gain no
// new dependency, and running the command again adds nothing.
func TestEpicOrder_AlreadyOrderedSpecsGainNothingAndARerunAddsNothing(t *testing.T) {
	root := orderEpicProject(t)

	// C shares pkg/z.go with B, which it depends on directly, and pkg/y.go
	// with A, which it reaches through B once B is ordered after A.
	got := runOrder(t, testEpic)
	require.Len(t, got.Added, 1)
	require.Equal(t, orderB, got.Added[0].Spec)
	require.Contains(t, epicSpecsBlock(t, root),
		"    - name: 000012_c\n      depends_on:\n        - 000011_b\n    - name: 000013_d\n")

	before := snapshotTree(t, root)
	again := runOrder(t, testEpic)
	require.Equal(t, epicOrderResult{Epic: testEpic, Added: []orderAddedJSON{}, Unplanned: []string{orderD}}, again)
	require.Equal(t, before, snapshotTree(t, root), "a run that adds nothing writes nothing")
}

// Specs ordered by the user before any run gain nothing, and nothing is
// written, not even a summary.
func TestEpicOrder_NothingToAddWritesNothing(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, orderA, epicTestSpecFixed)
	writeSpecFixture(t, root, orderB, epicTestSpecFixed)
	epicWrite(t, testEpic, `{"specs":[{"name":"`+orderA+`","depends_on":[]},{"name":"`+orderB+`","depends_on":["`+orderA+`"]}]}`)
	writeOrderPlan(t, root, orderA, "testproj", "pkg/x.go")
	writeOrderPlan(t, root, orderB, "testproj", "pkg/x.go")
	before := snapshotTree(t, root)

	got := runOrder(t, testEpic)
	require.Equal(t, epicOrderResult{Epic: testEpic, Added: []orderAddedJSON{}, Unplanned: []string{}}, got)
	require.Equal(t, before, snapshotTree(t, root))
	require.NoFileExists(t, summaryFilePath(root, testEpic))
}

// Criterion: The added dependency is listed in the summary and in the
// command's result, naming the shared files.
func TestEpicOrder_AddedDependencyIsListedInTheSummaryAndResult(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, orderA, epicTestSpecFixed)
	writeSpecFixture(t, root, orderB, epicTestSpecFixed)
	epicWrite(t, testEpic, specsData(orderA, orderB))
	writeOrderPlan(t, root, orderA, "testproj", "pkg/x.go", "pkg/y.go", "pkg/only-a.go")
	writeOrderPlan(t, root, orderB, "testproj", "pkg/y.go", "pkg/only-b.go", "pkg/x.go")

	got := runOrder(t, testEpic)
	require.Equal(t, epicOrderResult{
		Epic: testEpic,
		Added: []orderAddedJSON{{Spec: orderB, DependsOn: orderA, Files: []orderFileRef{
			{Repo: "testproj", Path: "pkg/x.go"},
			{Repo: "testproj", Path: "pkg/y.go"},
		}}},
		Unplanned: []string{},
	}, got)

	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\n"+
		"- Added: `000011_b` now depends on `000010_a`. Both change `testproj:pkg/x.go`, `testproj:pkg/y.go`.\n",
		readSummaryFile(t, root))
}

// A shared file is matched by repo as well as path: a "docs:" prefix names
// the docs repo, and the same path in a different repo is not shared.
func TestEpicOrder_SharedFilesAreMatchedByRepo(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeSpecCommandConfig(t, root, "spec:\n  id_method: counter\nrepos:\n  - name: testproj\n    location: .\n  - name: docs\n    location: .\n")
	writeSpecFixture(t, root, orderA, epicTestSpecFixed)
	writeSpecFixture(t, root, orderB, epicTestSpecFixed)
	epicWrite(t, testEpic, specsData(orderA, orderB))
	writeOrderPlan(t, root, orderA, "testproj", "pkg/x.go", "docs:guide/intro.md")
	writeOrderPlan(t, root, orderB, "docs", "pkg/x.go", "guide/intro.md")

	got := runOrder(t, testEpic)
	require.Equal(t, []orderAddedJSON{{Spec: orderB, DependsOn: orderA, Files: []orderFileRef{
		{Repo: "docs", Path: "guide/intro.md"},
	}}}, got.Added)
	require.Contains(t, readSummaryFile(t, root),
		"- Added: `000011_b` now depends on `000010_a`. Both change `docs:guide/intro.md`.\n")
}

// Criterion: Both specs' plans keep the same content and created date before
// and after.
func TestEpicOrder_PlansAndSpecsAreUnchanged(t *testing.T) {
	root := orderEpicProject(t)
	before := withoutEpicAndSummary(snapshotTree(t, root))
	for _, n := range []string{orderA, orderB} {
		for _, doc := range []string{"plan.md", "context.md"} {
			require.Contains(t, before, filepath.Join(".spektacular", "plans", n, doc))
		}
		require.Contains(t, before, filepath.Join(".spektacular", "specs", n+".md"))
	}

	got := runOrder(t, testEpic)
	require.Len(t, got.Added, 1)
	require.Equal(t, before, withoutEpicAndSummary(snapshotTree(t, root)),
		"every spec, plan and context document is byte-identical")

	runUnorder(t, orderB, orderA)
	require.Equal(t, before, withoutEpicAndSummary(snapshotTree(t, root)),
		"undoing an ordering writes no spec or plan either")
}

// Criterion: Undoing an added dependency removes it from the epic, the
// summary shows it was removed, and a later run does not add it back.
func TestEpicOrder_UndoRemovesTheDependencyForGood(t *testing.T) {
	root := orderEpicProject(t)
	runOrder(t, testEpic)

	got := runUnorder(t, orderB, orderA)
	require.Equal(t, epicUnorderResult{Epic: testEpic, Removed: map[string]string{"spec": orderB, "depends_on": orderA}}, got)
	require.Equal(t, "specs:\n"+
		"    - name: 000010_a\n"+
		"      depends_on: []\n"+
		"    - name: 000011_b\n"+
		"      depends_on: []\n"+
		"      parallel_with:\n"+
		"        - 000010_a\n"+
		"    - name: 000012_c\n"+
		"      depends_on:\n"+
		"        - 000011_b\n"+
		"    - name: 000013_d\n"+
		"      depends_on: []\n", epicSpecsBlock(t, root))

	added := "- Added: `000011_b` now depends on `000010_a`. Both change `testproj:pkg/x.go`.\n"
	removed := "- Removed at review: `000011_b` no longer depends on `000010_a`; they may be implemented side by side.\n"
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\n"+added+removed,
		readSummaryFile(t, root))

	// With B no longer after A, C no longer reaches A through B, so C (which
	// shares pkg/y.go with A) is ordered after A. B is not ordered after A
	// again.
	again := runOrder(t, testEpic)
	require.Equal(t, []orderAddedJSON{
		{Spec: orderC, DependsOn: orderA, Files: []orderFileRef{{Repo: "testproj", Path: "pkg/y.go"}}},
	}, again.Added)
	require.Contains(t, epicSpecsBlock(t, root),
		"    - name: 000011_b\n      depends_on: []\n      parallel_with:\n        - 000010_a\n")

	final := runOrder(t, testEpic)
	require.Empty(t, final.Added)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\n"+added+removed+
		"- Added: `000012_c` now depends on `000010_a`. Both change `testproj:pkg/y.go`.\n",
		readSummaryFile(t, root))
}

// Undoing a dependency the epic does not record, or giving malformed input,
// is refused and writes nothing.
func TestEpicOrder_UndoRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, data, code string
	}{
		{"a dependency the epic does not record", `{"unorder":{"spec":"000011_b","depends_on":"000012_c"}}`, "epic_dependency_not_found"},
		{"a dependency of a spec outside the epic", `{"unorder":{"spec":"000099_x","depends_on":"000010_a"}}`, "epic_dependency_not_found"},
		{"malformed data", `{not json`, "bad_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := orderEpicProject(t)
			runOrder(t, testEpic)
			before := snapshotTree(t, root)

			er := refuseEpic(t, "order", testEpic, "--data", tc.data)
			require.Equal(t, tc.code, er.Code)
			require.Equal(t, before, snapshotTree(t, root))
		})
	}

	t.Run("the refusal names the spec and points at status", func(t *testing.T) {
		orderEpicProject(t)
		er := refuseEpic(t, "order", testEpic, "--data", `{"unorder":{"spec":"000011_b","depends_on":"000010_a"}}`)
		require.Equal(t, "epic_dependency_not_found", er.Code)
		require.Equal(t, orderB, er.Resource)
		require.Contains(t, er.NextAction, "status "+testEpic)
	})
}

// Criterion: Specs without a plan are left out and reported as unplanned.
func TestEpicOrder_UnplannedSpecsAreLeftOutAndReported(t *testing.T) {
	root := orderEpicProject(t)
	// D's plan-less context document names the shared file; without a
	// plan.md, D is still unplanned and left out.
	dDir := filepath.Join(root, ".spektacular", "plans", orderD)
	require.NoError(t, os.MkdirAll(dDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dDir, "context.md"),
		[]byte("# Context\n\n### Task: Change 000013_d\n\nEdits `pkg/x.go`.\n"), 0o644))

	got := runOrder(t, testEpic)
	require.Equal(t, []string{orderD}, got.Unplanned)
	for _, a := range got.Added {
		require.NotEqual(t, orderD, a.Spec)
		require.NotEqual(t, orderD, a.DependsOn)
	}
	require.Contains(t, epicSpecsBlock(t, root), "    - name: 000013_d\n      depends_on: []\n")
}

// An epic that does not exist is refused.
func TestEpicOrder_UnknownEpicIsRefused(t *testing.T) {
	root := orderEpicProject(t)
	before := snapshotTree(t, root)
	er := refuseEpic(t, "order", "000099_absent")
	require.Equal(t, "epic_not_found", er.Code)
	require.Equal(t, before, snapshotTree(t, root))
}
