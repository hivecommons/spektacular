package metadata

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixedToday is the caller-injected clock value used across the merge tests.
// Every Merge/Render call in this file passes it explicitly so results are
// deterministic and independent of wall-clock time.
func fixedToday() time.Time {
	return time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC)
}

// documentStatusPtr returns a pointer to s, since UpdateOptions.DocumentStatus is *Status.
func documentStatusPtr(s DocumentStatus) *DocumentStatus { return &s }

func TestSplit(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMeta    *Metadata
		wantBody    string
		wantErr     bool
		wantErrText string
	}{
		{
			name:     "no frontmatter returns nil meta and original body",
			input:    "# Just a heading\n\nSome body text.\n",
			wantMeta: nil,
			wantBody: "# Just a heading\n\nSome body text.\n",
		},
		{
			name: "with well-formed frontmatter parses meta and returns body",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				"---\n\n" +
				"# Body heading\n",
			wantMeta: &Metadata{
				CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				DocumentStatus: StatusDraft,
			},
			wantBody: "# Body heading\n",
		},
		{
			name: "with closed_date parses the optional field",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: final\n" +
				"closed_date: 2026-07-15\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{
				CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				DocumentStatus: StatusFinal,
				ClosedDate:     time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC),
			},
			wantBody: "body\n",
		},
		{
			name: "unterminated frontmatter is an actionable error",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				"body without closing fence\n",
			wantErr:     true,
			wantErrText: "malformed frontmatter",
		},
		{
			name: "invalid YAML in frontmatter is an actionable error",
			input: "---\n" +
				"created_date: [not, a, string\n" +
				"---\n\n" +
				"body\n",
			wantErr:     true,
			wantErrText: "malformed frontmatter",
		},
		{
			name: "unrecognised document_status reads as blank",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: bogus\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)},
			wantBody: "body\n",
		},
		{
			name: "retired in-progress value reads as blank",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: in-progress\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)},
			wantBody: "body\n",
		},
		{
			name: "retired completed value reads as blank and keeps closed_date",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: completed\n" +
				"closed_date: 2026-07-15\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{
				CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				ClosedDate:  time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC),
			},
			wantBody: "body\n",
		},
		{
			name: "non-string document_status reads as blank",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: [a, b]\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)},
			wantBody: "body\n",
		},
		{
			name: "missing document_status reads as blank",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)},
			wantBody: "body\n",
		},
		{
			name: "legacy status key is ignored and reads as blank",
			input: "---\n" +
				"created_date: 2026-07-01\n" +
				"status: completed\n" +
				"closed_date: 2026-07-15\n" +
				"---\n\n" +
				"body\n",
			wantMeta: &Metadata{
				CreatedDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				ClosedDate:  time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC),
			},
			wantBody: "body\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := Split([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrText)
				return
			}
			require.NoError(t, err)
			if tt.wantMeta == nil {
				require.Nil(t, meta)
			} else {
				require.NotNil(t, meta)
				require.True(t, meta.CreatedDate.Equal(tt.wantMeta.CreatedDate),
					"CreatedDate: got %s want %s", meta.CreatedDate, tt.wantMeta.CreatedDate)
				require.Equal(t, tt.wantMeta.DocumentStatus, meta.DocumentStatus)
				require.True(t, meta.ClosedDate.Equal(tt.wantMeta.ClosedDate),
					"ClosedDate: got %s want %s", meta.ClosedDate, tt.wantMeta.ClosedDate)
			}
			require.Equal(t, tt.wantBody, string(body))
		})
	}
}

