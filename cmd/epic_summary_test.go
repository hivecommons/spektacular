package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This file tests `epic summary read / write` in cmd/epic_summary.go and the
// removal of an epic's summary by `epic delete`. Every command is driven
// through the root command against its own t.TempDir() project, and the
// stored summary is compared byte for byte with expected text written out by
// hand.

// epicSummaryWriteResult mirrors the `epic summary write` JSON envelope.
type epicSummaryWriteResult struct {
	Epic     string   `json:"epic"`
	Section  string   `json:"section"`
	Sections []string `json:"sections"`
}

// summaryFilePath is where the default config stores the named epic's summary.
func summaryFilePath(root, epicName string) string {
	return filepath.Join(root, ".spektacular", "epics", epicName, "summary.md")
}

// summaryBodyFile stages a section body outside the project.
func summaryBodyFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "section.md")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// summaryWrite runs an `epic summary write` expected to succeed.
func summaryWrite(t *testing.T, epicName, section, body string) epicSummaryWriteResult {
	t.Helper()
	stdout, code := runEpic(t, "summary", "write", epicName,
		"--data", `{"section":"`+section+`"}`, "--from", summaryBodyFile(t, body))
	require.Equalf(t, 0, code, "summary write failed: %s", stdout)
	var got epicSummaryWriteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// summaryEpicProject is a project holding testEpic with members 000010_a then
// 000011_b, and a third spec, 000012_c, that belongs to no epic.
func summaryEpicProject(t *testing.T) string {
	t.Helper()
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	writeSpecFixture(t, root, "000012_c", epicTestSpecFixed)
	epicWrite(t, testEpic, specsData("000010_a", "000011_b"))
	return root
}

func todayFrontmatter() string {
	return "---\ncreated_date: \"" + time.Now().UTC().Format("2006-01-02") + "\"\n---\n\n"
}

// Criterion: An epic's summary can be written section by section and read
// back, with decisions first whatever order the sections were written in.
func TestEpicSummary_WrittenSectionBySectionReadsBackDecisionsFirst(t *testing.T) {
	root := summaryEpicProject(t)

	got := summaryWrite(t, testEpic, "000011_b", "B's plan.\n")
	require.Equal(t, epicSummaryWriteResult{
		Epic: testEpic, Section: "000011_b",
		Sections: []string{"decisions", "ordering", "000011_b"},
	}, got)

	got = summaryWrite(t, testEpic, "000010_a", "A's plan.\n\n### Notes\nShares cmd/root.go.\n")
	require.Equal(t, []string{"decisions", "ordering", "000010_a", "000011_b"}, got.Sections,
		"spec sections follow the epic's list order, not write order")

	got = summaryWrite(t, testEpic, "decisions", "- Choose the default backend.\n")
	require.Equal(t, epicSummaryWriteResult{
		Epic: testEpic, Section: "decisions",
		Sections: []string{"decisions", "ordering", "000010_a", "000011_b"},
	}, got)

	want := todayFrontmatter() +
		"# Planning summary: 000050_rollout\n" +
		"\n" +
		"## Decisions\n" +
		"- Choose the default backend.\n" +
		"\n" +
		"## Order added for shared files\n" +
		"None added.\n" +
		"\n" +
		"## 000010_a\n" +
		"A's plan.\n" +
		"\n" +
		"### Notes\n" +
		"Shares cmd/root.go.\n" +
		"\n" +
		"## 000011_b\n" +
		"B's plan.\n"

	stdout, code := runEpic(t, "summary", "read", testEpic)
	require.Equal(t, 0, code)
	require.Equal(t, want, stdout)

	stored, err := os.ReadFile(summaryFilePath(root, testEpic))
	require.NoError(t, err)
	require.Equal(t, want, string(stored), "read writes the stored bytes unchanged")
}

// Criterion: The decisions section is always present and says there are none
// when it is empty.
func TestEpicSummary_DecisionsSectionAlwaysPresent(t *testing.T) {
	summaryEpicProject(t)

	summaryWrite(t, testEpic, "000010_a", "A's plan.\n")
	stdout, code := runEpic(t, "summary", "read", testEpic)
	require.Equal(t, 0, code)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\nNone added.\n\n"+
		"## 000010_a\nA's plan.\n", stdout)

	// Emptying the decisions again brings back the "none" line.
	summaryWrite(t, testEpic, "decisions", "- Something to decide.\n")
	summaryWrite(t, testEpic, "decisions", "\n")
	stdout, code = runEpic(t, "summary", "read", testEpic)
	require.Equal(t, 0, code)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\nNone added.\n\n"+
		"## 000010_a\nA's plan.\n", stdout)
}

