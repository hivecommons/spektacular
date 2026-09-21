package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A settings file's content apart from its format version, for each kind.
// Both bodies are valid at the current format, so a refusal can only come
// from the version line prefixed to them.
const (
	projectBody = "name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: ..\n"
	repoBody = "description: keep me\n" +
		"knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: kb\n"
)

// loader is one settings-file entry point under test.
type loader struct {
	kind string
	want int // the current format of this kind, hand-maintained
	file string
	body string
	load func(path string) error
}

func formatLoaders() map[string]loader {
	return map[string]loader{
		"ParseYAMLFile": {kind: "project", want: 3, file: "config.yaml", body: projectBody, load: func(p string) error {
			_, err := ParseYAMLFile(p)
			return err
		}},
		"FromYAMLFile": {kind: "project", want: 3, file: "config.yaml", body: projectBody, load: func(p string) error {
			_, err := FromYAMLFile(p)
			return err
		}},
		"RepoConfigFromYAMLFile": {kind: "repo", want: 2, file: RepoConfigFileName, body: repoBody, load: func(p string) error {
			_, err := RepoConfigFromYAMLFile(p)
			return err
		}},
	}
}

// Phase 2.1: a settings file behind the current format (no schema key, so
// format 1) is refused with a typed FormatError that reports it as older, and
// the file is left byte-identical.
func TestLoaders_OutdatedFileIsRefusedAsFormatError(t *testing.T) {
	for name, l := range formatLoaders() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), l.file)
			require.NoError(t, os.WriteFile(path, []byte(l.body), 0644))

			err := l.load(path)
			require.Error(t, err)
			fe, ok := IsFormatError(err)
			require.True(t, ok, "expected a *FormatError, got %T: %v", err, err)
			require.Equal(t, &FormatError{Path: path, Kind: l.kind, Found: 1, Want: l.want}, fe)
			require.False(t, fe.Newer())
			require.Equal(t, l.kind+" settings file "+path+fmt.Sprintf(" is format 1; this Spektacular needs format %d", l.want), err.Error())

			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, l.body, string(after), "a refused settings file must not be rewritten")
		})
	}
}

// Phase 2.1: a settings file written in a format newer than this build
// supports is refused with a FormatError that reports it as newer, and the
// file is left byte-identical.
func TestLoaders_NewerFileIsRefusedAsFormatError(t *testing.T) {
	for name, l := range formatLoaders() {
		t.Run(name, func(t *testing.T) {
			body := "schema: 99\n" + l.body
			path := filepath.Join(t.TempDir(), l.file)
			require.NoError(t, os.WriteFile(path, []byte(body), 0644))

			err := l.load(path)
			require.Error(t, err)
			fe, ok := IsFormatError(err)
			require.True(t, ok, "expected a *FormatError, got %T: %v", err, err)
			require.Equal(t, &FormatError{Path: path, Kind: l.kind, Found: 99, Want: l.want}, fe)
			require.True(t, fe.Newer())
			require.Equal(t, l.kind+" settings file "+path+fmt.Sprintf(" is format 99, newer than this Spektacular supports (format %d)", l.want), err.Error())

			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, body, string(after), "a refused settings file must not be rewritten")
		})
	}
}

// Phase 2.1: the same body at the current format loads through every entry
// point, so the refusals above are caused by the version line alone.
func TestLoaders_CurrentFormatLoads(t *testing.T) {
	for name, l := range formatLoaders() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), l.file)
			require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("schema: %d\n", l.want)+l.body), 0644))
			require.NoError(t, l.load(path))
		})
	}
}

// IsFormatError recognises a wrapped FormatError and rejects anything else.
func TestIsFormatError(t *testing.T) {
	fe := &FormatError{Path: "/p", Kind: "repo", Found: 1, Want: 2}
	got, ok := IsFormatError(fmt.Errorf("loading: %w", fe))
	require.True(t, ok)
	require.Same(t, fe, got)

	_, ok = IsFormatError(os.ErrNotExist)
	require.False(t, ok)
}
