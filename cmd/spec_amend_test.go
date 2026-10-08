package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// specAmendName is the planned spec every spec amend fixture stores.
const specAmendName = "000070_billing"

// specAmendFrontmatter is the fixture spec's frontmatter: lifecycle dates and
// status, an epic, a seeding source and three design references — one that
// resolves, one whose document is missing and one whose source is undeclared.
const specAmendFrontmatter = "---\n" +
	"created_date: 2026-07-01\n" +
	"document_status: final\n" +
	"designs:\n" +
	"    - source: api\n      path: payments/v2.md\n" +
	"    - source: api\n      path: missing.md\n" +
	"    - source: ghost\n      path: old.md\n" +
	"epic: 000068_payments\n" +
	"sources:\n" +
	"    - uri: https://example.com/issue/1\n      retrieved_date: \"2026-06-30\"\n" +
	"---\n\n"

// specAmendBody is the fixture spec's body, with every amendable section, a
// preamble and two sections an implement run may not amend.
const specAmendBody = "# Billing\n\nCharge customers for their usage.\n\n" +
	"## Overview\n\nThe billing overview.\n\n" +
	"## Requirements\n\n- [ ] Charge cards monthly\n\n" +
	"## Acceptance Criteria\n\n- [x] A card is charged on the first of the month\n\n" +
	"## Constraints\n\n- Card data never leaves the PCI zone\n\n" +
	"## Success Metrics\n\n- 99% of charges succeed first time\n\n" +
	"## Non-Goals\n\n- Refunds\n"

// specAmendResult mirrors spec amend's success output.
type specAmendResult struct {
	Spec            string              `json:"spec"`
	AmendedSections []string            `json:"amended_sections"`
	Design          *metadata.DesignRef `json:"design"`
	RecordedAt      string              `json:"recorded_at"`
	Hash            string              `json:"hash"`
	DryRun          bool                `json:"dry_run"`
	Error           bool                `json:"error"`
}

// specAmendProject lays out a project in a t.TempDir() and chdirs into it: an
// "api" design source holding payments/v2.md and overview.md, the fixture
// spec and a plan for it. It returns the project root and the spec's path.
func specAmendProject(t *testing.T) (root, specPath string) {
	t.Helper()
	root = t.TempDir()
	t.Chdir(root)
	apiLoc := filepath.Join(root, "docs", "design")
	seedDesignDoc(t, apiLoc, "payments/v2.md", "# Payments v2\n")
	seedDesignDoc(t, apiLoc, "overview.md", "# Overview\n")
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})
	specPath = writeSpecFixture(t, root, specAmendName, specAmendFrontmatter+specAmendBody)
	writeFixturePlan(t, filepath.Join(root, ".spektacular"), specAmendName)
	return root, specPath
}

// stageSpecAmend writes content to a file in its own temp folder, outside the
// project, and returns its path for --from.
func stageSpecAmend(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "amended.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// specAmendData renders spec amend's --data payload, with design appended
// verbatim as a JSON fragment when non-empty.
func specAmendData(name, reason, run, design string) string {
	b, _ := json.Marshal(map[string]string{"name": name, "reason": reason, "run": run})
	if design == "" {
		return string(b)
	}
	return strings.TrimSuffix(string(b), "}") + `,"design":` + design + "}"
}

// amendSpecOK runs `spec amend` with args and decodes a successful result.
func amendSpecOK(t *testing.T, args ...string) specAmendResult {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, append([]string{"spec", "amend"}, args...)...)
	require.Equalf(t, 0, code, "stdout: %s", stdout)
	require.Empty(t, stderr)
	var got specAmendResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.False(t, got.Error)
	return got
}

// refuseSpecAmend runs `spec amend` with args, asserts it fails, and decodes
// the error response.
func refuseSpecAmend(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, append([]string{"spec", "amend"}, args...)...)
	require.Equalf(t, 1, code, "stdout: %s", stdout)
	require.Empty(t, stderr)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// readSpecViaCLI runs `spec file read` for the fixture spec and returns its
