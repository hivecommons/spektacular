package spec

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

const (
	splitFixtureSpec = "000001_fixture"
	splitFixtureEpic = "000002_billing"
	splitFixtureURI  = "https://github.com/acme/app/issues/42"
)

// splitTestCfg is the configuration every split/finished test runs under: a
// real (non-dry-run) store with spec and epic directories configured.
func splitTestCfg() workflow.Config {
	return workflow.Config{Command: "spektacular", SpecDir: "specs", EpicDir: "epics"}
}

// seedFilledSpec writes a completed (non-scaffold) spec, naming epicName in
// its frontmatter when it is not empty.
func seedFilledSpec(t *testing.T, st store.Store, epicName string) {
	t.Helper()
	raw, err := metadata.Render(metadata.Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Epic:           epicName,
	}, []byte("# Fixture\n\nFilled body that is not the scaffold.\n"))
	require.NoError(t, err)
	require.NoError(t, st.Write(SpecFilePath("specs", splitFixtureSpec), raw))
}

// seedEpic writes an epic listing the fixture spec, with the given sources.
func seedEpic(t *testing.T, st store.Store, sources []metadata.SourceRef) {
	t.Helper()
	raw, err := epic.Epic{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs:          []epic.EpicSpec{{Name: splitFixtureSpec, DependsOn: []string{}}},
		Sources:        sources,
		Body:           []byte("## Overview\n\nBilling.\n\n## Specs\n"),
	}.Render()
	require.NoError(t, err)
	require.NoError(t, st.Write(artifact.Address{Kind: artifact.KindEpic, Feature: splitFixtureEpic}.StorePath("epics"), raw))
}

// runSpecStep drives a step callback against st and returns its instruction.
func runSpecStep(t *testing.T, cb workflow.StepCallback, st store.Store) string {
	t.Helper()
	writer := &captureWriter{}
	_, err := cb(&testData{values: map[string]any{"name": splitFixtureSpec}}, writer, st, splitTestCfg())
	require.NoError(t, err)
	return writer.result.Instruction
}

// The split step template renders through stepkit with both partials
// resolved and the command prefix filled in, and advances to finished.
func TestSplitStep_RendersPartialsAndCommand(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, "")

	out := runSpecStep(t, split(), st)

	require.NotContains(t, out, "{{", "every mustache tag, partials included, must render")
	require.Contains(t, out, "### The split check", "the split-check partial must resolve")
	require.Contains(t, out, "### The split flow", "the split-flow partial must resolve")
	require.Contains(t, out, "spektacular epic split --from .spektacular/tmp/epic_split.json")
	require.Contains(t, out, `spektacular spec goto --data '{"step":"finished"}'`)
}

// A spec that already belongs to an epic tells the split step so: the
// instruction names the epic and points the agent at it.
func TestSplitStep_SpecInAnEpicPassesTheEpicName(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, splitFixtureEpic)

	out := runSpecStep(t, split(), st)

	require.Contains(t, out, "This spec already belongs to the epic `"+splitFixtureEpic+"`")
	require.Contains(t, out, "spektacular epic read "+splitFixtureEpic)
}

// A standalone spec renders the split step with no epic paragraph.
func TestSplitStep_StandaloneSpecNamesNoEpic(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, "")

	out := runSpecStep(t, split(), st)

	require.NotContains(t, out, "already belongs to the epic")
	require.NotContains(t, out, "epic read")
}

// Finishing a spec in an epic that records a source renders the chaining
// offer, naming the epic and the source the agent must re-read.
func TestSpecFinished_SpecInAnEpicWithSourcesOffersTheNextItem(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, splitFixtureEpic)
	seedEpic(t, st, []metadata.SourceRef{{URI: splitFixtureURI, RetrievedDate: "2026-07-01"}})

	out := runSpecStep(t, finished(), st)

	require.NotContains(t, out, "{{")
	require.Contains(t, out, "**Offer the next item in the epic.**")
	require.Contains(t, out, "spektacular epic read "+splitFixtureEpic)
	require.Contains(t, out, "`"+splitFixtureURI+"`", "the epic's source must be named for the agent to re-read")
	require.NotContains(t, out, "the epic records no source")
	require.Contains(t, out, `"epic":"`+splitFixtureEpic+`"`, "the spec new command must join the next spec to the epic")
	require.Contains(t, out, "The offer stops at specifying")
}

// Finishing a spec in an epic with no source still renders the offer, but
// tells the agent there is nothing to chain to.
func TestSpecFinished_SpecInAnEpicWithoutSourcesSaysThereIsNothingToChainTo(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, splitFixtureEpic)
	seedEpic(t, st, nil)

	out := runSpecStep(t, finished(), st)

	require.Contains(t, out, "**Offer the next item in the epic.**")
	require.Contains(t, out, "the epic records no source")
	require.NotContains(t, out, splitFixtureURI)
}

// Finishing a standalone spec renders no chaining offer at all.
func TestSpecFinished_StandaloneSpecHasNoChainingOffer(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledSpec(t, st, "")

	out := runSpecStep(t, finished(), st)

	require.Contains(t, out, "The spec is complete.")
	require.NotContains(t, out, "Offer the next item in the epic")
	require.NotContains(t, out, "epic read")
	require.NotContains(t, out, "spec new --data")
}
