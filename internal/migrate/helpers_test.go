package migrate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testVersion pins the running Spektacular version so `written_by` is
// deterministic in the golden files.
const testVersion = "0.1.0"

// withSteps replaces the registered steps and current format for kind for the
// duration of the test, restoring the originals on cleanup.
func withSteps(t *testing.T, kind Kind, steps []Step, cur int) {
	t.Helper()
	oldSteps, hadSteps := registry[kind]
	oldCur, hadCur := current[kind]
	registry[kind] = steps
	current[kind] = cur
	t.Cleanup(func() {
		if hadSteps {
			registry[kind] = oldSteps
		} else {
			delete(registry, kind)
		}
		if hadCur {
			current[kind] = oldCur
		} else {
			delete(current, kind)
		}
	})
}

// newProject copies every file of testdata/<fixture> into a fresh
// <tmp>/.spektacular and returns the project root.
func newProject(t *testing.T, fixture string) string {
	t.Helper()
	root := t.TempDir()
	copyFixture(t, fixture, root)
	return root
}

func copyFixture(t *testing.T, fixture, root string) {
	t.Helper()
	cfgDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	entries, err := os.ReadDir(filepath.Join("testdata", fixture))
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", fixture, e.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(cfgDir, e.Name()), data, 0644))
	}
}

// writeProject creates <tmp>/.spektacular/config.yaml with the given content
// and returns the project root.
func writeProject(t *testing.T, configYAML string) string {
	t.Helper()
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(configYAML), 0644))
	return root
}

func cfgPath(root, name string) string {
	return filepath.Join(root, ".spektacular", name)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func golden(t *testing.T, fixture, name string) string {
	t.Helper()
	return string(readFile(t, filepath.Join("testdata", "golden", fixture, name+".golden")))
}

// snapshotDir maps every file under root (as a slash-separated relative path)
// to a sha256 of its content, so two snapshots compare both the file list and
// every file's bytes.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snap[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	}))
	return snap
}

// fakeInstaller is an Installer fake that records the agent it was asked for.
type fakeInstaller struct {
	calls []string
	err   error
}

func (f *fakeInstaller) install(agent string) error {
	f.calls = append(f.calls, agent)
	return f.err
}