// Criterion: Rewriting one spec's section leaves every other section exactly
// as it was.
func TestEpicSummary_RewritingOneSectionLeavesTheOthers(t *testing.T) {
	root := summaryEpicProject(t)
	// A hand-written summary with an older created date and an ordering log,
	// neither of which a section write may disturb.
	path := summaryFilePath(root, testEpic)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\ncreated_date: \"2026-01-02\"\n---\n\n"+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\n- Keep or drop the flag?\n\n"+
		"## Order added for shared files\n- 000010_a before 000011_b: both change cmd/root.go\n\n"+
		"## 000010_a\nOld A.\n\n### Detail\nKept?\n\n"+
		"## 000011_b\nB stays.\n"), 0o644))

	summaryWrite(t, testEpic, "000010_a", "New A.\n")

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "---\ncreated_date: \"2026-01-02\"\n---\n\n"+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\n- Keep or drop the flag?\n\n"+
		"## Order added for shared files\n- 000010_a before 000011_b: both change cmd/root.go\n\n"+
		"## 000010_a\nNew A.\n\n"+
		"## 000011_b\nB stays.\n", string(raw),
		"only A's section changes; created_date, decisions, ordering and B are untouched")
}

// Criterion: Reading the summary of an epic that has none is refused with a
// message saying how to get one.
func TestEpicSummary_ReadWithNoSummaryIsRefused(t *testing.T) {
	root := summaryEpicProject(t)
	before := snapshotTree(t, root)

	er := refuseEpic(t, "summary", "read", testEpic)
	require.Equal(t, "epic_summary_not_found", er.Code)
	require.Equal(t, testEpic, er.Resource)
	require.Contains(t, er.NextAction, "plan this epic")
	require.Contains(t, er.NextAction, "spektacular epic summary write "+testEpic)
	require.Equal(t, before, snapshotTree(t, root))

	er = refuseEpic(t, "summary", "read", "000099_absent")
	require.Equal(t, "epic_not_found", er.Code)
}

// Criterion: Writing an unknown section, a spec outside the epic, or content
// that would break the layout is refused and changes nothing.
func TestEpicSummary_InvalidWritesAreRefusedAndChangeNothing(t *testing.T) {
	for _, tc := range []struct {
		name, data, body, code string
	}{
		{"unknown section", `{"section":"risks"}`, "Text.\n", "epic_summary_section_invalid"},
		{"spec outside the epic", `{"section":"000012_c"}`, "Text.\n", "epic_summary_section_invalid"},
		{"the ordering section", `{"section":"ordering"}`, "Text.\n", "epic_summary_section_invalid"},
		{"a level-2 heading in the body", `{"section":"000010_a"}`, "Intro.\n\n## Smuggled\nText.\n", "epic_summary_section_invalid"},
		{"a level-1 heading in the body", `{"section":"decisions"}`, "# Title\nText.\n", "epic_summary_section_invalid"},
		{"no section named", `{}`, "Text.\n", "bad_input"},
		{"malformed data", `{not json`, "Text.\n", "bad_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, withSummary := range []bool{false, true} {
				root := summaryEpicProject(t)
				if withSummary {
					summaryWrite(t, testEpic, "000010_a", "A's plan.\n")
				}
				before := snapshotTree(t, root)

				er := refuseEpic(t, "summary", "write", testEpic,
					"--data", tc.data, "--from", summaryBodyFile(t, tc.body))
				require.Equal(t, tc.code, er.Code)
				require.Equal(t, before, snapshotTree(t, root))
				if !withSummary {
					require.NoDirExists(t, filepath.Dir(summaryFilePath(root, testEpic)))
				}
			}
		})
	}

	t.Run("no --from", func(t *testing.T) {
		root := summaryEpicProject(t)
		before := snapshotTree(t, root)
		er := refuseEpic(t, "summary", "write", testEpic, "--data", `{"section":"decisions"}`)
		require.Equal(t, "epic_from_required", er.Code)
		require.Equal(t, before, snapshotTree(t, root))
	})

	t.Run("an epic that does not exist", func(t *testing.T) {
		root := summaryEpicProject(t)
		before := snapshotTree(t, root)
		er := refuseEpic(t, "summary", "write", "000099_absent",
			"--data", `{"section":"decisions"}`, "--from", summaryBodyFile(t, "Text.\n"))
		require.Equal(t, "epic_not_found", er.Code)
		require.Equal(t, before, snapshotTree(t, root))
	})

	t.Run("the refusal names the valid sections", func(t *testing.T) {
		summaryEpicProject(t)
		er := refuseEpic(t, "summary", "write", testEpic,
			"--data", `{"section":"000012_c"}`, "--from", summaryBodyFile(t, "Text.\n"))
		require.Equal(t, "000012_c", er.Resource)
		require.Contains(t, er.NextAction, "000010_a, 000011_b")
	})
}