// output, the stored spec with its frontmatter.
func readSpecViaCLI(t *testing.T) string {
	t.Helper()
	resetRootCmd(t)
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "read", specAmendName})
	require.NoError(t, rootCmd.Execute())
	return stdout.String()
}

// splitStoredSpec reads the stored spec at path and returns its frontmatter
// and body.
func splitStoredSpec(t *testing.T, path string) (*metadata.Metadata, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	fm, body, err := metadata.Split(raw)
	require.NoError(t, err)
	require.NotNil(t, fm)
	return fm, string(body)
}

// recordedDay returns the YYYY-MM-DD day of an RFC3339 recorded_at value.
func recordedDay(t *testing.T, at string) string {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, at)
	require.NoError(t, err)
	return ts.Format(metadata.DateFormat)
}

// Criterion: amending a success metric leaves the spec holding the new text,
// plus an Amendments entry with the date, section, reason and run, and one
// metadata record whose hash is that of the stored body. The output's
// amended_sections is worked out by the CLI, not supplied.
func TestSpecAmend_SuccessMetricAmendment(t *testing.T) {
	_, specPath := specAmendProject(t)
	amended := strings.Replace(specAmendBody, "99% of charges succeed first time", "97% of charges succeed first time", 1)
	staged := stageSpecAmend(t, amended)

	got := amendSpecOK(t, "--data", specAmendData(specAmendName, "99% is unreachable  with the\n current gateway", "interactive implement run, task 2.1", ""), "--from", staged)
	require.Equal(t, specAmendName, got.Spec)
	require.Equal(t, []string{"Success Metrics"}, got.AmendedSections)
	require.Nil(t, got.Design)
	require.False(t, got.DryRun)

	fm, body := splitStoredSpec(t, specPath)
	day := recordedDay(t, got.RecordedAt)
	want := amended + "\n## Amendments\n\n- **" + day + ": Success Metrics** (interactive implement run, task 2.1)\n  99% is unreachable with the current gateway\n"
	require.Equal(t, want, body)

	require.Len(t, fm.Amendments, 1)
	rec := fm.Amendments[0]
	require.Equal(t, got.RecordedAt, rec.At)
	require.Equal(t, []string{"Success Metrics"}, rec.Sections)
	require.Nil(t, rec.Design)
	require.Equal(t, metadata.BodyHash([]byte(body)), rec.Hash)
	require.Equal(t, rec.Hash, got.Hash)
}

// Criterion: recording a design revision adds an Amendments entry naming the
// design and leaves the rest of the spec unchanged; the record carries the
// design and no sections.
func TestSpecAmend_DesignRecordOnly(t *testing.T) {
	_, specPath := specAmendProject(t)

	got := amendSpecOK(t, "--data", specAmendData(specAmendName, "the v2 request shape gained a currency", "epic run, task 1.2", `{"source":"api","path":"payments/v2.md"}`))
	require.Equal(t, []string{}, got.AmendedSections)
	require.Equal(t, &metadata.DesignRef{Source: "api", Path: "payments/v2.md"}, got.Design)

	fm, body := splitStoredSpec(t, specPath)
	day := recordedDay(t, got.RecordedAt)
	want := specAmendBody + "\n## Amendments\n\n- **" + day + ": design api:payments/v2.md** (epic run, task 1.2)\n  the v2 request shape gained a currency\n"
	require.Equal(t, want, body)

	require.Len(t, fm.Amendments, 1)
	require.Empty(t, fm.Amendments[0].Sections)
	require.Equal(t, &metadata.DesignRef{Source: "api", Path: "payments/v2.md"}, fm.Amendments[0].Design)
	require.Equal(t, metadata.BodyHash([]byte(body)), fm.Amendments[0].Hash)
}