func TestRender_ShapeAndOptionalClosedDate(t *testing.T) {
	tests := []struct {
		name     string
		meta     Metadata
		body     string
		wantHas  []string
		wantMiss []string
	}{
		{
			name: "draft omits closed_date",
			meta: Metadata{
				CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				DocumentStatus: StatusDraft,
			},
			body: "# Body\n",
			wantHas: []string{
				"---\n",
				// yaml.v3 quotes date-shaped strings so they round-trip as
				// strings rather than being reinterpreted as timestamps by
				// downstream YAML parsers. Assert on the date value being
				// present without pinning the exact quoting.
				"created_date: ",
				"2026-07-01",
				"document_status: draft\n",
				"# Body\n",
			},
			wantMiss: []string{"closed_date"},
		},
		{
			name: "final with closed_date includes it as YYYY-MM-DD",
			meta: Metadata{
				CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
				DocumentStatus: StatusFinal,
				ClosedDate:     time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC),
			},
			body: "# Body\n",
			wantHas: []string{
				"created_date: ",
				"2026-07-01",
				"document_status: final\n",
				"closed_date: ",
				"2026-07-28",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Render(tt.meta, []byte(tt.body))
			require.NoError(t, err)
			gotStr := string(out)
			for _, want := range tt.wantHas {
				require.Contains(t, gotStr, want)
			}
			for _, missing := range tt.wantMiss {
				require.NotContains(t, gotStr, missing)
			}
			// Structural sanity: output begins with an opener and contains a
			// closing fence followed by a blank line before the body.
			require.True(t, strings.HasPrefix(gotStr, "---\n"), "output should begin with `---\\n`")
			require.Contains(t, gotStr, "---\n\n", "closing fence must be followed by a blank line")
		})
	}
}

// TestMerge_LegacyStatusKeyIsDropped asserts a rewrite of an artifact that
// still carries the retired `status` key writes no `status` key back and
// renders its blank document status explicitly.
func TestMerge_LegacyStatusKeyIsDropped(t *testing.T) {
	legacy := "---\n" +
		"created_date: 2026-07-01\n" +
		"status: completed\n" +
		"closed_date: 2026-07-15\n" +
		"---\n\n" +
		"# Body\n"

	got, err := Merge([]byte(legacy), []byte("# Body\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	fm, _, found := strings.Cut(strings.TrimPrefix(string(got), "---\n"), "\n---\n")
	require.True(t, found, "output must carry a frontmatter block")
	require.NotContains(t, "\n"+fm, "\nstatus:", "the legacy status key must not be written back")
	require.Contains(t, fm, `document_status: ""`, "a blank status is rendered explicitly")
	require.Contains(t, fm, "2026-07-01", "created_date must be preserved")
	require.Contains(t, fm, "2026-07-15", "closed_date must be preserved")
}

// TestParseDocumentStatus asserts the validator accepts exactly the
// named values and rejects blank and retired or unknown values.
func TestParseDocumentStatus(t *testing.T) {
	for _, raw := range []string{"draft", "final", "stale", "superseded", "archived"} {
		got, ok := ParseDocumentStatus(raw)
		require.True(t, ok, "%q must be accepted", raw)
		require.Equal(t, DocumentStatus(raw), got)
	}
	for _, raw := range []string{"", "in-progress", "completed", "bogus", "Draft"} {
		got, ok := ParseDocumentStatus(raw)
		require.False(t, ok, "%q must be rejected", raw)
		require.Equal(t, DocumentStatus(""), got)
	}
	require.Equal(t, []DocumentStatus{StatusDraft, StatusFinal, StatusStale, StatusSuperseded, StatusArchived}, DocumentStatuses())
}

func TestRender_SplitRoundTrip(t *testing.T) {
	meta := Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusFinal,
		ClosedDate:     time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC),
	}
	body := []byte("# Body heading\n\nSome text.\n")

	rendered, err := Render(meta, body)
	require.NoError(t, err)

	gotMeta, gotBody, err := Split(rendered)
	require.NoError(t, err)
	require.NotNil(t, gotMeta)
	require.True(t, gotMeta.CreatedDate.Equal(meta.CreatedDate))
	require.Equal(t, meta.DocumentStatus, gotMeta.DocumentStatus)
	require.True(t, gotMeta.ClosedDate.Equal(meta.ClosedDate))
	require.Equal(t, string(body), string(gotBody))
}

