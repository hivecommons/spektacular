package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This file tests `epic split` (cmd/epic_split.go): one complete spec turned
// into an epic of complete specs from a single staged JSON description. The
// contract held here is that the split is all-or-nothing: every new spec is
// written complete and final with a sequentially allocated name, the split
// spec is rewritten as its narrowed self keeping its provenance, the epic
// holds only an overview and its specs, every member is linked both ways, and
// a refusal or a failure part-way leaves the project byte-for-byte unchanged.
//
// Like cmd/epic_test.go, every command is driven through resetRootCmd +
// runRootCmd (via runEpic / refuseEpic), fixtures are written with
// os.WriteFile, and stored state is read back by scanning bytes by hand.

// epicSplitResult mirrors the `epic split` JSON envelope.
type epicSplitResult struct {
	Epic    string   `json:"epic"`
	Path    string   `json:"path"`
	Created []string `json:"created"`
	Linked  []string `json:"linked"`
}

// The spec being split in most tests: complete, final, carrying a design
// reference and two sources — one that moves to the epic and one that is only
// about the narrowed part and stays.
const (
	splitSpecName    = "000010_payments"
	splitSpecFixture = "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: final\n" +
		"designs:\n" +
		"    - source: api\n" +
		"      path: payments/v2.md\n" +
		"sources:\n" +
		"    - uri: https://example.com/issue/1\n" +
		"      retrieved_date: \"2026-06-01\"\n" +
		"    - uri: https://example.com/own\n" +
		"      retrieved_date: \"2026-06-02\"\n" +
		"---\n\n" +
		"# Feature: 000010_payments\n\nThe whole oversized payments spec.\n"
)

// sb builds one resulting spec's body for a description.
type sb = map[string]any

// splitDescription is the standard three-way split of splitSpecName used by
// the happy-path tests: the narrowed spec, "refunds" depending on it, and
// "reports" depending on refunds by title. Every requirement and criterion is
// distinct; "Use the shared ledger" is a constraint all three share.
func splitDescription() map[string]any {
	return map[string]any{
		"spec":     splitSpecName,
		"overview": "Take payments end to end.",
		"sources":  []map[string]string{{"uri": "https://example.com/issue/1"}},
		"specs": []map[string]any{
			{
				"name":       splitSpecName,
				"depends_on": []string{},
				"scope":      "Card capture.",
				"body": sb{
					"overview":            "Capture card payments. Nothing else.",
					"requirements":        []string{"Capture a card payment"},
					"acceptance_criteria": []string{"A captured payment is stored\nwith its amount and currency"},
					"constraints":         []string{"Use the shared ledger"},
					"technical_approach":  []string{"Reuse the gateway client"},
					"success_metrics":     []string{"Capture p99 under 200ms"},
					"non_goals":           []string{"Refunds are out of scope"},
				},
			},
			{
				"title":      "refunds",
				"depends_on": []string{splitSpecName},
				"scope":      "Refund flow.",
				"sources":    []map[string]string{{"uri": "https://example.com/refunds", "retrieved_date": "2026-05-05"}},
				"body": sb{
					"overview":            "Refund captured payments.",
					"requirements":        []string{"Refund a captured payment", "Partially refund a payment"},
					"acceptance_criteria": []string{"A refund reverses the ledger entry"},
					"constraints":         []string{"Use the shared ledger"},
					"technical_approach":  []string{"Drive refunds from the ledger"},
					"success_metrics":     []string{"Refund errors under 0.1%"},
					"non_goals":           []string{"Chargebacks are out of scope"},
				},
			},
			{
				"title":      "reports",
				"depends_on": []string{"refunds"},
				"body": sb{
					"overview":            "Report on payments daily. Exported as CSV.",
					"requirements":        []string{"Produce a daily report"},
					"acceptance_criteria": []string{"The report lists every payment and refund"},
					"constraints":         []string{"Use the shared ledger"},
				},
			},
		},
	}
}

// stageSplit writes a description outside the project (so a project snapshot
// does not include it) and returns its path.
func stageSplit(t *testing.T, desc any) string {
	t.Helper()
	raw, err := json.Marshal(desc)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "split.json")
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	return path
}

// stageSplitRaw writes literal bytes as the description.
func stageSplitRaw(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "split.json")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
	return path
}

