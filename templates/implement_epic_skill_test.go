package templates_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// The spek-implement-epic skill drives the implement workflow for every
// planned spec of an epic through child agents, each in its own worktrees.
// These tests pin its contract on the template itself, rendered exactly as the
// installer renders a workflow skill (mustache, partials from templates.FS,
// only {{command}} as data), so they hold before and after the skill is
// registered for installation.

const implementEpicSkill = "skills/workflows/spek-implement-epic/SKILL.md"

// renderImplementEpicSkill renders the skill with the given command name and
// requires every partial and tag to resolve.
func renderImplementEpicSkill(t *testing.T, command string) string {
	t.Helper()
	out, err := mustache.RenderPartials(readTemplate(t, implementEpicSkill), stepkit.FSPartials{FS: templates.FS},
		map[string]string{"command": command})
	require.NoError(t, err, "the skill must render with every partial resolved")
	require.NotContains(t, out, "{{", "no mustache tag may survive installation")
	return out
}

func installedImplementEpicSkill(t *testing.T) string {
	t.Helper()
	return renderImplementEpicSkill(t, installedCommand)
}

// Criterion 1: the skill is triggered by plain wording.
func TestImplementEpicSkillIsTriggeredByPlainWording(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	require.Regexp(t, `(?m)^name: spek-implement-epic$`, skill)
	desc := regexp.MustCompile(`(?m)^description: (.*)$`).FindStringSubmatch(skill)
	require.Len(t, desc, 2, "the skill must carry a description line")
	requirePhrases(t, "the skill's trigger description", desc[1],
		`"implement this epic"`, `"build the epic"`, `"implement <epic name>"`,
	)
}

// The installed skill opens on the version check, renders {{command}}
// everywhere, and leaves no tag behind.
func TestImplementEpicSkillRendersLikeAnInstalledSkill(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	partial, err := mustache.Render(readTemplate(t, versionCheckRaw), map[string]string{"command": installedCommand})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(strings.TrimLeft(planEpicBody(t, skill), "\n"), strings.TrimRight(partial, "\n")),
		"the skill body must open with the version-check partial")

	raw := readTemplate(t, implementEpicSkill)
	want := strings.Count(raw, "{{command}}") + strings.Count(readTemplate(t, versionCheckRaw), "{{command}}")
	custom := renderImplementEpicSkill(t, "spekx")
	require.Equal(t, want, strings.Count(custom, "spekx"), "every {{command}} must render to the installed command")
	require.NotRegexp(t,
		"`spektacular |\\bspektacular (status|plan|spec|epic|repo|version|migrate|implement|changelog)\\b", custom,
		"no CLI mention may bypass {{command}}")
	requirePhrases(t, "the skill rendered with a custom command", flat(custom),
		"`spekx status <epic> --format json`",
		"`spekx status <spec> --format json`",
		"`spekx implement new --data '{\"name\":\"<spec>\",\"orchestrated\":true}'`",
		"spekx epic worktree --data '{\"spec\":\"<spec>\"}'",
		"spekx epic merge --data '{\"spec\":\"<spec>\"}'",
		"`spekx plan file read`",
		"`spekx plan file write --from <path>`",
		"`spekx changelog file`",
		"`spekx epic list`",
		"`spekx epic write`",
		"the project root to run every `spekx` command from",
		"it never runs `spekx` from inside a worktree",
		"`spekx design`",
		"`spekx spec amend`",
		"`spekx spec file read <spec>`",
		"`spekx spec amend --data '{\"name\":\"<spec>\",\"reason\":\"<why>\",\"run\":\"epic <epic> implement run, task <task>\"}' --from <staged file>`",
		"`spekx design author`",
		"`spekx design write`",
	)
}

// Criterion 2: nothing is implemented while a spec is unplanned or the
// dependencies are broken, and the problem is named.
func TestImplementEpicSkillRefusesWhenImplementingIsBlocked(t *testing.T) {
	check := flat(section(t, installedImplementEpicSkill(t), "# Step 2: Check up front"))
	requirePhrases(t, "Check up front", check,
		"Run `"+installedCommand+" status <epic> --format json` and read `epic.run`.",
		"If `epic.run.problems` holds any problem whose `blocks` includes `implement`, implement nothing.",
		"Relay each problem's `message` to the user",
		"`epic_unplanned` names the specs with no plan yet",
		"`epic_dependency_cycle` names the specs whose dependencies form a cycle",
		"`epic_dependency_outside` names a dependency outside the epic that is not implemented",
		"Then stop.",
	)
}