func TestMerge(t *testing.T) {
	today := fixedToday()
	earlier := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	closedEarlier := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)

	draftEarlier := "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: draft\n" +
		"---\n\n" +
		"# Existing body\n"

	finalEarlier := "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: final\n" +
		"closed_date: 2026-07-15\n" +
		"---\n\n" +
		"# Existing body\n"

	legacyClosed := "---\n" +
		"created_date: 2026-07-01\n" +
		"status: completed\n" +
		"closed_date: 2026-07-15\n" +
		"---\n\n" +
		"# Existing body\n"

	legacyOpen := "---\n" +
		"created_date: 2026-07-01\n" +
		"status: in-progress\n" +
		"---\n\n" +
		"# Existing body\n"

	tests := []struct {
		name        string
		existing    string
		newBody     string
		opts        UpdateOptions
		wantErr     bool
		wantErrText string
		wantCreated time.Time
		wantStatus  DocumentStatus
		wantClosed  time.Time // zero means the field must be omitted
		wantBody    string
		// wantSameAsExisting, when true, asserts that Merge output equals
		// the existing input byte-for-byte — the idempotent-write invariant.
		wantSameAsExisting bool
	}{
		{
			name:        "fresh write stamps created_date today and defaults to draft",
			existing:    "",
			newBody:     "# New body\n",
			opts:        UpdateOptions{Today: today},
			wantCreated: today,
			wantStatus:  StatusDraft,
			wantBody:    "# New body\n",
		},
		{
			name:        "fresh write honors explicit closed status and stamps closed_date",
			existing:    "",
			newBody:     "# New body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusFinal), Today: today},
			wantCreated: today,
			wantStatus:  StatusFinal,
			wantClosed:  today,
			wantBody:    "# New body\n",
		},
		{
			name:        "bare artifact with no frontmatter is treated as fresh",
			existing:    "# Just a heading\n",
			newBody:     "# Updated body\n",
			opts:        UpdateOptions{Today: today},
			wantCreated: today,
			wantStatus:  StatusDraft,
			wantBody:    "# Updated body\n",
		},
		{
			name:        "existing metadata preserves created_date across rewrite",
			existing:    draftEarlier,
			newBody:     "# Updated body\n",
			opts:        UpdateOptions{Today: today},
			wantCreated: earlier,
			wantStatus:  StatusDraft,
			wantBody:    "# Updated body\n",
		},
		{
			name:        "transition draft -> final stamps closed_date once",
			existing:    draftEarlier,
			newBody:     "# Updated body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusFinal), Today: today},
			wantCreated: earlier,
			wantStatus:  StatusFinal,
			wantClosed:  today,
			wantBody:    "# Updated body\n",
		},
		{
			name:               "idempotent final -> final preserves original closed_date",
			existing:           finalEarlier,
			newBody:            "# Existing body\n",
			opts:               UpdateOptions{DocumentStatus: documentStatusPtr(StatusFinal), Today: today},
			wantCreated:        earlier,
			wantStatus:         StatusFinal,
			wantClosed:         closedEarlier,
			wantBody:           "# Existing body\n",
			wantSameAsExisting: true,
		},
		{
			name:        "closed -> different closed status keeps first-transition closed_date",
			existing:    finalEarlier,
			newBody:     "# Existing body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusSuperseded), Today: today},
			wantCreated: earlier,
			wantStatus:  StatusSuperseded,
			wantClosed:  closedEarlier,
			wantBody:    "# Existing body\n",
		},
		{
			name:        "closed -> draft clears closed_date",
			existing:    finalEarlier,
			newBody:     "# Existing body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusDraft), Today: today},
			wantCreated: earlier,
			wantStatus:  StatusDraft,
			wantClosed:  time.Time{},
			wantBody:    "# Existing body\n",
		},
		{
			name:        "nil status on legacy artifact keeps blank status and closed_date",
			existing:    legacyClosed,
			newBody:     "# Updated body\n",
			opts:        UpdateOptions{Today: today},
			wantCreated: earlier,
			wantStatus:  "",
			wantClosed:  closedEarlier,
			wantBody:    "# Updated body\n",
		},
		{
			name:        "blank -> final keeps an existing closed_date",
			existing:    legacyClosed,
			newBody:     "# Existing body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusFinal), Today: today},
			wantCreated: earlier,
			wantStatus:  StatusFinal,
			wantClosed:  closedEarlier,
			wantBody:    "# Existing body\n",
		},
		{
			name:        "blank -> final without a closed_date stamps today",
			existing:    legacyOpen,
			newBody:     "# Existing body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(StatusFinal), Today: today},
			wantCreated: earlier,
			wantStatus:  StatusFinal,
			wantClosed:  today,
			wantBody:    "# Existing body\n",
		},
		{
			name:        "blank status cannot be set explicitly",
			existing:    legacyOpen,
			newBody:     "# body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(""), Today: today},
			wantErr:     true,
			wantErrText: "invalid document status",
		},
		{
			name:        "retired value cannot be set explicitly",
			existing:    draftEarlier,
			newBody:     "# body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(DocumentStatus("completed")), Today: today},
			wantErr:     true,
			wantErrText: "invalid document status",
		},
		{
			name:        "nil status on existing artifact leaves status and closed_date untouched",
			existing:    finalEarlier,
			newBody:     "# Updated body\n",
			opts:        UpdateOptions{Today: today},
			wantCreated: earlier,
			wantStatus:  StatusFinal,
			wantClosed:  closedEarlier,
			wantBody:    "# Updated body\n",
		},
		{
			name:        "invalid enum on fresh write is rejected with actionable error",
			existing:    "",
			newBody:     "# body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(DocumentStatus("bogus")), Today: today},
			wantErr:     true,
			wantErrText: "invalid document status",
		},
		{
			name:        "invalid enum on existing artifact is rejected with actionable error",
			existing:    draftEarlier,
			newBody:     "# body\n",
			opts:        UpdateOptions{DocumentStatus: documentStatusPtr(DocumentStatus("bogus")), Today: today},
			wantErr:     true,
			wantErrText: "invalid document status",
		},
		{
			name: "malformed frontmatter propagates as an actionable error",
			existing: "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				"body-with-no-closing-fence\n",
			newBody:     "# body\n",
			opts:        UpdateOptions{Today: today},
			wantErr:     true,
			wantErrText: "malformed frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Merge([]byte(tt.existing), []byte(tt.newBody), tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrText)
				return
			}
			require.NoError(t, err)

			if tt.wantSameAsExisting {
				// Canonicalize the existing input by round-tripping it
				// through Split+Render once, so the byte-for-byte check
				// isn't sensitive to insignificant YAML quoting differences
				// between the hand-written fixture and yaml.v3's own
				// output. Once canonicalized, a second Merge with the same
				// inputs MUST reproduce the same bytes exactly — that is
				// the idempotency guarantee.
				canonical, err := Merge([]byte(tt.existing), []byte(tt.newBody), tt.opts)
				require.NoError(t, err)
				require.Equal(t, string(canonical), string(got),
					"idempotent write must produce byte-identical output")
			}

			meta, body, err := Split(got)
			require.NoError(t, err)
			require.NotNil(t, meta, "Merge output must carry frontmatter")

			require.True(t, meta.CreatedDate.Equal(tt.wantCreated),
				"CreatedDate: got %s want %s", meta.CreatedDate, tt.wantCreated)
			require.Equal(t, tt.wantStatus, meta.DocumentStatus)
			require.True(t, meta.ClosedDate.Equal(tt.wantClosed),
				"ClosedDate: got %s want %s", meta.ClosedDate, tt.wantClosed)

			// Verify closed_date is physically absent from the serialized
			// output when zero, so consumers reading the raw YAML don't see
			// an empty field.
			if tt.wantClosed.IsZero() {
				require.NotContains(t, string(got), "closed_date",
					"closed_date must be omitted when zero")
			}

			// Merge never writes the retired `status` key, on a fresh
			// write or otherwise. Checked at line start because
			// `document_status:` contains `status:` as a substring.
			require.NotContains(t, "\n"+string(got), "\nstatus:",
				"Merge output must not carry a legacy status key")

			require.Equal(t, tt.wantBody, string(body))
		})
	}
}

