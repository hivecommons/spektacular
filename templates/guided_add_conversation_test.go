package templates

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// guidedAddStep is one rung of the guided add's conversation, written out by
// hand from Steps() in internal/steps/repo/steps.go. It is deliberately not
// read back from that package: the step table is the thing under test, so
// deriving the oracle from it would assert only that the table equals itself.
// The templates package is also imported by stepkit, which internal/steps/repo
// depends on, so an in-package test could not import it in any case.
type guidedAddStep struct {
	// File is the instruction rendered when the workflow enters Step.
	File string
	// Step is the workflow state name.
	Step string
	// Next is the state the engine advances to, which it substitutes into
	// the template's {{next_step}} placeholder. Empty for the terminal step.
	Next string
}

// guidedAddOrder is the fixed conversation order: locate is asked cold, the
// four proposals follow in this sequence, then placement, confirmation and
// the single write. The internal "new" step is absent because it renders no
// instruction of its own; it exists only to auto-advance into locate.
var guidedAddOrder = []guidedAddStep{
	{File: "steps/repo/01-locate.md", Step: "locate", Next: "name"},
	{File: "steps/repo/02-name.md", Step: "name", Next: "description"},
	{File: "steps/repo/03-description.md", Step: "description", Next: "role"},
	{File: "steps/repo/04-role.md", Step: "role", Next: "tags"},
	{File: "steps/repo/05-tags.md", Step: "tags", Next: "placement"},
	{File: "steps/repo/06-placement.md", Step: "placement", Next: "confirm"},
	{File: "steps/repo/07-confirm.md", Step: "confirm", Next: "register"},
	{File: "steps/repo/08-register.md", Step: "register", Next: "finished"},
	{File: "steps/repo/09-finished.md", Step: "finished", Next: ""},
}

// guidedAddProposalSteps are the four steps that arrive with an answer
// already drafted, in the order the user meets them.
var guidedAddProposalSteps = []string{
	"steps/repo/02-name.md",
	"steps/repo/03-description.md",
	"steps/repo/04-role.md",
	"steps/repo/05-tags.md",
}

// coldQuestionPhrases are the sentences that ask the user for the repo's
// folder from an empty hand. They belong to the locate step alone: every
// later step already knows where the code is, so a second one carrying any
// of these would be asking the user something they have already answered.
var coldQuestionPhrases = []string{
	"Ask which repo they want to add",
	"in terms of the folder its code is in",
	"Resolve their answer to an absolute path",
}

// TestGuidedAddOnlyLocateAsksCold verifies that the folder the repo lives in
// is asked for in exactly one place. The locate instruction branches on
// whether the add was started with that folder already named: the known
// branch must suppress the question outright rather than confirming it, and
// no later instruction may reintroduce it.
func TestGuidedAddOnlyLocateAsksCold(t *testing.T) {
	locate := mustReadTemplate(t, "steps/repo/01-locate.md")

	require.Contains(t, locate, "{{#repo_path}}",
		"the locate step must branch on the repo folder already being known")
	require.Contains(t, locate, "{{^repo_path}}",
		"the locate step must branch on the repo folder not yet being known")

	known := templateSection(t, locate, "steps/repo/01-locate.md", "repo_path", false)
	require.Contains(t, known, "Do not ask for it again",
		"the already-named branch must forbid asking for the folder a second time")

	unknown := templateSection(t, locate, "steps/repo/01-locate.md", "repo_path", true)
	for _, phrase := range coldQuestionPhrases {
		require.Containsf(t, unknown, phrase,
			"the cold branch of the locate step must carry the phrase %q", phrase)
	}

	// The cold question must live in the locate step and nowhere else.
	for _, p := range guidedAddProposalSteps {
		body := mustReadTemplate(t, p)
		for _, phrase := range coldQuestionPhrases {
			require.NotContainsf(t, body, phrase,
				"%s must not ask for the repo's folder; the locate step already did (found %q)", p, phrase)
		}
	}
}

// proposalStatements are the sentences that require the proposed value to be
// spoken inside the question, so that a bare "yes" is enough to record it.
// Each is hand-written from the instruction it belongs to: the wording
// differs per step because the value differs (a name, a sentence, a role, a
// set of tags), and pinning each one keeps a step from being reduced to a
// bare "propose something" without the state-it-in-the-question contract.
var proposalStatements = map[string]string{
	"steps/repo/02-name.md":        "state it inside the question so that agreeing alone is enough to record it",
	"steps/repo/03-description.md": "State the sentence inside the question so that agreeing alone is enough to record it",
	"steps/repo/04-role.md":        "state it inside the question so that agreeing alone is enough to record it",
	"steps/repo/05-tags.md":        "state them inside the question so that agreeing alone is enough to record them",
}

// singleTopicStatements are the sentences that confine each proposal step to
// one topic. Steps 02 to 04 say so directly, because each has later questions
// it could bundle into the same message. The tags step is the last of the
// four and has nothing left to bundle, so it scopes the turn by saying so.
var singleTopicStatements = map[string]string{
	"steps/repo/02-name.md":        "Ask about exactly one thing here.",
	"steps/repo/03-description.md": "Ask about exactly one thing here.",
	"steps/repo/04-role.md":        "Ask about exactly one thing here.",
	"steps/repo/05-tags.md":        "This is the last of the four questions.",
}