// Criterion: a spec change and a design revision recorded in one call produce
// a single entry naming both.
func TestSpecAmend_SpecAndDesignInOneCall(t *testing.T) {
	_, specPath := specAmendProject(t)
	amended := strings.Replace(specAmendBody, "- Card data never leaves the PCI zone", "- Card data never leaves the PCI zone\n- Amounts carry a currency", 1)

	got := amendSpecOK(t,
		"--data", specAmendData(specAmendName, "currency was missing", "run 7", `{"source":"api","path":"payments/v2.md"}`),
		"--from", stageSpecAmend(t, amended))
	require.Equal(t, []string{"Constraints"}, got.AmendedSections)

	fm, body := splitStoredSpec(t, specPath)
	day := recordedDay(t, got.RecordedAt)
	require.Equal(t, amended+"\n## Amendments\n\n- **"+day+": Constraints; design api:payments/v2.md** (run 7)\n  currency was missing\n", body)
	require.Equal(t, 1, strings.Count(body, "- **"))
	require.Len(t, fm.Amendments, 1)
	require.Equal(t, []string{"Constraints"}, fm.Amendments[0].Sections)
	require.Equal(t, &metadata.DesignRef{Source: "api", Path: "payments/v2.md"}, fm.Amendments[0].Design)
}

// Criterion: a second amendment, staged from `spec file read` output, keeps the
// first entry and appends its own, and the frontmatter holds both records.
func TestSpecAmend_SecondAmendmentAppends(t *testing.T) {
	_, specPath := specAmendProject(t)
	first := strings.Replace(specAmendBody, "99% of charges", "97% of charges", 1)
	got1 := amendSpecOK(t, "--data", specAmendData(specAmendName, "first reason", "run 1", ""), "--from", stageSpecAmend(t, first))

	current := readSpecViaCLI(t)
	require.True(t, strings.HasPrefix(current, "---\n"), "spec file read returns the frontmatter too")
	second := strings.Replace(current, "- [ ] Charge cards monthly", "- [ ] Charge cards weekly", 1)
	got2 := amendSpecOK(t, "--data", specAmendData(specAmendName, "second reason", "run 2", ""), "--from", stageSpecAmend(t, second))
	require.Equal(t, []string{"Requirements"}, got2.AmendedSections)

	fm, body := splitStoredSpec(t, specPath)
	wantBody := strings.Replace(first, "Charge cards monthly", "Charge cards weekly", 1) +
		"\n## Amendments\n\n" +
		"- **" + recordedDay(t, got1.RecordedAt) + ": Success Metrics** (run 1)\n  first reason\n" +
		"- **" + recordedDay(t, got2.RecordedAt) + ": Requirements** (run 2)\n  second reason\n"
	require.Equal(t, wantBody, body)

	require.Len(t, fm.Amendments, 2)
	require.Equal(t, []string{"Success Metrics"}, fm.Amendments[0].Sections)
	require.Equal(t, got1.Hash, fm.Amendments[0].Hash)
	require.Equal(t, []string{"Requirements"}, fm.Amendments[1].Sections)
	require.Equal(t, metadata.BodyHash([]byte(body)), fm.Amendments[1].Hash)
}