// Criterion 3: each independent spec runs in its own worktree alongside the
// others and is merged back before its dependents start.
func TestImplementEpicSkillRunsSpecsInWorktreesAndMergesBeforeDependents(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	what := flat(section(t, skill, "# What this skill does"))
	requirePhrases(t, "What this skill does", what,
		"Each spec is built in its **own git worktrees**",
		"on branch `spek/<spec>`, under `.spektacular/worktrees/<spec>/`",
		"specs that do not depend on each other are implemented side by side without touching each other's files",
		"you merge it back into every repo's main line before anything that depends on it starts",
		"The worktrees hold only code: every child runs `"+installedCommand+"` from the project root",
		"the implement workflow itself tells it where each repo's code lives in its spec's worktrees",
		"Specs, plans, changelogs and progress stay in the project",
		"their progress can be followed in the project while they run",
		"It never implements the epic itself",
	)

	loop := flat(section(t, skill, "# Step 3: The loop"))
	requirePhrases(t, "The loop", loop,
		"Repeat until nothing is left to start or merge and no child is running",
		"read each spec's `run.implement`",
		"For each spec that is `awaiting_merge`, merge it now (Step 5).",
		"For each spec that is `ready`, create its worktrees:",
		installedCommand+" epic worktree --data '{\"spec\":\"<spec>\"}'",
		"Then start a child to **start** it.",
		"specs that are `blocked` wait for the specs in their `waiting_on`, which must be implemented **and merged** first.",
		"independent specs are implemented in overlapping time",
		"Start every ready spec; there is no limit on how many run at once.",
		"**In a headless session, keep every child inside your turn.**",
		"start the ready specs as foreground sub-agents in a single message, so they still run in overlapping time, and handle every hand-back (Step 6) before you end the turn.",
		"A child that handed back a `QUESTION:` and is waiting for its answer still counts as running.",
		"Never start a second child for a spec that already has one, waiting or not.",
	)

	merge := flat(section(t, skill, "# Step 5: Merging a finished spec"))
	requirePhrases(t, "Merging a finished spec", merge,
		"Merge a spec as soon as its child hands back `DONE:`, and whenever `status` reports it `awaiting_merge`",
		installedCommand+" epic merge --data '{\"spec\":\"<spec>\"}'",
		"so specs that depend on it can start",
	)

	handling := flat(section(t, skill, "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"**`DONE:`** — merge the spec (Step 5)",
		"then re-read `status` and start the specs that have just become ready",
	)
}

// Criterion 4: a merge conflict stops the run and is shown to the user, never
// resolved silently.
func TestImplementEpicSkillStopsOnMergeConflict(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	merge := flat(section(t, skill, "# Step 5: Merging a finished spec"))
	requirePhrases(t, "Merging a finished spec", merge,
		"The merge is all or nothing across the spec's repos.",
		"On `epic_merge_conflict`, nothing was merged in any repo",
		"show the user the conflicting paths for each repo",
		"leave the worktrees in place",
		"enter stopping mode",
		"Never resolve a conflict yourself.",
	)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"Never resolve a merge or git conflict yourself, and never merge, rebase or switch branches",
	)

	report := flat(section(t, skill, "# Step 8: The final report"))
	requirePhrases(t, "The final report", report, "stopped by which failure or conflict")
}

// Criterion 5: no input is asked between specs unless a genuine question or
// failure arises.
func TestImplementEpicSkillAsksNothingBetweenSpecs(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"tasks run one after another without asking between them",
		"you never ask the user anything yourself",
	)

	check := flat(section(t, skill, "# Step 2: Check up front"))
	requirePhrases(t, "Check up front", check,
		"If `epic.run.dirty` is true",
		"`epic.run.dirty_repos` names each one",
		"A dirty repo the epic does not build is never reported",
		"Name the repos in `dirty_repos` to the user and ask once whether to commit that work first",
		"Apart from settling which epic is meant, this is the only start-of-run question.",
		"If they decline, go ahead, and say that uncommitted code will not be in any spec's worktree.",
		"Specs, plans and progress are read from the project itself, so they need no commit to be seen.",
	)

	relay := flat(section(t, skill, "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", relay,
		"**`QUESTION:`** — put the question to the user.",
		"Present it as ordinary text first, naming the spec, the options and the recommended default, then ask.",
		"the other children keep going",
		"Send the user's answer back to the same child",
		"Do one or the other, never both",
		"only one child may work on a spec at a time",
		"In a headless session, always start the fresh child, in the foreground",
	)

	// Every mention of asking, outside the version-check partial, is one of
	// the sanctioned ones.
	partial, err := mustache.Render(readTemplate(t, versionCheckRaw), map[string]string{"command": installedCommand})
	require.NoError(t, err)
	body := flat(strings.Replace(planEpicBody(t, skill), partial, "", 1))
	allowed := []string{
		"so a question the user already answered is not asked again",
		"ask the user which epic they mean, but only when it is still ambiguous",
		"Name the repos in `dirty_repos` to the user and ask once whether to commit that work first; the answer is theirs.",
		"tasks run one after another without asking between them",
		"you never ask the user anything yourself",
		"A child never asks whether to continue to its next task.",
		"naming the spec, the options and the recommended default, then ask.",
	}
	for _, loc := range askMention.FindAllStringIndex(body, -1) {
		lo, hi := max(0, loc[0]-80), min(len(body), loc[1]+80)
		ok := false
		for _, a := range allowed {
			if i := strings.Index(body, a); i >= 0 && i <= loc[0] && loc[1] <= i+len(a) {
				ok = true
				break
			}
		}
		require.Truef(t, ok, "unsanctioned stop for the user: %q", body[lo:hi])
	}

	// The knowledge offer is the only end-of-run question, and nothing is
	// saved unless the user accepts.
	report := flat(section(t, skill, "# Step 8: The final report"))
	requirePhrases(t, "The final report", report,
		"Offer to save any durable discovery the children reported with the `spek-knowledge` skill, and save it only if the user accepts.",
	)
}

