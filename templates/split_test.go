package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The split step's contract lives in three files: the step template itself
// and the two partials it includes. These tests pin the behaviour each one
// must carry, so a rewrite cannot quietly drop a rule the split depends on.
const (
	splitStepTemplate  = "steps/spec/08b-split.md"
	splitCheckPartial  = "partials/split-check.md"
	splitFlowPartial   = "partials/split-flow.md"
	finishedStepTmpl   = "steps/spec/09-finished.md"
	splitRequestMarker = "**A split asked for now.**"
)

// flatTemplate reads a template with its whitespace collapsed, so a phrase
// soft-wrapped across lines still matches.
func flatTemplate(t *testing.T, p string) string {
	t.Helper()
	return strings.Join(strings.Fields(mustReadTemplate(t, p)), " ")
}

// requireAllContained asserts every phrase is present in body.
func requireAllContained(t *testing.T, label, body string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		require.Containsf(t, body, p, "%s must carry %q", label, p)
	}
}

// The split step pulls in both partials and honours a split the user asked
// for earlier, which was recorded in the working context.
func TestSplitStepIncludesCheckAndFlowAndHonoursEarlierRequest(t *testing.T) {
	body := flatTemplate(t, splitStepTemplate)

	requireAllContained(t, splitStepTemplate, body,
		"{{> partials/split-check}}",
		"{{> partials/split-flow}}",
		"Read `.spektacular/working-context.md`",
		"asked for a split while this spec was still being written, act on that request now",
		"tell the user plainly why this spec cannot be split",
	)
	require.Less(t, strings.Index(body, "{{> partials/split-check}}"), strings.Index(body, "{{> partials/split-flow}}"),
		"the check must come before the flow it gates")
}

// The split check: the gate, both kinds of signal, the supporting-work rule,
// the counter-signals, and the live sensitivity read.
func TestSplitCheckCarriesGateSignalsAndRules(t *testing.T) {
	body := flatTemplate(t, splitCheckPartial)

	t.Run("gate", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"**The gate.**",
			"at least two specs",
			"at least one acceptance criterion that can be verified without the others",
			"If you cannot name them, do not offer",
		)
	})

	t.Run("signals", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"**Strong signals.** Any one is enough",
			"lists child items",
			"could each ship and be useful alone",
			"cannot all be verified by one change",
			"phases the work",
			"**Weak signals.** At least two together are needed",
			"more than about seven requirements",
			"more than one design document needed",
		)
	})

	t.Run("supporting work", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"**Supporting work never counts.**",
			"Code plus its docs is one spec, at every sensitivity.",
		)
	})

	t.Run("counter-signals", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"**Counter-signals.**",
			"suppress the offer even when signals have fired, at every sensitivity",
			"tightly coupled",
		)
	})

	t.Run("sensitivity", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"Read `epic_split_threshold` from `.spektacular/config.yaml` now, at the moment you are deciding",
			"Treat a missing or absent value as `\"moderate\"`",
			"separate from `spec_trigger_threshold`",
			"It never overrides the supporting-work rule or the counter-signals.",
			"`\"strict\"`",
			"`\"moderate\"`",
			"`\"lenient\"`",
			"The gate applies at every level.",
		)
	})

	t.Run("offer, never act", func(t *testing.T) {
		requireAllContained(t, splitCheckPartial, body,
			"offer — never split on your own",
			"Wait for the user's decision.",
		)
	})
}

// The split flow acts only on agreement, redistributes content without loss
// or duplication, reviews the result once, and writes it through the CLI from
// a staged scratch file. The re-offer rule after a decline may sit in either
// partial, so it is asserted across both.
func TestSplitFlowRequiresAgreementAndRedistributesThroughTheCLI(t *testing.T) {
	flow := flatTemplate(t, splitFlowPartial)

	requireAllContained(t, splitFlowPartial, flow,
		"Never take any step below without the user's explicit agreement to the split.",
		"every requirement and every acceptance criterion goes to **exactly one** spec",
		"a constraint or non-goal that applies to several specs is **copied into each** of them",
		"**2. One review over every resulting spec.**",
		"Spawn one subagent with a fresh context",
		"hand it every resulting spec",
		"no requirement or acceptance criterion appears in more than one spec",
		"Write one JSON description under `.spektacular/tmp/`",
		"{{command}} epic split --from .spektacular/tmp/epic_split.json",
		"rm .spektacular/tmp/epic_split.json",
		"**On a decline,** nothing is written",
	)

	both := flatTemplate(t, splitCheckPartial) + " " + flow
	requireAllContained(t, splitCheckPartial+" + "+splitFlowPartial, both,
		"Do not repeat the offer unless the scope visibly grows after the decline",
		"a new strong signal, or a new independent requirement group",
	)
}

// Every step that runs before the spec is complete records a split request
// in the working context and defers acting on it to the split step.
func TestSectionStepsRecordAMidWorkflowSplitRequest(t *testing.T) {
	steps := append([]string{"steps/spec/00b-interview.md"}, specSectionGatheringSteps...)
	for _, f := range steps {
		body := flatTemplate(t, f)
		requireAllContained(t, f, body,
			splitRequestMarker,
			"do not split yet: a split always acts on a complete spec",
			"Record the request in `.spektacular/working-context.md`",
			"acted on at the `split` step",
		)
	}
}

// The finished step offers to chain to the next item of the epic, reading the
// epic and starting the next spec seeded from its source and joined to the
// epic, and stops at specifying.
func TestFinishedStepChainsWithinTheEpicAndStopsAtSpecifying(t *testing.T) {
	body := flatTemplate(t, finishedStepTmpl)

	requireAllContained(t, finishedStepTmpl, body,
		"{{#epic_name}}",
		"{{/epic_name}}",
		"**Offer the next item in the epic.**",
		"{{command}} epic read {{epic_name}}",
		`{{command}} spec new --data '{"name":"<name>","sources":[{"uri":"<the child item's link>"}],"epic":"{{epic_name}}"}'`,
		"Never start it without the user's agreement.",
		"The offer stops at specifying: do not offer to plan or implement the next spec.",
	)

	open := strings.Index(body, "{{#epic_name}}")
	closeAt := strings.Index(body, "{{/epic_name}}")
	offer := strings.Index(body, "**Offer the next item in the epic.**")
	require.True(t, open < offer && offer < closeAt, "the chaining offer must render only for a spec in an epic")
}