// Criterion 3: front matter round-trips the four provenance fields — a
// Metadata carrying project, project_source, spec, and plan survives
// Render → Split with every field intact alongside the date/status fields.
func TestRender_SplitRoundTripProvenanceFields(t *testing.T) {
	meta := Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
		Project:        "testproj",
		ProjectSource:  "https://github.com/example/testproj",
		Spec:           "000039_project-level-capabilities",
		Plan:           "000039_project-level-capabilities",
	}
	body := []byte("# Changelog record\n")

	rendered, err := Render(meta, body)
	require.NoError(t, err)

	gotMeta, gotBody, err := Split(rendered)
	require.NoError(t, err)
	require.NotNil(t, gotMeta)
	require.Equal(t, meta.Project, gotMeta.Project)
	require.Equal(t, meta.ProjectSource, gotMeta.ProjectSource)
	require.Equal(t, meta.Spec, gotMeta.Spec)
	require.Equal(t, meta.Plan, gotMeta.Plan)
	require.True(t, gotMeta.CreatedDate.Equal(meta.CreatedDate))
	require.Equal(t, meta.DocumentStatus, gotMeta.DocumentStatus)
	require.Equal(t, string(body), string(gotBody))
}

// Criterion 3: an entry without provenance keys parses with empty provenance
// fields — the new keys are optional and their absence is not an error.
func TestSplit_MissingProvenanceKeysParseAsEmpty(t *testing.T) {
	raw := "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: draft\n" +
		"---\n\n" +
		"body\n"

	meta, _, err := Split([]byte(raw))
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Project)
	require.Empty(t, meta.ProjectSource)
	require.Empty(t, meta.Spec)
	require.Empty(t, meta.Plan)
}