// Criterion: every refusal names its code and a corrective next step, and
// leaves the stored spec untouched.
func TestSpecAmend_Refusals(t *testing.T) {
	const run = "run 1"
	const reason = "a reason"
	validFrom := func(t *testing.T) string {
		return stageSpecAmend(t, strings.Replace(specAmendBody, "99%", "97%", 1))
	}
	cases := []struct {
		name string
		code string
		// setup prepares the project beyond specAmendProject and returns the
		// command's arguments.
		setup func(t *testing.T, root, specPath string) []string
	}{
		{"malformed JSON", "bad_input", func(t *testing.T, _, _ string) []string {
			return []string{"--data", "{not json", "--from", validFrom(t)}
		}},
		{"invalid name", "bad_input", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData("Bad Name", reason, run, ""), "--from", validFrom(t)}
		}},
		{"missing reason", "spec_amend_reason_required", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, "  ", run, ""), "--from", validFrom(t)}
		}},
		{"missing run", "spec_amend_run_required", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, reason, "", ""), "--from", validFrom(t)}
		}},
		{"nothing to record", "spec_amend_nothing_to_record", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, reason, run, "")}
		}},
		{"unknown spec", "spec_not_found", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData("000099_nothing", reason, run, ""), "--from", validFrom(t)}
		}},
		{"spec has no plan", "spec_amend_no_plan", func(t *testing.T, root, _ string) []string {
			require.NoError(t, os.RemoveAll(filepath.Join(root, ".spektacular", "plans", specAmendName)))
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", validFrom(t)}
		}},
		{"checkbox and whitespace only", "spec_amend_no_change", func(t *testing.T, _, _ string) []string {
			staged := strings.Replace(specAmendBody, "- [ ] Charge", "- [x] Charge", 1)
			staged = strings.Replace(staged, "- [x] A card", "- [ ] A card", 1)
			staged = strings.ReplaceAll(staged, "\n", "\r\n") + "\n\n"
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"overview changed", "spec_amend_section_not_amendable", func(t *testing.T, _, _ string) []string {
			staged := strings.Replace(specAmendBody, "The billing overview.", "A new overview.", 1)
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"non-goals changed alongside an amendable section", "spec_amend_section_not_amendable", func(t *testing.T, _, _ string) []string {
			staged := strings.Replace(specAmendBody, "- Refunds", "- Refunds\n- Disputes", 1)
			staged = strings.Replace(staged, "99%", "97%", 1)
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"overview moved alongside an amendable change", "spec_amend_section_not_amendable", func(t *testing.T, _, _ string) []string {
			staged := strings.Replace(specAmendBody, "## Overview\n\nThe billing overview.\n\n", "", 1)
			staged = strings.Replace(staged, "99%", "97%", 1)
			staged = strings.Replace(staged, "## Non-Goals", "## Overview\n\nThe billing overview.\n\n## Non-Goals", 1)
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"preamble changed", "spec_amend_section_not_amendable", func(t *testing.T, _, _ string) []string {
			staged := strings.Replace(specAmendBody, "Charge customers for their usage.", "Charge customers.", 1)
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"Amendments section hand-added", "spec_amend_section_not_amendable", func(t *testing.T, _, _ string) []string {
			staged := specAmendBody + "\n## Amendments\n\n- **2026-01-01: Requirements** (forged)\n  x\n"
			return []string{"--data", specAmendData(specAmendName, reason, run, ""), "--from", stageSpecAmend(t, staged)}
		}},
		{"design not referenced", "spec_amend_design_not_referenced", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, reason, run, `{"source":"api","path":"overview.md"}`)}
		}},
		{"design document missing", "spec_amend_design_unresolved", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, reason, run, `{"source":"api","path":"missing.md"}`)}
		}},
		{"design source undeclared", "spec_amend_design_unresolved", func(t *testing.T, _, _ string) []string {
			return []string{"--data", specAmendData(specAmendName, reason, run, `{"source":"ghost","path":"old.md"}`)}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, specPath := specAmendProject(t)
			args := tc.setup(t, root, specPath)
			before, err := os.ReadFile(specPath)
			require.NoError(t, err)

			er := refuseSpecAmend(t, args...)
			require.Equal(t, tc.code, er.Code)
			require.NotEmpty(t, er.NextAction)

			after, err := os.ReadFile(specPath)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
		})
	}
}

// Criterion: a past amendment entry cannot be edited or removed. After one
// real amendment, staging the stored spec with that entry's text altered, or
// with the Amendments section dropped, is refused as not amendable.
func TestSpecAmend_PastAmendmentEntryIsAppendOnly(t *testing.T) {
	cases := []struct {
		name  string
		alter func(current string) string
	}{
		{"entry reason edited", func(current string) string {
			return strings.Replace(current, "  original reason", "  a rewritten reason", 1)
		}},
		{"section removed", func(current string) string {
			i := strings.Index(current, "\n## Amendments")
			return current[:i+1]
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, specPath := specAmendProject(t)
			amendSpecOK(t, "--data", specAmendData(specAmendName, "original reason", "run 1", ""),
				"--from", stageSpecAmend(t, strings.Replace(specAmendBody, "99%", "97%", 1)))

			current := readSpecViaCLI(t)
			// Also make an otherwise legal change, so the refusal is about the
			// history alone.
			staged := strings.Replace(tc.alter(current), "Charge cards monthly", "Charge cards weekly", 1)
			before, err := os.ReadFile(specPath)
			require.NoError(t, err)

			er := refuseSpecAmend(t, "--data", specAmendData(specAmendName, "second", "run 2", ""), "--from", stageSpecAmend(t, staged))
			require.Equal(t, "spec_amend_section_not_amendable", er.Code)
			require.Contains(t, er.Message, "Amendments")
			require.NotEmpty(t, er.NextAction)

			after, err := os.ReadFile(specPath)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
		})
	}
}

