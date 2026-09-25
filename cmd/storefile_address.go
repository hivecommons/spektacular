package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The artifact package owns the address grammar; this file owns turning its
// refusals into the error envelope. Only the command layer knows the verb the
// caller ran, the flags they set and the configured command name, which is
// what a next action needs to restate the caller's own command correctly
// spelled.

// storeCommandName is the configured way to invoke the CLI, for next actions.
// A project whose config cannot be loaded still gets a runnable suggestion.
func storeCommandName() string {
	if cfg, err := loadConfig(); err == nil && cfg.Command != "" {
		return cfg.Command
	}
	return "spektacular"
}

// addressString renders an address the way a caller types it: the feature,
// followed by the document for a plan.
func addressString(a artifact.Address) string {
	if a.Document != "" {
		return a.Feature + " " + a.Document
	}
	return a.Feature
}

// restateCommand rebuilds the command cmd was invoked as, with args in place
// of the caller's positional arguments and every flag the caller set carried
// over, so the result can be run as-is.
func restateCommand(cmd *cobra.Command, args []string) string {
	parts := []string{storeCommandName()}
	// CommandPath starts with the root command's own name, which the
	// configured command name replaces.
	path := strings.Fields(cmd.CommandPath())
	if len(path) > 1 {
		parts = append(parts, path[1:]...)
	}
	parts = append(parts, args...)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		// Visit also yields flags a previous invocation of the same
		// command tree set; Changed is what reflects this one.
		if !f.Changed {
			return
		}
		parts = append(parts, "--"+f.Name, shellWord(f.Value.String()))
	})
	return strings.Join(parts, " ")
}

// shellWord quotes s when it would not survive a shell as one word.
func shellWord(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"`$\\*?[]{}()<>|&;#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// addressRefusal renders an artifact.Parse failure for kind as the standard
// error envelope. Nothing has been read or written when it is raised.
func addressRefusal(cmd *cobra.Command, kind artifact.Kind, args []string, err error) error {
	input := strings.Join(args, " ")

	var extErr *artifact.ExtensionError
	if errors.As(err, &extErr) {
		corrected := []string{extErr.Corrected.Feature}
		rule := fmt.Sprintf("%s documents are addressed by the bare feature name", kind)
		if kind == artifact.KindPlan {
			rule = "plan documents are addressed as <feature> <document>, two arguments with no extension"
			// `plan file list <feature>` takes the feature alone.
			if cmd.Name() != "list" {
				doc := extErr.Corrected.Document
				if doc == "" {
					doc = "<document>"
				}
				corrected = append(corrected, doc)
			}
		}
		return output.NewError(artifact.ErrCodeUnexpectedExtension,
			fmt.Sprintf("%q carries a file extension or path; %s", extErr.Input, rule)).
			WithResource(input).
			WithNextAction(fmt.Sprintf("run `%s`", restateCommand(cmd, corrected)))
	}

	var docErr *artifact.DocumentRequiredError
	if errors.As(err, &docErr) {
		command := storeCommandName()
		return output.NewError(artifact.ErrCodeDocumentRequired,
			fmt.Sprintf("plan %q needs a document name; plan documents are addressed as <feature> <document>, such as %s plan", docErr.Feature, docErr.Feature)).
			WithResource(input).
			WithNextAction(fmt.Sprintf("run `%s plan file list %s` to see its documents, then name one, e.g. `%s`",
				command, docErr.Feature, restateCommand(cmd, []string{docErr.Feature, "plan"})))
	}

	return output.NewError("bad_input", fmt.Sprintf("invalid %s address %q: %v", kind, input, err)).
		WithResource(input).
		WithNextAction(fmt.Sprintf("run `%s` to see the names it accepts", listCommand(kind, "")))
}

// listCommand is the list command that shows the names available for kind,
// or for a plan feature's documents when feature is set.
func listCommand(kind artifact.Kind, feature string) string {
	c := fmt.Sprintf("%s %s file list", storeCommandName(), kind)
	if feature != "" {
		c += " " + feature
	}
	return c
}

// addressNotFound refuses a correctly spelled address that names no stored
// document, pointing at the list command that shows what does exist.
func addressNotFound(cmd *cobra.Command, addr artifact.Address) error {
	label := string(addr.Kind)
	if addr.Kind == artifact.KindPlan && addr.Document == "" {
		label = "plan feature"
	}
	next := listCommand(addr.Kind, "")
	if addr.Kind == artifact.KindPlan && addr.Document != "" {
		next = listCommand(addr.Kind, addr.Feature)
	}
	msg := fmt.Sprintf("no %s named %q", label, addressString(addr))
	if addr.Kind == artifact.KindPlan && addr.Document != "" {
		msg = fmt.Sprintf("plan %q has no document named %q", addr.Feature, addr.Document)
	}
	if repo, _ := cmd.Flags().GetString("repo"); repo != "" {
		msg += fmt.Sprintf(" in repo %q", repo)
		next += " --repo " + shellWord(repo)
	}
	return output.NewError("not_found", msg).
		WithResource(addressString(addr)).
		WithNextAction(fmt.Sprintf("run `%s` to see available names", next))
}
