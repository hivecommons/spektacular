package templates_test

import (
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

func TestImplementStepsExplicitlyDelegateCodeRootsAndDependencySetup(t *testing.T) {
	for _, file := range []string{"03-implement.md", "04-test.md", "05-verify.md"} {
		t.Run(file, func(t *testing.T) {
			out, err := mustache.RenderPartials(readTemplate(t, "steps/implement/"+file), stepkit.FSPartials{FS: templates.FS}, map[string]any{
				"config": map[string]string{"command": "spekx"},
			})
			require.NoError(t, err)
			requirePhrases(t, file, flat(out),
				"Use the repo `root` paths resolved by `read_plan` from this spec's workflow project root",
				"Pass the workflow project root and each relevant code root explicitly to **every sub-agent**",
				"Never substitute a main checkout for a spec's worktree",
				"ignored dependencies and build state such as `node_modules`",
				"Follow each repo's documented dependency setup in its worktree",
				"Do not install into the main checkout",
			)
		})
	}
}

func TestImplementEpicChecksMainCheckoutIsolation(t *testing.T) {
	skill := flat(installedImplementEpicSkill(t))
	requirePhrases(t, "epic checkout isolation", skill,
		"Name the repos in `epic.run.dirty_repos`",
		"match them against each spec's `run.implement.repos`",
		"Pass the workflow project root and the relevant worktree code roots explicitly to every sub-agent",
		"install ignored dependencies using each repo's documented setup inside its worktree",
		"record `git status --porcelain --untracked-files=all` from every registered repo's main code root",
		"including repos no spec touches",
		"repeat `git status --porcelain --untracked-files=all` in every main code root and compare with the isolation baseline",
		"Report unexpected changes as an isolation failure",
	)
}