// Criterion 6: repeating the request resumes in-progress specs in their
// worktrees and skips implemented ones.
func TestImplementEpicSkillResumesOnARepeatedRequest(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	what := flat(section(t, skill, "# What this skill does"))
	requirePhrases(t, "What this skill does", what,
		"repeating the request after an interruption picks up exactly where the epic stands",
		"implemented specs are skipped, an interrupted spec resumes from its record in the project, with its code in the same worktrees, and a finished one is merged",
		"When the request is repeated, read them back first",
	)

	loop := flat(section(t, skill, "# Step 3: The loop"))
	requirePhrases(t, "The loop", loop,
		"For each spec that is `in_progress` and has no child running, start a child to **resume** it (its `root` is the spec's worktree, where its code lives).",
		"Specs that are `done` are skipped",
	)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"to **start** or **resume**: run `"+installedCommand+` implement new --data '{"name":"<spec>","orchestrated":true}'`+"`",
		"If it returns a resume report for the spec's lane",
		"then run the `goto` it gives",
		"Follow the `spek-implement` skill.",
	)
}

// The hand-back contract, the genuine-question definition and stopping mode
// are all defined.
func TestImplementEpicSkillDefinesHandBackAndStopping(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	contract := flat(section(t, skill, "## The hand-back contract"))
	requirePhrases(t, "The hand-back contract", contract,
		"final message whose **first line** is exactly one of",
		"`DONE: <spec>`", "`QUESTION: <spec>`", "`FAILED: <spec>`",
		"followed by the step it reached and the reason it cannot go on",
		"the question, the options, and the default it recommends",
	)

	questions := flat(section(t, skill, "## What counts as a genuine open question"))
	requirePhrases(t, "What counts as a genuine open question", questions,
		"Only a stop the implement workflow itself defines earns a question: the plan no longer matching the code, "+
			"the spec or a design it references being wrong, a task outgrowing its scope, "+
			"a verification failure the child cannot fix within the task, "+
			"or the goal check running out of attempts or judging a criterion impossible.",
		"Everything else the child decides itself and records in the plan's changelog.",
		"A child never asks whether to continue to its next task.",
	)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the hand-back contract and the definition of a genuine open question, below.",
		"You never write the spec's text or a design document.",
		"If the spec or a design it references is wrong, propose the amendment in a `QUESTION:` hand-back, "+
			"naming the document, the section, the conflict and the change you propose.",
	)

	handling := flat(section(t, skill, "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"**`FAILED:`** — enter stopping mode.",
		"**The user declines to answer now** — enter stopping mode.",
		"**Stopping mode:** start no new child, let the children already running finish",
		"and merge each one that finishes cleanly",
		"repeating the request picks it up",
		"Name the spec that stopped the run and why.",
	)
}