// epicSplit runs an `epic split` expected to succeed.
func epicSplit(t *testing.T, desc any) epicSplitResult {
	t.Helper()
	stdout, code := runEpic(t, "split", "--from", stageSplit(t, desc))
	require.Equalf(t, 0, code, "epic split failed: %s", stdout)
	var got epicSplitResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

func specFilePath(root, name string) string {
	return filepath.Join(root, ".spektacular", "specs", name+".md")
}

// bodyOf returns everything after a stored document's frontmatter block and
// the blank line that follows it.
func bodyOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(raw)
	require.True(t, strings.HasPrefix(s, "---\n"))
	end := strings.Index(s[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0, "%s has no closing frontmatter fence", path)
	return strings.TrimPrefix(s[4+end+len("\n---\n"):], "\n")
}

// sectionLines returns the content lines of a markdown section — every
// non-blank line between heading and the next "## " heading, with HTML
// guidance comments skipped.
func sectionLines(t *testing.T, body, heading string) []string {
	t.Helper()
	lines := strings.Split(body, "\n")
	out := []string{}
	in, inComment, found := false, false, false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			in = line == heading
			found = found || in
			continue
		}
		if !in {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--") {
			inComment = !strings.HasSuffix(trimmed, "-->")
			continue
		}
		if inComment {
			if strings.HasSuffix(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if trimmed != "" {
			out = append(out, line)
		}
	}
	require.Truef(t, found, "section %q is missing", heading)
	return out
}

// fmValue scans a stored document's frontmatter for a top-level scalar key.
func fmValue(t *testing.T, path, key string) string {
	t.Helper()
	for _, line := range frontmatterOf(t, path) {
		if v, ok := strings.CutPrefix(line, key+": "); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// Criteria: splitting a standalone spec produces one epic named after it,
// listing it and the new specs with dependencies whose titles are rewritten
// to allocated names; the epic holds only an overview and its specs list;
// with counter IDs the new specs get distinct, sequential names; sources move
// from the split spec to the epic.
func TestEpicSplit_StandaloneSpecBecomesAnEpic(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, splitSpecName, splitSpecFixture)

	got := epicSplit(t, splitDescription())
	require.Equal(t, epicSplitResult{
		Epic:    splitSpecName,
		Path:    "epics/" + splitSpecName + ".md",
		Created: []string{"000011_refunds", "000012_reports"},
		Linked:  []string{splitSpecName, "000011_refunds", "000012_reports"},
	}, got)

	entries, err := os.ReadDir(filepath.Join(root, ".spektacular", "epics"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "a split creates exactly one epic")

	ep := epicFilePath(root, splitSpecName)
	today := time.Now().UTC().Format("2006-01-02")
	require.Equal(t, []string{
		"created_date: \"" + today + "\"",
		"document_status: draft",
		"spec: " + splitSpecName,
		"specs:",
		"    - name: " + splitSpecName,
		"      depends_on: []",
		"    - name: 000011_refunds",
		"      depends_on:",
		"        - " + splitSpecName,
		"    - name: 000012_reports",
		"      depends_on:",
		"        - 000011_refunds",
		"sources:",
		"    - uri: https://example.com/issue/1",
		"      retrieved_date: \"" + today + "\"",
	}, frontmatterOf(t, ep))

	require.Equal(t,
		"# Epic: 000010_payments\n\n"+
			"## Overview\n\nTake payments end to end.\n\n"+
			"## Specs\n\n"+
			"| # | Spec | Scope |\n|---|---|---|\n"+
			"| 1 | 000010_payments | Card capture. |\n"+
			"| 2 | 000011_refunds | Refund flow. |\n"+
			"| 3 | 000012_reports | Report on payments daily. |\n",
		bodyOf(t, ep))
	epicBody := bodyOf(t, ep)
	for _, h := range []string{"## Requirements", "## Acceptance Criteria", "## Constraints", "## Non-Goals", "## Technical Approach", "## Success Metrics"} {
		require.NotContains(t, epicBody, h, "an epic holds only an overview and its specs")
	}

	requireAgreement(t, root, splitSpecName, splitSpecName, "000011_refunds", "000012_reports")

	// The moved source left the narrowed spec; the one about it alone stayed.
	fm := strings.Join(frontmatterOf(t, specFilePath(root, splitSpecName)), "\n")
	require.NotContains(t, fm, "https://example.com/issue/1")
	require.Contains(t, fm, "sources:\n    - uri: https://example.com/own\n      retrieved_date: \"2026-06-02\"")
}

// Criterion: every resulting spec is complete and final, with every section
// it was given, a non-empty overview and at least one acceptance criterion;
// the narrowed spec keeps its created_date and designs.
func TestEpicSplit_EveryResultingSpecIsCompleteAndFinal(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, splitSpecName, splitSpecFixture)
	epicSplit(t, splitDescription())
	today := time.Now().UTC().Format("2006-01-02")

	type want struct {
		created  string
		sections map[string][]string
	}
	for name, w := range map[string]want{
		splitSpecName: {created: "2026-07-01", sections: map[string][]string{
			"## Overview":            {"Capture card payments. Nothing else."},
			"## Requirements":        {"- [ ] **Capture a card payment**"},
			"## Constraints":         {"- Use the shared ledger"},
			"## Acceptance Criteria": {"- [ ] **A captured payment is stored**", "  with its amount and currency"},
			"## Technical Approach":  {"- Reuse the gateway client"},
			"## Success Metrics":     {"- Capture p99 under 200ms"},
			"## Non-Goals":           {"- Refunds are out of scope"},
		}},
		"000011_refunds": {created: today, sections: map[string][]string{
			"## Overview":            {"Refund captured payments."},
			"## Requirements":        {"- [ ] **Refund a captured payment**", "- [ ] **Partially refund a payment**"},
			"## Constraints":         {"- Use the shared ledger"},
			"## Acceptance Criteria": {"- [ ] **A refund reverses the ledger entry**"},
			"## Technical Approach":  {"- Drive refunds from the ledger"},
			"## Success Metrics":     {"- Refund errors under 0.1%"},
			"## Non-Goals":           {"- Chargebacks are out of scope"},
		}},
		"000012_reports": {created: today, sections: map[string][]string{
			"## Overview":            {"Report on payments daily. Exported as CSV."},
			"## Requirements":        {"- [ ] **Produce a daily report**"},
			"## Constraints":         {"- Use the shared ledger"},
			"## Acceptance Criteria": {"- [ ] **The report lists every payment and refund**"},
			"## Technical Approach":  {},
			"## Success Metrics":     {},
			"## Non-Goals":           {},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			path := specFilePath(root, name)
			require.Equal(t, "final", fmValue(t, path, "document_status"))
			require.Equal(t, w.created, fmValue(t, path, "created_date"))
			require.Equal(t, splitSpecName, fmValue(t, path, "epic"))
			body := bodyOf(t, path)
			require.True(t, strings.HasPrefix(body, "# Feature: "+name+"\n"), "body opens with the spec's title, got %q", body)
			for heading, lines := range w.sections {
				require.Equalf(t, lines, sectionLines(t, body, heading), "section %s", heading)
			}
		})
	}

	fm := strings.Join(frontmatterOf(t, specFilePath(root, splitSpecName)), "\n")
	require.Contains(t, fm, "designs:\n    - source: api\n      path: payments/v2.md",
		"the narrowed spec keeps its design references")
	require.Contains(t, strings.Join(frontmatterOf(t, specFilePath(root, "000011_refunds")), "\n"),
		"sources:\n    - uri: https://example.com/refunds\n      retrieved_date: \"2026-05-05\"",
		"a new spec carries the sources it was given")
}

// Criterion: every requirement and acceptance criterion in the description
// appears in exactly one resulting spec, and a constraint shared by several
// appears in each of them.
func TestEpicSplit_ContentLandsExactlyWhereItWasPlaced(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, splitSpecName, splitSpecFixture)
	epicSplit(t, splitDescription())

	bodies := map[string]string{}
	for _, n := range []string{splitSpecName, "000011_refunds", "000012_reports"} {
		bodies[n] = bodyOf(t, specFilePath(root, n))
	}
	holders := func(text string) []string {
		out := []string{}
		for _, n := range []string{splitSpecName, "000011_refunds", "000012_reports"} {
			if strings.Contains(bodies[n], text) {
				out = append(out, n)
			}
		}
		return out
	}
	for text, where := range map[string]string{
		"Capture a card payment":                    splitSpecName,
		"A captured payment is stored":              splitSpecName,
		"Refund a captured payment":                 "000011_refunds",
		"Partially refund a payment":                "000011_refunds",
		"A refund reverses the ledger entry":        "000011_refunds",
		"Produce a daily report":                    "000012_reports",
		"The report lists every payment and refund": "000012_reports",
	} {
		require.Equalf(t, []string{where}, holders(text), "%q must appear in exactly one spec", text)
	}
	require.Equal(t, []string{splitSpecName, "000011_refunds", "000012_reports"}, holders("- Use the shared ledger"),
		"a shared constraint appears in every spec that was given it")
	for n, b := range bodies {
		require.Equalf(t, 1, strings.Count(b, "- Use the shared ledger"), "spec %s", n)
	}
}

// Criterion: splitting a spec that already belongs to an epic adds the new
// specs to that same epic; no second epic is created and the existing
// members, overview and table scopes are kept.
func TestEpicSplit_SpecInAnEpicExtendsThatEpic(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, `{"specs":[{"name":"000010_a","depends_on":[]},{"name":"000011_b","depends_on":["000010_a"]}]}`)

	got := epicSplit(t, map[string]any{
		"spec": "000010_a",
		"specs": []map[string]any{
			{"name": "000010_a", "depends_on": []string{}, "body": sb{"overview": "Narrowed a.", "acceptance_criteria": []string{"a works"}}},
			{"title": "c", "depends_on": []string{"000010_a"}, "body": sb{"overview": "Split-off c. More.", "acceptance_criteria": []string{"c works"}}},
		},
	})
	require.Equal(t, epicSplitResult{
		Epic:    testEpic,
		Path:    "epics/" + testEpic + ".md",
		Created: []string{"000012_c"},
		Linked:  []string{"000012_c"},
	}, got)

	entries, err := os.ReadDir(filepath.Join(root, ".spektacular", "epics"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no second epic may be created")
	require.NoFileExists(t, epicFilePath(root, "000010_a"))

	ep := epicFilePath(root, testEpic)
	require.Equal(t, []string{"000010_a", "000011_b", "000012_c"}, epicSpecsOf(t, ep))
	require.Contains(t, strings.Join(frontmatterOf(t, ep), "\n"),
		"    - name: 000011_b\n      depends_on:\n        - 000010_a\n    - name: 000012_c\n      depends_on:\n        - 000010_a")
	require.Equal(t,
		"# Epic: "+testEpic+"\n\n"+
			"## Overview\n\nRoll the feature out in stages.\n\n"+
			"## Specs\n\n"+
			"| # | Spec | Scope |\n|---|---|---|\n"+
			"| 1 | 000010_a | Narrowed a. |\n"+
			"| 2 | 000011_b |  |\n"+
			"| 3 | 000012_c | Split-off c. |\n",
		bodyOf(t, ep))
	requireAgreement(t, root, testEpic, "000010_a", "000011_b", "000012_c")
	require.Equal(t, "final", fmValue(t, specFilePath(root, "000012_c"), "document_status"))
}

// Criterion: with counter IDs the new specs get distinct, sequential names,
// continuing from the highest ID already in the spec folder.
func TestEpicSplit_CounterIDsAreSequential(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, splitSpecName, splitSpecFixture)
	writeSpecFixture(t, root, "000020_other", epicTestSpecFixed)

	desc := splitDescription()
	specs := desc["specs"].([]map[string]any)
	specs = append(specs, map[string]any{
		"title": "exports", "depends_on": []string{"reports"},
		"body": sb{"overview": "Export reports.", "acceptance_criteria": []string{"Reports export"}},
	})
	desc["specs"] = specs

	got := epicSplit(t, desc)
	require.Equal(t, []string{"000021_refunds", "000022_reports", "000023_exports"}, got.Created)
	for _, n := range got.Created {
		require.FileExists(t, specFilePath(root, n))
	}
	require.Contains(t, strings.Join(frontmatterOf(t, epicFilePath(root, splitSpecName)), "\n"),
		"    - name: 000023_exports\n      depends_on:\n        - 000022_reports")
}

// Criterion: a description that cannot be split is refused with a code and a
// next step, and nothing is written anywhere in the project.
func TestEpicSplit_InvalidDescriptionIsRefusedAndWritesNothing(t *testing.T) {
	narrowed := func(deps ...string) map[string]any {
		if deps == nil {
			deps = []string{}
		}
		return map[string]any{"name": splitSpecName, "depends_on": deps, "body": sb{"overview": "N.", "acceptance_criteria": []string{"n"}}}
	}
	newSpec := func(title string, deps ...string) map[string]any {
		if deps == nil {
			deps = []string{}
		}
		return map[string]any{"title": title, "depends_on": deps, "body": sb{"overview": title + ".", "acceptance_criteria": []string{title + " works"}}}
	}
	desc := func(specs ...map[string]any) map[string]any {
		return map[string]any{"spec": splitSpecName, "overview": "O.", "specs": specs}
	}

	for _, tc := range []struct {
		name, code, msg string
		desc            any
		raw             string
	}{
		{name: "fewer than two specs", code: "epic_split_invalid", msg: "at least two resulting specs", desc: desc(narrowed())},
		{name: "a spec with no acceptance criteria", code: "epic_split_invalid", msg: "no acceptance criteria",
			desc: desc(narrowed(), map[string]any{"title": "x", "depends_on": []string{}, "body": sb{"overview": "X.", "acceptance_criteria": []string{"  "}}})},
		{name: "a spec with an empty overview", code: "epic_split_invalid", msg: "empty overview",
			desc: desc(narrowed(), map[string]any{"title": "x", "depends_on": []string{}, "body": sb{"acceptance_criteria": []string{"x"}}})},
		{name: "a cycle through titles", code: "epic_invalid", msg: "dependency cycle",
			desc: desc(narrowed(), newSpec("a", "b"), newSpec("b", "a"))},
		{name: "a dependency on an unknown title", code: "epic_invalid", msg: "not a spec in this epic",
			desc: desc(narrowed(), newSpec("a", "nope"))},
		{name: "no overview when creating the epic", code: "epic_split_invalid", msg: `no "overview"`,
			desc: map[string]any{"spec": splitSpecName, "specs": []map[string]any{narrowed(), newSpec("a")}}},
		{name: "an unknown split spec", code: "epic_split_invalid", msg: `no spec named "000099_missing"`,
			desc: map[string]any{"spec": "000099_missing", "overview": "O.", "specs": []map[string]any{
				{"name": "000099_missing", "depends_on": []string{}, "body": sb{"overview": "N.", "acceptance_criteria": []string{"n"}}}, newSpec("a")}}},
		{name: "the split spec missing from specs", code: "epic_split_invalid", msg: "must appear exactly once",
			desc: desc(newSpec("a"), newSpec("b"))},
		{name: "a new spec given a name", code: "epic_split_invalid", msg: "only the spec being split",
			desc: desc(narrowed(), map[string]any{"name": "000030_x", "depends_on": []string{}, "body": sb{"overview": "X.", "acceptance_criteria": []string{"x"}}})},
		{name: "a duplicate title", code: "epic_split_invalid", msg: "used by more than one new spec",
			desc: desc(narrowed(), newSpec("a"), newSpec("a"))},
		{name: "no spec named", code: "epic_split_invalid", msg: `no "spec"`,
			desc: map[string]any{"overview": "O.", "specs": []map[string]any{narrowed(), newSpec("a")}}},
		{name: "a source with no uri", code: "sources_invalid", msg: "has no uri",
			desc: map[string]any{"spec": splitSpecName, "overview": "O.", "sources": []map[string]string{{"uri": ""}}, "specs": []map[string]any{narrowed(), newSpec("a")}}},
		{name: "not JSON", code: "epic_split_invalid", msg: "not valid JSON", raw: "{not json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := epicProject(t)
			writeSpecFixture(t, root, splitSpecName, splitSpecFixture)
			before := snapshotTree(t, root)

			from := ""
			if tc.raw != "" {
				from = stageSplitRaw(t, tc.raw)
			} else {
				from = stageSplit(t, tc.desc)
			}
			er := refuseEpic(t, "split", "--from", from)
			require.Equal(t, tc.code, er.Code)
			require.Contains(t, er.Message, tc.msg)
			require.Equal(t, before, snapshotTree(t, root), "a refused split must write nothing")
			require.NoDirExists(t, filepath.Join(root, ".spektacular", "epics"))
		})
	}
}