// A section body may use ### and deeper headings.
func TestEpicSummary_DeeperHeadingsAreAccepted(t *testing.T) {
	summaryEpicProject(t)
	summaryWrite(t, testEpic, "000011_b", "### Steps\n#### Detail\nText.\n")
	stdout, code := runEpic(t, "summary", "read", testEpic)
	require.Equal(t, 0, code)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\nNone added.\n\n"+
		"## 000011_b\n### Steps\n#### Detail\nText.\n", stdout)
}

// Criterion: Deleting an epic also deletes its summary (and its folder).
func TestEpicDelete_AlsoDeletesTheSummaryAndItsFolder(t *testing.T) {
	root := summaryEpicProject(t)
	summaryWrite(t, testEpic, "decisions", "- Decide.\n")
	require.FileExists(t, summaryFilePath(root, testEpic))

	stdout, code := runEpic(t, "delete", testEpic)
	require.Equalf(t, 0, code, "delete failed: %s", stdout)
	var got epicDeleteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, epicDeleteResult{Name: testEpic, Deleted: true, Unlinked: []string{"000010_a", "000011_b"}}, got)

	require.NoFileExists(t, epicFilePath(root, testEpic))
	require.NoFileExists(t, summaryFilePath(root, testEpic))
	require.NoDirExists(t, filepath.Dir(summaryFilePath(root, testEpic)))
}

// A delete that fails part-way restores the summary along with the epic and
// every already-unlinked spec.
func TestEpicDelete_FailurePartWayRestoresTheSummary(t *testing.T) {
	root := summaryEpicProject(t)
	summaryWrite(t, testEpic, "000010_a", "A's plan.\n")
	summaryWrite(t, testEpic, "decisions", "- Decide.\n")
	before := snapshotTree(t, root)
	failEpicLinkOnCall(t, 2, nil)

	er := refuseEpic(t, "delete", testEpic)
	require.Equal(t, "epic_link_failed", er.Code)
	require.Equal(t, before, snapshotTree(t, root),
		"the epic, its summary and both specs must be back exactly as they were")
	require.FileExists(t, summaryFilePath(root, testEpic))
}

// `epic list` lists exactly the epics, not an epic's summary folder.
func TestEpicList_IgnoresTheSummaryFolder(t *testing.T) {
	summaryEpicProject(t)
	summaryWrite(t, testEpic, "decisions", "- Decide.\n")

	stdout, code := runEpic(t, "list")
	require.Equal(t, 0, code)
	var got struct {
		Files []map[string]string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Files, 1)
	require.Equal(t, testEpic, got.Files[0]["name"])
	require.Equal(t, "epics/"+testEpic+".md", got.Files[0]["path"])
}

// appendOrdering, used by `epic order`, adds to the ordering log and creates
// the summary when the epic has none, leaving every other section alone.
func TestEpicSummary_AppendOrdering(t *testing.T) {
	root := summaryEpicProject(t)
	cfg, st, err := epicStore()
	require.NoError(t, err)
	order := []string{"000010_a", "000011_b"}

	require.NoError(t, appendOrdering(newDocTxn(st), cfg, testEpic, order, []string{"- first"}))
	raw, err := os.ReadFile(summaryFilePath(root, testEpic))
	require.NoError(t, err)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\n- first\n", string(raw))

	summaryWrite(t, testEpic, "000011_b", "B's plan.\n")
	require.NoError(t, appendOrdering(newDocTxn(st), cfg, testEpic, order, []string{"- second", "- third"}))
	raw, err = os.ReadFile(summaryFilePath(root, testEpic))
	require.NoError(t, err)
	require.Equal(t, todayFrontmatter()+
		"# Planning summary: 000050_rollout\n\n"+
		"## Decisions\nNone.\n\n"+
		"## Order added for shared files\n- first\n- second\n- third\n\n"+
		"## 000011_b\nB's plan.\n", string(raw))
}
