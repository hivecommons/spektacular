package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/migrate"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/spf13/cobra"
)

// gate is the root command's persistent pre-run hook. It refuses to run a
// command against a project whose settings are behind the current format,
// whose installed skills are stale or unrecorded, or whose settings were
// written by a newer Spektacular, so nobody keeps working against a file
// whose meaning has changed. It asks the upgrade engine, so it reports
// exactly what `migrate` would change.
//
// It lets through commands annotated gateExempt (migrate, init, version
// check), cobra's help and completion, and the root command itself, so the
// way out is always reachable. Outside a project it does nothing: commands
// report no_project as before. A project whose settings cannot even be read
// is also let through, for the command's own loading to report.
//
// A subcommand that defines its own PersistentPreRun(E) would shadow this
// hook for itself and its children; none does.
func gate(cmd *cobra.Command, _ []string) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	if err := refuseInsideSpecWorktree(cmd, root); err != nil {
		return err
	}
	if gateExempted(cmd) {
		return nil
	}
	cfgPath := filepath.Join(config.ProjectConfigDir(root), config.ProjectConfigFileName)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return nil
	}

	rep, err := migrate.Inspect(root, version)
	if err != nil {
		if refusal := formatRefusal(err); refusal != nil {
			return refusal
		}
		// Anything else (an unreadable config.yaml, say) is not an upgrade
		// matter: the command runs and its own loading reports the problem
		// with the remediation that fits it. The loaders still refuse an
		// out-of-date file, so nothing runs against one.
		return nil
	}

	command := migrate.PeekCommand(cfgPath)
	next := fmt.Sprintf("run `%s migrate` to upgrade (preview the changes with `%s migrate --dry-run`); nothing is changed until you do", command, command)

	if len(rep.Files) > 0 {
		behind := make([]string, 0, len(rep.Files))
		for _, f := range rep.Files {
			behind = append(behind, fmt.Sprintf("%s (format %d, needs %d)", f.Path, f.From, f.To))
		}
		return output.NewError("upgrade_required", "this project's settings are out of date for this Spektacular: "+strings.Join(behind, ", ")).
			WithResource(rep.Files[0].Path).
			WithNextAction(next)
	}
	if rep.Skills.Status != "match" {
		installed := "not recorded"
		if rep.Skills.Installed != "" {
			installed = "from Spektacular " + rep.Skills.Installed
		}
		return output.NewError("upgrade_required", fmt.Sprintf("this project's installed agent skills are %s; this Spektacular is %s", installed, version)).
			WithResource(cfgPath).
			WithNextAction(next)
	}
	return nil
}

// gateExempted reports whether cmd runs regardless of the project's state.
// refuseInsideSpecWorktree refuses a command run from inside a spec's
// worktree. A worktree holds only code: the .spektacular it carries is a
// stale copy whose stores, workflow state and relative repo locations are all
// wrong from there, and anything written to it is refused when the branch is
// merged back. Every command but help, completion and version is refused, so
// the way to the project root is the only way on.
func refuseInsideSpecWorktree(cmd *cobra.Command, cwd string) error {
	project, ok := specWorktreeProject(cwd)
	if !ok || cmd == rootCmd {
		return nil
	}
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "completion", "version", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return nil
		}
	}
	return output.NewError("inside_spec_worktree",
		fmt.Sprintf("%s is inside a spec's worktree, which holds only code; Spektacular is only ever run from the project root, %s", cwd, project)).
		WithResource(cwd).
		WithNextAction(fmt.Sprintf("re-run the same command from %s; the implement workflow's instructions name where each repo's code lives in the spec's worktrees", project))
}

// specWorktreeProject reports the project root holding cwd's spec worktree,
// when cwd is inside one: spec worktrees all live under the project's
// .spektacular/worktrees folder.
func specWorktreeProject(cwd string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	marker := string(filepath.Separator) + filepath.Join(".spektacular", worktree.Dir) + string(filepath.Separator)
	i := strings.Index(cwd+string(filepath.Separator), marker)
	if i <= 0 {
		return "", false
	}
	return cwd[:i], true
}

func gateExempted(cmd *cobra.Command) bool {
	if cmd == rootCmd {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[gateAnnotation] == gateExempt {
			return true
		}
		switch c.Name() {
		case "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
}