// Refusals that are not about the description's content still name a next
// step: a missing --from, and the split description refusal points at the
// schema.
func TestEpicSplit_RefusalsCarryNextAction(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, splitSpecName, splitSpecFixture)

	er := refuseEpic(t, "split")
	require.Equal(t, "epic_from_required", er.Code)
	require.Contains(t, er.NextAction, "spektacular epic split --from")

	er = refuseEpic(t, "split", "--from", stageSplit(t, map[string]any{"spec": splitSpecName, "specs": []any{}}))
	require.Equal(t, "epic_split_invalid", er.Code)
	require.Equal(t, "fix the staged split description and reissue `spektacular epic split --from <that file>`; run `spektacular epic split --schema` for its shape", er.NextAction)

	er = refuseEpic(t, "split", "--from", stageSplit(t, map[string]any{"spec": "000099_missing", "overview": "O.", "specs": []map[string]any{
		{"name": "000099_missing", "depends_on": []string{}, "body": sb{"overview": "N.", "acceptance_criteria": []string{"n"}}},
		{"title": "a", "depends_on": []string{}, "body": sb{"overview": "A.", "acceptance_criteria": []string{"a"}}},
	}}))
	require.Equal(t, "000099_missing", er.Resource)
	require.Contains(t, er.NextAction, "spektacular spec file list")
}

