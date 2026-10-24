package spec

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// unattendedHeading opens the partial stepkit appends to every step of an
// unattended run.
const unattendedHeading = "## Running unattended"

// runUnattendedStep drives a step callback for the fixture spec with the
// unattended flag spec new stores in the workflow data.
func runUnattendedStep(t *testing.T, cb workflow.StepCallback, st store.Store) string {
	t.Helper()
	writer := &captureWriter{}
	data := &testData{values: map[string]any{"name": splitFixtureSpec, "unattended": true}}
	_, err := cb(data, writer, st, splitTestCfg())
	require.NoError(t, err)
	return writer.result.Instruction
}

// Every step of an unattended run carries the hand-off instructions, and
// they name the run's own working files.
func TestUnattended_EveryStepCarriesTheHandOffInstructions(t *testing.T) {
	for _, step := range Steps() {
		if step.Name == "new" {
			continue // new writes the scaffold and resets the working context in the cwd
		}
		t.Run(step.Name, func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			seedFilledSpec(t, st, "")

			out := runUnattendedStep(t, step.Callback, st)

			require.Contains(t, out, unattendedHeading)
			require.Contains(t, out, ".spektacular/work/"+splitFixtureSpec+"/assumptions.md")
			require.Contains(t, out, ".spektacular/work/"+splitFixtureSpec+"/open_questions.md")
			require.NotContains(t, out, "{{")
		})
	}
}

// An ordinary run carries none of the unattended text, so the interactive
// behaviour stays the default.
func TestUnattended_AttendedRunIsUnchanged(t *testing.T) {
	for _, step := range Steps() {
		if step.Name == "new" {
			continue
		}
		t.Run(step.Name, func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			seedFilledSpec(t, st, "")

			out := runSpecStep(t, step.Callback, st)

			require.NotContains(t, out, unattendedHeading)
			require.NotContains(t, out, "unattended")
			require.NotContains(t, out, "Review Notes")
		})
	}
}

// The verification step of an unattended run triages alone and commits the
// spec with its review notes.
func TestUnattended_VerificationAddsReviewNotes(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	out := runUnattendedStep(t, verification(), st)

	require.Contains(t, out, "you triage alone and ask nothing")
	require.Contains(t, out, "## Review Notes")
	require.Contains(t, out, "### Assumptions")
	require.Contains(t, out, "### Open Questions")
}

// The split step of an unattended run never splits: it carries the proposal
// to the review.
func TestUnattended_SplitIsCarriedToTheReview(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, "")

	out := runUnattendedStep(t, split(), st)

	require.Contains(t, out, "This run is unattended, so do not split.")
	require.Contains(t, out, "spektacular spec file write "+splitFixtureSpec)
}

// Finishing an unattended run leaves the spec a draft marked unreviewed, with
// no closed date, and ends with the one summary and how to sign it off.
func TestUnattended_FinishedLeavesTheSpecUnreviewed(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, "")

	out := runUnattendedStep(t, finished(), st)

	raw, err := st.Read(SpecFilePath("specs", splitFixtureSpec))
	require.NoError(t, err)
	meta, body, err := metadata.Split(raw)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus, "an unreviewed spec must not be closed")
	require.True(t, meta.ClosedDate.IsZero())
	require.True(t, meta.Unreviewed)
	require.Equal(t, "# Fixture\n\nFilled body that is not the scaffold.\n", string(body), "marking must not touch the body")

	require.Contains(t, out, "stored as a draft marked `unreviewed`")
	require.Contains(t, out, "**Give the user one summary, now, and nothing before it.**")
	require.Contains(t, out, "spektacular spec file set-document-status <name> --document-status final")
	require.NotContains(t, out, "ready to use")
	require.NotContains(t, out, "Offer the next item")
}

// An unattended spec that was never written is reported as unwritten and is
// not marked: there is nothing to review yet.
func TestUnattended_FinishedLeavesUnwrittenSpecAlone(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")

	out := runUnattendedStep(t, finished(), st)

	require.Contains(t, out, "never committed to the store")
	require.NotContains(t, out, "Give the user one summary")
}

// Finishing an unattended spec in an epic starts the next item's spec,
// unattended too, instead of offering it.
func TestUnattended_FinishedInAnEpicChainsWithoutAsking(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, splitFixtureEpic)
	seedEpic(t, st, []metadata.SourceRef{{URI: splitFixtureURI, RetrievedDate: "2026-07-01"}})

	out := runUnattendedStep(t, finished(), st)

	require.Contains(t, out, `"epic":"`+splitFixtureEpic+`","unattended":true`)
	require.Contains(t, out, "spektacular epic read "+splitFixtureEpic)
	require.NotContains(t, out, "Never start it without the user's agreement")
	require.Contains(t, out, "covering every spec this hand-off wrote")
}