// TestGuidedAddProposalStepsStateAValueAndAskOneThing verifies the shape
// shared by the four questions that follow locate: each branches on whether
// the repo could be read, each puts its proposed value inside the question
// when it could, and each confines itself to a single topic so the user is
// never handed four questions at once.
func TestGuidedAddProposalStepsStateAValueAndAskOneThing(t *testing.T) {
	for _, p := range guidedAddProposalSteps {
		// An empty expectation would make every Contains below vacuous, so
		// the oracle is required to cover the step before it is used.
		require.Containsf(t, proposalStatements, p, "no proposal statement written down for %s", p)
		require.Containsf(t, singleTopicStatements, p, "no single-topic statement written down for %s", p)

		body := mustReadTemplate(t, p)

		require.Containsf(t, body, "{{#evidence.readable}}",
			"%s must branch on the repo being readable", p)
		require.Containsf(t, body, "{{^evidence.readable}}",
			"%s must branch on the repo saying nothing readable about itself", p)

		readable := templateSection(t, body, p, "evidence.readable", false)
		require.Containsf(t, readable, proposalStatements[p],
			"%s must state its proposed value inside the question", p)

		unreadable := templateSection(t, body, p, "evidence.readable", true)
		require.Containsf(t, unreadable, "you have nothing to propose",
			"%s must not invent a proposal when the repo says nothing about itself", p)

		require.Containsf(t, body, singleTopicStatements[p],
			"%s must confine its turn to a single topic", p)
	}
}

// TestGuidedAddStepsFollowAFixedOrder verifies the conversation order and
// that no instruction hard-codes the state it advances to. The engine owns
// the destination and supplies it as {{next_step}}; a template naming a
// successor literally would silently diverge from the step table the moment
// the order changed.
func TestGuidedAddStepsFollowAFixedOrder(t *testing.T) {
	// The instruction files on disk are exactly the nine expected, in the
	// order their numeric prefixes imply.
	var found []string
	err := fs.WalkDir(FS, "steps/repo", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || path.Ext(p) != ".md" {
			return nil
		}
		found = append(found, p)
		return nil
	})
	require.NoError(t, err, "walking steps/repo")
	sort.Strings(found)

	expected := make([]string, 0, len(guidedAddOrder))
	for _, s := range guidedAddOrder {
		expected = append(expected, s.File)
	}
	require.Equal(t, expected, found,
		"steps/repo must hold exactly the nine guided-add instructions, in conversation order")

	// The hand-written chain is unbroken: every step's successor is the step
	// whose instruction comes next.
	for i := 0; i < len(guidedAddOrder)-1; i++ {
		require.Equalf(t, guidedAddOrder[i+1].Step, guidedAddOrder[i].Next,
			"the step after %q must be %q", guidedAddOrder[i].Step, guidedAddOrder[i+1].Step)
	}
	require.Empty(t, guidedAddOrder[len(guidedAddOrder)-1].Next,
		"the terminal step advances nowhere")

	const gotoPrefix = `repo goto --data '{"step":"{{next_step}}"`
	for _, s := range guidedAddOrder {
		body := mustReadTemplate(t, s.File)

		if s.Next == "" {
			require.NotContainsf(t, body, "repo goto",
				"terminal instruction %s must not advance anywhere", s.File)
			continue
		}

		require.Containsf(t, body, gotoPrefix,
			"%s must advance via {{next_step}} rather than naming %q itself", s.File, s.Next)
		require.NotContainsf(t, body, `"step":"`+s.Next+`"`,
			"%s must not hard-code %q as its successor", s.File, s.Next)
	}
}

// delegationOfferMarker is a hand-written substring of the one-time offer to
// fill in the remaining answers. The offer has to be made once: repeated at
// every question it becomes nagging, and never made at all the user has no
// way out of a four-turn interview they did not want.
const delegationOfferMarker = "Make the offer here and nowhere else."

// delegationPayload is the call that carries all four values at once, so a
// user who hands the rest over skips the three remaining questions instead of
// being asked them one at a time with pre-filled answers.
const delegationPayload = `'{"step":"placement","name":"<name>","description":"<description>","role":"<role>","tags":["<tag>"]}'`