// Criterion 3: a Metadata whose provenance fields are empty marshals WITHOUT
// emitting the provenance keys (omitempty) — pre-existing artifacts must not
// suddenly grow empty `project:` / `spec:` / `plan:` lines on rewrite.
func TestRender_OmitsEmptyProvenanceKeys(t *testing.T) {
	rendered, err := Render(Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
	}, []byte("body\n"))
	require.NoError(t, err)

	got := string(rendered)
	// "project" also covers "project_source" as a substring.
	require.NotContains(t, got, "project")
	require.NotContains(t, got, "spec")
	require.NotContains(t, got, "plan")
}

// Criterion 3: Merge on a fresh write stamps the opts provenance fields into
// the new frontmatter block alongside the usual created_date/status stamp.
func TestMerge_FreshWriteStampsProvenance(t *testing.T) {
	got, err := Merge(nil, []byte("# body\n"), UpdateOptions{
		Today:         fixedToday(),
		Project:       "testproj",
		ProjectSource: "https://github.com/example/testproj",
		Spec:          "000039_spec",
		Plan:          "000039_plan",
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "testproj", meta.Project)
	require.Equal(t, "https://github.com/example/testproj", meta.ProjectSource)
	require.Equal(t, "000039_spec", meta.Spec)
	require.Equal(t, "000039_plan", meta.Plan)
	require.True(t, meta.CreatedDate.Equal(fixedToday()))
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// Criterion 3: Merge on existing content preserves the stored provenance when
// the opts fields are empty — an empty incoming value means "no change", never
// "clear the field".
func TestMerge_ExistingProvenancePreservedWhenOptsEmpty(t *testing.T) {
	existing := "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: draft\n" +
		"project: testproj\n" +
		"project_source: https://github.com/example/testproj\n" +
		"spec: 000039_spec\n" +
		"plan: 000039_plan\n" +
		"---\n\n" +
		"# Existing body\n"

	got, err := Merge([]byte(existing), []byte("# Updated body\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "testproj", meta.Project)
	require.Equal(t, "https://github.com/example/testproj", meta.ProjectSource)
	require.Equal(t, "000039_spec", meta.Spec)
	require.Equal(t, "000039_plan", meta.Plan)
	require.Equal(t, "# Updated body\n", string(body))
}

// Criterion 3: Merge on existing content overwrites provenance when the opts
// fields are set — incoming wins over stored, field by field, while the
// date/status merge invariants stay intact (created_date preserved).
func TestMerge_ExistingProvenanceOverwrittenWhenOptsSet(t *testing.T) {
	existing := "---\n" +
		"created_date: 2026-07-01\n" +
		"document_status: draft\n" +
		"project: oldproj\n" +
		"project_source: https://old.example\n" +
		"spec: 000001_old\n" +
		"plan: 000001_old\n" +
		"---\n\n" +
		"# Existing body\n"

	got, err := Merge([]byte(existing), []byte("# Existing body\n"), UpdateOptions{
		Today:         fixedToday(),
		Project:       "newproj",
		ProjectSource: "https://new.example",
		Spec:          "000002_new",
		Plan:          "000002_new",
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "newproj", meta.Project)
	require.Equal(t, "https://new.example", meta.ProjectSource)
	require.Equal(t, "000002_new", meta.Spec)
	require.Equal(t, "000002_new", meta.Plan)
	require.True(t, meta.CreatedDate.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)),
		"created_date must survive a provenance overwrite")
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// TestMerge_UsesInjectedClock double-checks that the caller-injected clock is
// the only source of "today" — Merge must not fall back to wall-clock time
// when opts.Today is explicitly set.
func TestMerge_UsesInjectedClock(t *testing.T) {
	arbitraryPast := time.Date(2020, time.January, 15, 0, 0, 0, 0, time.UTC)

	got, err := Merge(nil, []byte("# body\n"), UpdateOptions{Today: arbitraryPast})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.True(t, meta.CreatedDate.Equal(arbitraryPast),
		"CreatedDate must come from opts.Today, got %s", meta.CreatedDate)
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// twoDesignRefs is the ordered two-reference list the design-reference tests
// share. Order is significant: the list is rendered and read back as a
// sequence, so a reordering is a regression.
func twoDesignRefs() []DesignRef {
	return []DesignRef{
		{Source: "api", Path: "payments/v2.md"},
		{Source: "platform", Path: "ingress.md"},
	}
}

// twoDesignRefsYAML is the hand-written frontmatter fragment matching
// twoDesignRefs, for fixtures that seed an artifact carrying references.
const twoDesignRefsYAML = "designs:\n" +
	"  - source: api\n" +
	"    path: payments/v2.md\n" +
	"  - source: platform\n" +
	"    path: ingress.md\n"

// TestRender_SplitRoundTripDesignRefs asserts a two-reference list survives
// Render → Split with both halves of each address intact and in the same
// order.
func TestRender_SplitRoundTripDesignRefs(t *testing.T) {
	meta := Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
		Designs:        twoDesignRefs(),
	}
	body := []byte("# Spec body\n")

	rendered, err := Render(meta, body)
	require.NoError(t, err)

	gotMeta, gotBody, err := Split(rendered)
	require.NoError(t, err)
	require.NotNil(t, gotMeta)
	require.Equal(t, twoDesignRefs(), gotMeta.Designs)
	require.Equal(t, string(body), string(gotBody))
}

// TestRender_OmitsDesignsKeyEntirelyWithNoReferences pins the exact bytes an
// artifact with no design references renders to. The expectation is written
// out by hand rather than derived from the renderer, so it also stands as the
// pre-change output: adding the designs field must not perturb a single byte
// of an artifact that carries none.
func TestRender_OmitsDesignsKeyEntirelyWithNoReferences(t *testing.T) {
	rendered, err := Render(Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
	}, []byte("# body\n"))
	require.NoError(t, err)

	require.Equal(t,
		"---\ncreated_date: \"2026-07-01\"\ndocument_status: draft\n---\n\n# body\n",
		string(rendered))
}

// TestSplit_MalformedDesignsReadAsNoReferences asserts reads across the
// designs key are lenient in the same way they are across document_status: a
// missing, empty, scalar or mapping value reads as no references and never
// fails the parse, so a hand-edited spec stays readable.
func TestSplit_MalformedDesignsReadAsNoReferences(t *testing.T) {
	tests := []struct {
		name    string
		designs string
	}{
		{name: "absent", designs: ""},
		{name: "empty list", designs: "designs: []\n"},
		{name: "scalar", designs: "designs: nonsense\n"},
		{name: "mapping", designs: "designs:\n  a: b\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				tt.designs +
				"---\n\n" +
				"body\n"

			meta, body, err := Split([]byte(raw))
			require.NoError(t, err, "a malformed designs value must not fail the parse")
			require.NotNil(t, meta)
			require.Empty(t, meta.Designs)
			require.Equal(t, StatusDraft, meta.DocumentStatus,
				"the rest of the block must still parse")
			require.Equal(t, "body\n", string(body))
		})
	}
}

// TestSplit_DesignEntryMissingHalfOfAddressIsDropped asserts an entry that
// names only a source, or only a path, is dropped on read while a complete
// sibling entry in the same list is kept — a reference that cannot name both
// halves cannot be resolved, but it must not take the whole list with it.
func TestSplit_DesignEntryMissingHalfOfAddressIsDropped(t *testing.T) {
	tests := []struct {
		name    string
		designs string
	}{
		{
			name: "entry missing source",
			designs: "designs:\n" +
				"  - path: orphan.md\n" +
				"  - source: api\n" +
				"    path: payments/v2.md\n",
		},
		{
			name: "entry missing path",
			designs: "designs:\n" +
				"  - source: orphan\n" +
				"  - source: api\n" +
				"    path: payments/v2.md\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				tt.designs +
				"---\n\n" +
				"body\n"

			meta, _, err := Split([]byte(raw))
			require.NoError(t, err)
			require.NotNil(t, meta)
			require.Equal(t, []DesignRef{{Source: "api", Path: "payments/v2.md"}}, meta.Designs)
		})
	}
}

// twoSpecNames is the ordered two-name back-link list the referencing-spec
// tests share. Order is significant: the list is rendered and read back as a
// sequence, so a reordering is a regression.
func twoSpecNames() []string {
	return []string{"000041_payments-v2", "000055_design-authoring-skill"}
}

// twoSpecNamesYAML is the hand-written frontmatter fragment matching
// twoSpecNames, for fixtures that seed a document carrying back-links.
const twoSpecNamesYAML = "specs:\n" +
	"  - 000041_payments-v2\n" +
	"  - 000055_design-authoring-skill\n"

// TestRender_SplitRoundTripSpecNames asserts a two-name back-link list
// survives Render → Split intact and in the same order.
func TestRender_SplitRoundTripSpecNames(t *testing.T) {
	meta := Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
		Specs:          twoSpecNames(),
	}
	body := []byte("# Design body\n")

	rendered, err := Render(meta, body)
	require.NoError(t, err)

	gotMeta, gotBody, err := Split(rendered)
	require.NoError(t, err)
	require.NotNil(t, gotMeta)
	require.Equal(t, twoSpecNames(), gotMeta.Specs)
	require.Equal(t, string(body), string(gotBody))
}