// The orchestrator keeps its notes in working-context.md, restates the
// store-access rule to itself and to every child, reports progress and ends
// on a completed/skipped/outstanding/worktrees report.
func TestImplementEpicSkillCarriesNotesStoreAccessProgressAndReport(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	require.Contains(t, flat(section(t, skill, "# What this skill does")), "`.spektacular/working-context.md`")

	store := flat(section(t, skill, "# Spektacular's files are reached through Spektacular"))
	requirePhrases(t, "the store-access rule", store,
		"Never use your own file tools on a store directory, and never build a store path by hand.",
		"`"+installedCommand+" plan file read`",
		"`"+installedCommand+" plan file write --from <path>`",
		"`"+installedCommand+" spec file`",
		"`"+installedCommand+" changelog file`",
		"`"+installedCommand+" epic`",
		"and designs with `"+installedCommand+" design`.",
		"A spec is amended during a run only with `"+installedCommand+" spec amend`.",
		"Every child prompt must restate this rule",
	)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the spec name, and the project root to run every `"+installedCommand+"` command from.",
		"The implement workflow's own instructions name where each repo's code lives in the spec's worktrees;",
		"it never runs `"+installedCommand+"` from inside a worktree and never touches a worktree's `.spektacular` directory;",
		"the store-access rule above, word for word;",
	)

	progress := flat(section(t, skill, "# Step 7: Progress"))
	requirePhrases(t, "Progress", progress,
		"After every child you start, every hand-back and every merge, tell the user in one line which specs are being implemented",
		"how many remain",
	)

	report := flat(section(t, skill, "# Step 8: The final report"))
	requirePhrases(t, "The final report", report,
		"End every run, finished or stopped",
		"**Completed in this run:**", "**Skipped:**", "**Still outstanding:**", "**Worktrees left behind:**",
	)
}