// TestGuidedAddOffersDelegationExactlyOnce verifies that the offer to take
// the rest of the answers is made in one instruction only, that it carries
// the shortcut into placement with every value, and that the later questions
// still know what to do if the user hands over mid-way.
func TestGuidedAddOffersDelegationExactlyOnce(t *testing.T) {
	var offering []string

	err := fs.WalkDir(FS, "steps/repo", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || path.Ext(p) != ".md" {
			return nil
		}
		body := mustReadTemplate(t, p)
		count := strings.Count(body, delegationOfferMarker)
		require.LessOrEqualf(t, count, 1,
			"%s repeats the one-time delegation offer %d times", p, count)
		if count == 1 {
			offering = append(offering, p)
		}
		return nil
	})
	require.NoError(t, err, "walking steps/repo")

	require.Equal(t, []string{"steps/repo/02-name.md"}, offering,
		"the delegation offer belongs to the first proposal and to no other step")

	name := mustReadTemplate(t, "steps/repo/02-name.md")
	require.Contains(t, name, delegationPayload,
		"the delegation shortcut must carry name, description, role and tags in one call to placement")

	// The three later questions never re-offer, but must still act on a
	// hand-over that arrives unprompted.
	for _, p := range []string{
		"steps/repo/03-description.md",
		"steps/repo/04-role.md",
		"steps/repo/05-tags.md",
	} {
		body := mustReadTemplate(t, p)
		require.Containsf(t, body, "If the user hands the rest over",
			"%s must handle a hand-over that arrives without being offered", p)
		require.Containsf(t, body, "carry every one of the four together in a single call to the placement step",
			"%s must skip the remaining questions rather than ask them", p)
	}
}

// TestGuidedAddPlacementDefaultIsSilent verifies that the common case costs
// the user no turn at all. Where the project's files can go in the repo being
// added, that is taken without comment; only where it is impossible does the
// step ask, and then in the user's terms rather than by reporting the
// obstacle it hit.
func TestGuidedAddPlacementDefaultIsSilent(t *testing.T) {
	body := mustReadTemplate(t, "steps/repo/06-placement.md")

	writable := templateSection(t, body, "steps/repo/06-placement.md", "writable", false)
	require.Contains(t, writable, "Take it silently.",
		"the default placement must be taken without comment")
	require.Contains(t, writable, "do not offer it as a choice",
		"the default placement must not be presented as a decision for the user")
	require.Contains(t, writable, "do not ask the user to approve it here",
		"the default placement must not be confirmed here; the confirmation step covers it")

	unwritable := templateSection(t, body, "steps/repo/06-placement.md", "writable", true)
	require.Contains(t, unwritable, "you do have to ask",
		"placement must be asked about when the default is unavailable")
	require.Contains(t, unwritable, "Do not explain the obstacle in Spektacular's terms",
		"the unavailable default must be raised in the user's terms")
	require.Contains(t, unwritable, "do not quote the error",
		"the unavailable default must not be justified by a raw error")
}

// TestGuidedAddConfirmationNamesEveryFact verifies that the last stop before
// anything is written tells the user all three things they need to judge it:
// which repo is being registered, what folder appears on their disk, and
// where the code lives. It must also treat only an explicit yes as a yes.
func TestGuidedAddConfirmationNamesEveryFact(t *testing.T) {
	body := mustReadTemplate(t, "steps/repo/07-confirm.md")

	require.Contains(t, body, "Nothing has been written yet.",
		"the confirmation step must establish that nothing is written until the user agrees")

	for _, fact := range []string{
		"which repo is being registered, and what it will be known as",
		"which folder will be created, by its full path",
		"where its code lives",
	} {
		require.Containsf(t, body, fact,
			"the confirmation must state %q", fact)
	}

	require.Contains(t, body, "A confirmation has to be explicit.",
		"the confirmation step must require an explicit agreement")
	require.Contains(t, body, "Silence, a change of subject, or an ambiguous reply is not agreement",
		"the confirmation step must rule out silence and ambiguity as agreement")
	require.Contains(t, body, "neither is the absence of an objection",
		"the confirmation step must rule out an unvoiced objection as agreement")
}

// TestGuidedAddInstructionsUseNoEmDashes verifies the project's prose
// convention across every guided-add instruction. The standing
// working-context footer is appended at render time rather than carried in
// the template files, so every line of each file is authored prose.
func TestGuidedAddInstructionsUseNoEmDashes(t *testing.T) {
	checked := 0

	err := fs.WalkDir(FS, "steps/repo", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || path.Ext(p) != ".md" {
			return nil
		}
		checked++

		require.NotContainsf(t, mustReadTemplate(t, p), "—",
			"%s uses an em dash in its authored prose", p)
		return nil
	})
	require.NoError(t, err, "walking steps/repo")

	require.Equal(t, len(guidedAddOrder), checked,
		"every guided-add instruction must be checked for em dashes")
}

// templateSection returns the body of a Mustache section from a template.
// Passing inverted selects the {{^name}} block rather than the {{#name}} one,
// which is how these instructions carry their two branches. The section must
// exist: a missing branch is a failure of the template, not of the test.
func templateSection(t *testing.T, body, name, section string, inverted bool) string {
	t.Helper()

	open := "{{#" + section + "}}"
	if inverted {
		open = "{{^" + section + "}}"
	}
	closeTag := "{{/" + section + "}}"

	start := strings.Index(body, open)
	require.GreaterOrEqualf(t, start, 0, "%s has no %s section", name, open)

	rest := body[start+len(open):]
	end := strings.Index(rest, closeTag)
	require.GreaterOrEqualf(t, end, 0, "%s has no %s closing %s", name, closeTag, open)

	return rest[:end]
}
