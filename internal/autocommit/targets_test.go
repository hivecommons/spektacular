package autocommit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// commitCall records one CommitAll invocation on the fake.
type commitCall struct {
	Dir     string
	Message string
}

// fakeGit is a recording, hand-canned Git. tops maps a queried directory to
// the work-tree top level it belongs to; a directory absent from tops is
// reported as "not a git work tree" (ok=false, nil error), which is exactly
// how the real execGit reports a non-repo. dirty maps a work-tree top to
// whether it has uncommitted changes.
type fakeGit struct {
	tops      map[string]string
	dirty     map[string]bool
	topErr    map[string]error
	dirtyErr  map[string]error
	commitErr map[string]error

	dirtyCalls  []string
	commitCalls []commitCall
}

func (f *fakeGit) TopLevel(dir string) (string, bool, error) {
	if err := f.topErr[dir]; err != nil {
		return "", false, err
	}
	top, ok := f.tops[dir]
	if !ok {
		return "", false, nil
	}
	return top, true, nil
}

func (f *fakeGit) Dirty(top string) (bool, error) {
	f.dirtyCalls = append(f.dirtyCalls, top)
	if err := f.dirtyErr[top]; err != nil {
		return false, err
	}
	return f.dirty[top], nil
}

func (f *fakeGit) CommitAll(top, message string) error {
	f.commitCalls = append(f.commitCalls, commitCall{Dir: top, Message: message})
	return f.commitErr[top]
}

// newLocation creates an existing, registerable repo location: a directory
// with no repo.yaml, which LocalSource resolves to the directory itself.
func newLocation(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

// newTargetsConfig builds a project config registering the given entries in
// order.
func newTargetsConfig(entries ...config.RepoEntry) config.Config {
	cfg := config.NewDefault()
	cfg.Repos = entries
	return cfg
}

// Criterion 2: a registered repo whose directory is not inside a git work
// tree is skipped, without an error and without displacing the repos either
// side of it in the registry.
func TestTargets_SkipsRepoThatIsNotAGitRepository(t *testing.T) {
	parent := t.TempDir()
	gitRepo := newLocation(t, parent, "with-git")
	plain := newLocation(t, parent, "no-git")

	git := &fakeGit{tops: map[string]string{gitRepo: gitRepo}}

	targets, err := Targets(newTargetsConfig(
		config.RepoEntry{Name: "plain", Location: plain},
		config.RepoEntry{Name: "tracked", Location: gitRepo},
	), t.TempDir(), git)

	require.NoError(t, err)
	require.Equal(t, []Target{{Repos: []string{"tracked"}, Dir: gitRepo}}, targets)
}

// Criterion 2: a registered repo that has not been cloned — its location is
// not on disk at all — is skipped without an error, and git is never even
// asked about it.
func TestTargets_SkipsRepoNotOnDisk(t *testing.T) {
	parent := t.TempDir()
	missing := filepath.Join(parent, "never-cloned")
	present := newLocation(t, parent, "cloned")

	git := &fakeGit{tops: map[string]string{
		missing: missing, // canned, but must never be reached
		present: present,
	}}

	targets, err := Targets(newTargetsConfig(
		config.RepoEntry{Name: "absent", Location: missing},
		config.RepoEntry{Name: "present", Location: present},
	), t.TempDir(), git)

	require.NoError(t, err)
	require.Equal(t, []Target{{Repos: []string{"present"}, Dir: present}}, targets)
}

// Criterion 3: two registered repos that resolve to the same git work tree
// collapse into a single Target carrying both names in registry order, so
// the work tree is committed once rather than twice. The surrounding entry
// shows registry order is preserved across the merge.
func TestTargets_MergesReposSharingOneWorkTree(t *testing.T) {
	parent := t.TempDir()
	mono := newLocation(t, parent, "mono")
	monoSub := newLocation(t, mono, "packages/api")
	other := newLocation(t, parent, "other")

	git := &fakeGit{tops: map[string]string{
		mono:    mono,
		monoSub: mono,
		other:   other,
	}}

	targets, err := Targets(newTargetsConfig(
		config.RepoEntry{Name: "mono-root", Location: mono},
		config.RepoEntry{Name: "sibling", Location: other},
		config.RepoEntry{Name: "mono-api", Location: monoSub},
	), t.TempDir(), git)

	require.NoError(t, err)
	require.Equal(t, []Target{
		{Repos: []string{"mono-root", "mono-api"}, Dir: mono},
		{Repos: []string{"sibling"}, Dir: other},
	}, targets)
}
