package design

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// Every refusal in this package names what went wrong and a runnable next
// step. That is the repo-wide rule, and it earns its keep here in particular:
// four of this feature's acceptance criteria are statements about what happens
// when something is wrong, and an agent handed a vague refusal tends to
// abandon the CLI and reach for raw file tools instead of correcting its call.

// availableSources renders the declared source names for a next action, or
// says plainly that there are none. A caller that named the wrong source needs
// to see what it could have named instead; a caller working in a project that
// declares no design sources at all needs to be told that, not handed an empty
// list to puzzle over.
func availableSources(names []string) string {
	if len(names) == 0 {
		return "this project declares no design sources; add one under design.sources in config.yaml"
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("%q", n))
	}
	return fmt.Sprintf("declared design sources are %s; run 'design sources' to see their locations", strings.Join(quoted, ", "))
}

// unknownSource refuses an address naming a source the project has not
// declared. It never falls back to a single declared source: resolving a name
// the caller did not ask for is how an agent ends up writing to the wrong
// place and trusting the result.
func unknownSource(name string, declared []string) error {
	return output.NewError(
		"design_source_unknown",
		fmt.Sprintf("no design source named %q is declared by this project", name),
	).WithResource(name).WithNextAction(availableSources(declared))
}

// incompleteAddress refuses an address missing one of its two required
// halves. A design document is addressed by a source and a path together, and
// neither is inferred from the other.
func incompleteAddress(missing string, declared []string) error {
	return output.NewError(
		"design_address_incomplete",
		fmt.Sprintf("a design document address needs both a source and a path; %q is missing", missing),
	).WithNextAction(fmt.Sprintf(
		`address a document as --data '{"source":"<name>","path":"<path within the source>"}' (%s)`,
		availableSources(declared)))
}

// unreachableSource refuses a declared location that is not a directory on
// disk. It names the source, the path the declaration resolved to, and the
// base a relative location resolved from, because the commonest cause is a
// relative location written against the wrong base.
//
// The location is reported, never created. A design source points at a folder
// the team already has, so creating one would mask a typo as a new empty
// folder in an unexpected place.
func unreachableSource(name, declared, resolved, base string) error {
	er := output.NewError(
		"design_source_unreachable",
		fmt.Sprintf("design source %q is unreachable at %s", name, resolved),
	).WithResource(resolved)

	if isAbs(declared) {
		return er.WithNextAction(fmt.Sprintf(
			"create %s, or correct the location of the %q design source in config.yaml", resolved, name))
	}
	return er.WithNextAction(fmt.Sprintf(
		"create %s, or correct the location of the %q design source in config.yaml; %q is relative, and a relative design location resolves from %s, the folder holding config.yaml",
		resolved, name, declared, base))
}

// unsupportedProvider refuses a declared backend this build does not
// implement, by name rather than by ignoring the source. A source silently
// dropped at construction would read as a project that declared nothing.
func unsupportedProvider(name, provider string) error {
	return output.NewError(
		"design_provider_unsupported",
		fmt.Sprintf("design source %q declares provider %q, which this build does not implement", name, provider),
	).WithResource(name).WithNextAction(fmt.Sprintf(
		"set the %q design source's provider to %q, the only design storage backend this release ships", name, providerFile))
}

// readOnlySource refuses a write to a source whose provider cannot perform
// one. No provider shipping today is read-only, so this is the named outcome
// reserved for the first one that is, rather than an undefined result.
func readOnlySource(src *resolvedSource, path string) error {
	return output.NewError(
		"design_source_read_only",
		fmt.Sprintf("design source %q uses provider %q, which cannot write; %q was not written", src.name, src.provider, path),
	).WithResource(src.name).WithNextAction(
		"write the document to a design source whose provider supports writing, or edit it at its own origin; run 'design sources' to see each source's provider")
}

// notFound refuses a read of a document that is not in the source that was
// named. It is deliberately a different failure from an unknown source: one
// means "you asked for a source that does not exist", the other means "that
// source exists and does not hold this". It names the absolute location that
// was searched, so a broken reference is diagnosable from the error alone.
func notFound(src *resolvedSource, path string, cause error) error {
	if !isNotFound(cause) {
		return cause
	}
	return output.NewError(
		"design_not_found",
		fmt.Sprintf("design source %q holds no document at %q (searched %s)", src.name, path, joinPath(src.location, path)),
	).WithResource(joinPath(src.location, path)).WithNextAction(fmt.Sprintf(
		"run 'design list --source %s' to see what that source holds, or write the document with 'design write'", src.name))
}