// Criterion: a dry run reports the result without writing anything.
func TestSpecAmend_DryRunWritesNothing(t *testing.T) {
	root, specPath := specAmendProject(t)
	staged := stageSpecAmend(t, strings.Replace(specAmendBody, "99%", "97%", 1))
	before := snapshotTree(t, root)

	got := amendSpecOK(t, "--dry-run", "--data", specAmendData(specAmendName, "r", "run 1", ""), "--from", staged)
	require.True(t, got.DryRun)
	require.Equal(t, []string{"Success Metrics"}, got.AmendedSections)
	require.NotEmpty(t, got.Hash)

	require.Equal(t, before, snapshotTree(t, root))
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Equal(t, specAmendFrontmatter+specAmendBody, string(raw))
}

// Criterion: --schema describes the name, reason, run and design input, with
// name, reason and run required.
func TestSpecAmend_Schema(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "amend", "--schema")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.NotNil(t, got.Input)
	require.ElementsMatch(t, []string{"name", "reason", "run", "design"}, keysOf(got.Input.Properties))
	require.ElementsMatch(t, []string{"name", "reason", "run"}, got.Input.Required)
	require.Equal(t, "object", got.Input.Properties["design"].Type)
	require.ElementsMatch(t, []string{"source", "path"}, keysOf(got.Input.Properties["design"].Properties))
	require.Contains(t, got.Flags, "from")
}

// keysOf returns the keys of a schema property map.
func keysOf(m map[string]*schemaProp) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Criterion: an amendment preserves the spec's existing frontmatter — its
// created date, document status, design references, sources and epic.
func TestSpecAmend_PreservesFrontmatter(t *testing.T) {
	_, specPath := specAmendProject(t)
	amendSpecOK(t, "--data", specAmendData(specAmendName, "r", "run 1", ""),
		"--from", stageSpecAmend(t, strings.Replace(specAmendBody, "99%", "97%", 1)))

	fm, _ := splitStoredSpec(t, specPath)
	require.Equal(t, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), fm.CreatedDate.UTC())
	require.Equal(t, metadata.DocumentStatus("final"), fm.DocumentStatus)
	require.Equal(t, "000068_payments", fm.Epic)
	require.Equal(t, []metadata.DesignRef{
		{Source: "api", Path: "payments/v2.md"},
		{Source: "api", Path: "missing.md"},
		{Source: "ghost", Path: "old.md"},
	}, fm.Designs)
	require.Equal(t, []metadata.SourceRef{{URI: "https://example.com/issue/1", RetrievedDate: "2026-06-30"}}, fm.Sources)
	require.Len(t, fm.Amendments, 1)
}

// Criterion: nothing is written outside the project's spec store — a
// successful amend changes the spec file's bytes and no other file under the
// project, and creates no file.
func TestSpecAmend_WritesOnlyTheSpec(t *testing.T) {
	root, specPath := specAmendProject(t)
	staged := stageSpecAmend(t, strings.Replace(specAmendBody, "99%", "97%", 1))
	before := snapshotTree(t, root)

	amendSpecOK(t, "--data", specAmendData(specAmendName, "r", "run 1", `{"source":"api","path":"payments/v2.md"}`), "--from", staged)

	after := snapshotTree(t, root)
	rel, err := filepath.Rel(root, specPath)
	require.NoError(t, err)
	require.Contains(t, before, rel)
	require.NotEqual(t, before[rel], after[rel])
	delete(before, rel)
	delete(after, rel)
	require.Equal(t, before, after)
}