// A failure part-way — after the new specs, the narrowed spec and the epic are
// written — restores everything: new spec files removed, the narrowed spec
// back byte-for-byte, and no epic file.
func TestEpicSplit_FailurePartWayRestoresEverything(t *testing.T) {
	for _, n := range []int{1, 3} {
		t.Run("new epic, link write "+string(rune('0'+n))+" fails", func(t *testing.T) {
			root := epicProject(t)
			writeSpecFixture(t, root, splitSpecName, splitSpecFixture)
			before := snapshotTree(t, root)
			failEpicLinkOnCall(t, n, nil)

			er := refuseEpic(t, "split", "--from", stageSplit(t, splitDescription()))
			require.Equal(t, "epic_link_failed", er.Code)
			require.Contains(t, er.Message, errForcedEpicLink.Error())
			require.Contains(t, er.NextAction, "reissue the same command")

			require.Equal(t, before, snapshotTree(t, root))
			require.NoFileExists(t, specFilePath(root, "000011_refunds"))
			require.NoFileExists(t, specFilePath(root, "000012_reports"))
			require.NoFileExists(t, epicFilePath(root, splitSpecName))
			raw, err := os.ReadFile(specFilePath(root, splitSpecName))
			require.NoError(t, err)
			require.Equal(t, splitSpecFixture, string(raw))
		})
	}

	t.Run("existing epic is restored", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
		writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
		epicWrite(t, testEpic, specsData("000010_a", "000011_b"))
		before := snapshotTree(t, root)
		failEpicLinkOnCall(t, 2, nil)

		er := refuseEpic(t, "split", "--from", stageSplit(t, map[string]any{
			"spec": "000010_a",
			"specs": []map[string]any{
				{"name": "000010_a", "depends_on": []string{}, "body": sb{"overview": "A.", "acceptance_criteria": []string{"a"}}},
				{"title": "c", "depends_on": []string{}, "body": sb{"overview": "C.", "acceptance_criteria": []string{"c"}}},
				{"title": "d", "depends_on": []string{"c"}, "body": sb{"overview": "D.", "acceptance_criteria": []string{"d"}}},
			},
		}))
		require.Equal(t, "epic_link_failed", er.Code)
		require.Equal(t, before, snapshotTree(t, root),
			"the epic, the narrowed spec and every new spec must be back exactly as they were")
	})
}