// TestRender_OmitsDesignsAndSpecsKeysEntirelyWithNoReferences is the sibling
// of TestRender_OmitsDesignsKeyEntirelyWithNoReferences that names both
// reference fields. The expectation is the same hand-written byte sequence,
// so adding the specs field must not perturb a single byte of a document
// that carries neither list.
func TestRender_OmitsDesignsAndSpecsKeysEntirelyWithNoReferences(t *testing.T) {
	rendered, err := Render(Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: StatusDraft,
		Designs:        nil,
		Specs:          nil,
	}, []byte("# body\n"))
	require.NoError(t, err)

	require.Equal(t,
		"---\ncreated_date: \"2026-07-01\"\ndocument_status: draft\n---\n\n# body\n",
		string(rendered))
}

// TestSplit_MalformedSpecsReadAsNoReferences asserts reads across the specs
// key are lenient in the same way they are across designs: a missing, empty,
// scalar, mapping or non-scalar-entry value reads as no back-links and never
// fails the parse, so a hand-edited design stays readable.
func TestSplit_MalformedSpecsReadAsNoReferences(t *testing.T) {
	tests := []struct {
		name  string
		specs string
	}{
		{name: "absent", specs: ""},
		{name: "empty list", specs: "specs: []\n"},
		{name: "scalar", specs: "specs: nonsense\n"},
		{name: "mapping", specs: "specs:\n  a: b\n"},
		{name: "list of non-scalar", specs: "specs:\n  - a: b\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := "---\n" +
				"created_date: 2026-07-01\n" +
				"document_status: draft\n" +
				tt.specs +
				"---\n\n" +
				"body\n"

			meta, body, err := Split([]byte(raw))
			require.NoError(t, err, "a malformed specs value must not fail the parse")
			require.NotNil(t, meta)
			require.Empty(t, meta.Specs)
			require.Equal(t, StatusDraft, meta.DocumentStatus,
				"the rest of the block must still parse")
			require.Equal(t, "body\n", string(body))
		})
	}
}