// Each child runs Spektacular from the project root; the worktrees hold only
// code. The old model, where a child worked from its spec's project worktree
// and resolved every repo inside it, must not come back.
func TestImplementEpicSkillRunsSpektacularFromTheProjectRoot(t *testing.T) {
	skill := flat(installedImplementEpicSkill(t))

	child := flat(section(t, installedImplementEpicSkill(t), "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the project root to run every `"+installedCommand+"` command from",
	)

	handling := flat(section(t, installedImplementEpicSkill(t), "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"start a fresh child on the same spec with the answer included in its prompt; it resumes from its lane.",
	)

	for _, old := range []string{
		"repo list`, run from there",
		"the root to work in: the spec's project worktree",
		"resolves to that spec's worktrees",
		"in the returned `project` path",
		"repo list",
		"resumes in its worktree",
		"in the same worktree.",
	} {
		require.NotContainsf(t, skill, old,
			"the skill must not tell a child to work from, or resolve repos inside, a worktree (found %q)", old)
	}
}

// A child that finds the spec or a referenced design wrong proposes the
// amendment; the orchestrator applies it from the project root, only once the
// user explicitly approves, and answers the child with what changed so it
// re-reads and re-verifies. A rejection is answered too.
func TestImplementEpicSkillAppliesApprovedAmendmentsFromTheProjectRoot(t *testing.T) {
	handling := flat(section(t, installedImplementEpicSkill(t), "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"**An amendment the user approves** — when a `QUESTION:` proposes amending the spec or a design",
		"apply it yourself only after the user's explicit approval",
		"from the project root and never in a worktree",
		"read it with `"+installedCommand+" spec file read <spec>`",
		"change only the sections the user approved",
		"stage the full result under `.spektacular/tmp/<spec>/`",
		"`"+installedCommand+` spec amend --data '{"name":"<spec>","reason":"<why>","run":"epic <epic> implement run, task <task>"}' --from <staged file>`+"`",
		"revise it with `"+installedCommand+" design author`",
		"`"+installedCommand+" design write` for a design they wrote",
		"then record it with `"+installedCommand+" spec amend` and a `\"design\":{\"source\":\"<name>\",\"path\":\"<path>\"}` field",
		"Note the amendment in `.spektacular/working-context.md`",
		"then answer the child naming what changed, so it re-reads the amended text and re-verifies its task.",
		"If the user rejects the amendment, answer the child with their decision.",
	)

	// The amendment is applied by the orchestrator, never by the child.
	child := flat(section(t, installedImplementEpicSkill(t), "# Step 4: The child prompt"))
	require.NotContains(t, child, "spec amend", "a child never runs spec amend itself")
}

// A child that ends without a hand-back was cut off by its agent's limit on
// how long one sub-agent may run. The skill resumes it from its lane and
// worktrees straight away, gives up after two cut-offs at the same step, and
// reports every resume with the remedy.
func TestImplementEpicSkillResumesAChildCutOffWithoutAHandBack(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	handling := flat(section(t, skill, "# Step 6: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"**No hand-back at all**",
		"does not open with `DONE:`, `QUESTION:` or `FAILED:`",
		"cut off by the agent's own limit on how long one sub-agent may run",
		"its code stays in its worktrees and its progress in its lane",
		"Start a fresh child on the same spec straight away, on your own and without waiting for the user, with the **resume** prompt",
		"If a spec is cut off twice in a row at the same step in `status`, treat it as `FAILED:`.",
	)

	loop := flat(section(t, skill, "# Step 3: The loop"))
	requirePhrases(t, "The loop", loop,
		"A child that ended without a hand-back is no longer running (Step 6).",
	)

	report := flat(section(t, skill, "# Step 8: The final report"))
	requirePhrases(t, "The final report", report,
		"**Resumed after being cut off:**",
		"`session.maxTurns` in `~/.bob/settings/settings.json`",
	)
}

// A merge refused only because another merge or commit held the project's
// commit lock is retried, not treated as a stop.
func TestImplementEpicSkillRetriesAMergeThatWaitedTooLongForTheLock(t *testing.T) {
	merging := flat(section(t, installedImplementEpicSkill(t), "# Step 5: Merging a finished spec"))
	requirePhrases(t, "Merging a finished spec", merging,
		"On `merge_lock_timeout`, another merge or commit in the project was still running and nothing was merged: run the same merge again.",
		"Any other refusal from `epic worktree` or `epic merge` is handled the same way as a conflict",
	)
}

// Goal-check stops are genuine questions: the orchestrator relays a child's
// exhausted or impossible goal check to the user, the child prompt says how
// to hand it back, and a spec with an open goal-check question is not merged.
func TestImplementEpicSkillTreatsGoalCheckStopsAsGenuineQuestions(t *testing.T) {
	out := installedImplementEpicSkill(t)
	require.Contains(t, out, "or the goal check running out of attempts or judging a criterion impossible.")
	require.Contains(t, out, "If the goal check stops your task, because its attempts ran out or it judged a criterion impossible, hand it back as a `QUESTION:`")
	require.Contains(t, out, "Never merge a spec while its goal-check question is open: `epic merge` refuses it (`goal_check_open`)")
}

// The orchestrator never schedules its own wake-up while children run: a
// pending timer or recurring job keeps the session alive after the work is
// done. The rule sits in the loop, after the headless paragraph, which is
// left word for word as it was.
func TestImplementEpicSkillNeverSetsATimedWakeUpWhileChildrenRun(t *testing.T) {
	loop := flat(section(t, installedImplementEpicSkill(t), "# Step 3: The loop"))

	rule := "**Never set a timed wake-up while children run.**"
	requirePhrases(t, "The loop", loop,
		rule,
		"Do not use a scheduled wake-up, a timer, a recurring job or a command that sleeps to check on your children",
		"each child's hand-back, or its completion notice, is what wakes you.",
		"A wake-up still pending once the work is done keeps the session running with nothing left to do.",
	)

	headless := "**In a headless session, keep every child inside your turn.** A session is headless when no one replies between your turns: you were started non-interactively, or your prompt says so. There, a sub-agent still running in the background after you end your turn may be stopped before it finishes; headless Claude Code stops one after ten minutes. So instead of starting children in the background, start the ready specs as foreground sub-agents in a single message, so they still run in overlapping time, and handle every hand-back (Step 6) before you end the turn."
	require.Contains(t, loop, headless, "the headless paragraph must be unchanged")

	running := "A child that handed back a `QUESTION:` and is waiting for its answer still counts as running."
	require.Less(t, strings.Index(loop, headless), strings.Index(loop, rule), "the rule follows the headless paragraph")
	require.Less(t, strings.Index(loop, rule), strings.Index(loop, running), "the rule comes before the running-child paragraph")
}

// Each child is told where its spec's code lives: the worktree paths that
// `epic worktree` reports, which it reports again unchanged on a resume.
func TestImplementEpicSkillNamesTheSpecsWorktreePathsInEveryChildPrompt(t *testing.T) {
	skill := installedImplementEpicSkill(t)

	child := flat(section(t, skill, "# Step 4: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the spec's code locations: the worktree path of every repo that `"+installedCommand+` epic worktree --data '{"spec":"<spec>"}'`+"` reports for the spec",
		"and that the child works on code only there.",
		"Running it again for a spec whose worktrees already exist reports the same paths, so a resumed child is given them too;",
	)
	require.Less(t, strings.Index(child, "- the spec name, and the project root"), strings.Index(child, "- the spec's code locations:"),
		"the code locations follow the spec name")
	require.Less(t, strings.Index(child, "- the spec's code locations:"), strings.Index(child, "- to **start** or **resume**:"),
		"the code locations come before the start-or-resume instruction")

	loop := flat(section(t, skill, "# Step 3: The loop"))
	requirePhrases(t, "The loop", loop,
		"Then start a child to **start** it. Its prompt names each worktree path the command reported (Step 4).",
	)
}