// `epic split --schema` publishes the description's shape and the result's.
func TestEpicSplit_Schema(t *testing.T) {
	epicProject(t)
	stdout, code := runEpic(t, "split", "--schema")
	require.Equal(t, 0, code)

	var got struct {
		Input struct {
			Properties map[string]struct {
				Items struct {
					Properties map[string]struct {
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"properties"`
			Required []string `json:"required"`
		} `json:"input"`
		Output struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"output"`
		Flags map[string]json.RawMessage `json:"flags"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))

	keys := func(m map[string]json.RawMessage) []string {
		out := []string{}
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	inKeys := []string{}
	for k := range got.Input.Properties {
		inKeys = append(inKeys, k)
	}
	require.ElementsMatch(t, []string{"spec", "overview", "sources", "specs", "confirm_completed_epic"}, inKeys)
	require.ElementsMatch(t, []string{"spec", "specs"}, got.Input.Required)

	specKeys := []string{}
	for k := range got.Input.Properties["specs"].Items.Properties {
		specKeys = append(specKeys, k)
	}
	require.ElementsMatch(t, []string{"name", "title", "id", "depends_on", "scope", "sources", "body"}, specKeys)
	require.ElementsMatch(t,
		[]string{"overview", "requirements", "acceptance_criteria", "constraints", "technical_approach", "success_metrics", "non_goals"},
		keys(got.Input.Properties["specs"].Items.Properties["body"].Properties))

	require.ElementsMatch(t, []string{"epic", "path", "created", "linked"}, keys(got.Output.Properties))
	require.ElementsMatch(t, []string{"from"}, keys(got.Flags))
}

// Splitting a spec keeps another member's parallel_with, recorded when a
// dependency was undone at review, so `epic order` still never re-adds it.
func TestEpicSplit_KeepsAnotherSpecsParallelWith(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, `{"specs":[{"name":"000010_a","depends_on":[]},{"name":"000011_b","depends_on":["000010_a"]}]}`)
	stdout, code := runEpic(t, "order", testEpic, "--data", `{"unorder":{"spec":"000011_b","depends_on":"000010_a"}}`)
	require.Equalf(t, 0, code, "unorder failed: %s", stdout)

	epicSplit(t, map[string]any{
		"spec": "000010_a",
		"specs": []map[string]any{
			{"name": "000010_a", "depends_on": []string{}, "body": sb{"overview": "Narrowed a.", "acceptance_criteria": []string{"a works"}}},
			{"title": "c", "depends_on": []string{"000010_a"}, "body": sb{"overview": "Split-off c. More.", "acceptance_criteria": []string{"c works"}}},
		},
	})

	require.Contains(t, strings.Join(frontmatterOf(t, epicFilePath(root, testEpic)), "\n"),
		"    - name: 000010_a\n      depends_on: []\n"+
			"    - name: 000011_b\n      depends_on: []\n      parallel_with:\n        - 000010_a\n"+
			"    - name: 000012_c\n      depends_on:\n        - 000010_a")
}

// Splitting the spec that carries parallel_with keeps it on that spec, so a
// later `epic order` does not put back the dependency the user removed.
func TestEpicSplit_KeepsTheSplitSpecsOwnParallelWith(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000010_a", epicTestSpecFixed)
	writeSpecFixture(t, root, "000011_b", epicTestSpecFixed)
	epicWrite(t, testEpic, `{"specs":[{"name":"000010_a","depends_on":[]},{"name":"000011_b","depends_on":["000010_a"]}]}`)
	stdout, code := runEpic(t, "order", testEpic, "--data", `{"unorder":{"spec":"000011_b","depends_on":"000010_a"}}`)
	require.Equalf(t, 0, code, "unorder failed: %s", stdout)

	epicSplit(t, map[string]any{
		"spec": "000011_b",
		"specs": []map[string]any{
			{"name": "000011_b", "depends_on": []string{}, "body": sb{"overview": "Narrowed b.", "acceptance_criteria": []string{"b works"}}},
			{"title": "c", "depends_on": []string{"000011_b"}, "body": sb{"overview": "Split-off c. More.", "acceptance_criteria": []string{"c works"}}},
		},
	})

	require.Contains(t, strings.Join(frontmatterOf(t, epicFilePath(root, testEpic)), "\n"),
		"    - name: 000010_a\n      depends_on: []\n"+
			"    - name: 000011_b\n      depends_on: []\n      parallel_with:\n        - 000010_a\n"+
			"    - name: 000012_c\n      depends_on:\n        - 000011_b")
}
