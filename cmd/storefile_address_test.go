package cmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// addressProject lays out a project (chdir'd into) whose configured command
// is `spek`, with counter IDs, docs-rooted spec/plan/changelog stores and one
// registered member repo named "member". It seeds, through the CLI, the
// spec, central changelog record and plan documents `plan` and `context` for
// 000001_feat, plus a member-routed changelog record 000002_other. A staged
// source file `src.md` sits in the project root so write commands can pass
// the relative `--from src.md`.
func addressProject(t *testing.T) (projectDir, memberDir string) {
	t.Helper()
	memberDir = footprintMemberRepo(t)
	projectDir = t.TempDir()
	t.Chdir(projectDir)
	writeSpecCommandConfig(t, projectDir,
		"command: spek\n"+
			"spec:\n  id_method: counter\n  config:\n    directory: ../docs/specs\n"+
			"plan:\n  config:\n    directory: ../docs/plans\n"+
			"changelog:\n  config:\n    directory: ../docs/changelog\n"+
			"repos:\n  - name: member\n    location: "+memberDir+"\n")

	seed := func(body string, args ...string) {
		t.Helper()
		src := filepath.Join(projectDir, "src.md")
		require.NoError(t, os.WriteFile(src, []byte(body), 0o644))
		addrOK(t, append(args, "--from", "src.md")...)
	}
	seed("spec body", "spec", "file", "write", "000001_feat")
	seed("changelog body", "changelog", "file", "write", "000001_feat")
	seed("member changelog body", "changelog", "file", "write", "000002_other", "--repo", "member")
	seed("plan body", "plan", "file", "write", "000001_feat", "plan")
	seed("context body", "plan", "file", "write", "000001_feat", "context")
	return projectDir, memberDir
}

// addrOK runs args and requires success, returning stdout.
func addrOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equal(t, 0, code, "%v\nstdout: %s\nstderr: %s", args, stdout, stderr)
	return stdout
}

// addrErr runs args, requires failure and returns the decoded envelope along
// with the raw stdout and stderr.
func addrErr(t *testing.T, args ...string) (output.ErrorResponse, string) {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equal(t, 1, code, "%v\nstdout: %s", args, stdout)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er), stdout)
	return er, stdout + stderr
}

// addrBody reads a document and returns its body without front matter.
func addrBody(t *testing.T, args ...string) string {
	t.Helper()
	_, body, err := metadata.Split([]byte(addrOK(t, args...)))
	require.NoError(t, err)
	return string(body)
}

// addrNames runs a list command and returns the listed names in order.
func addrNames(t *testing.T, args ...string) []string {
	t.Helper()
	var res struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(addrOK(t, args...)), &res))
	names := make([]string, 0, len(res.Files))
	for _, f := range res.Files {
		names = append(names, f.Name)
	}
	return names
}

// addrSnapshot captures every file under the given roots, path to content.
func addrSnapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	for _, root := range roots {
		require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			snap[p] = string(b)
			return err
		}))
	}
	return snap
}

// Every name a list prints reads back through the matching read verb, for
// specs, central and repo-routed changelog records, plan features and plan
// documents.
func TestAddressRoundTrip_ListedNamesReadBack(t *testing.T) {
	addressProject(t)

	cases := []struct {
		name   string
		list   []string
		want   []string
		read   func(name string) []string
		bodies map[string]string
	}{
		{
			name:   "spec",
			list:   []string{"spec", "file", "list"},
			want:   []string{"000001_feat"},
			read:   func(n string) []string { return []string{"spec", "file", "read", n} },
			bodies: map[string]string{"000001_feat": "spec body"},
		},
		{
			name:   "changelog central",
			list:   []string{"changelog", "file", "list"},
			want:   []string{"000001_feat"},
			read:   func(n string) []string { return []string{"changelog", "file", "read", n} },
			bodies: map[string]string{"000001_feat": "changelog body"},
		},
		{
			name:   "changelog repo",
			list:   []string{"changelog", "file", "list", "--repo", "member"},
			want:   []string{"000002_other"},
			read:   func(n string) []string { return []string{"changelog", "file", "read", n, "--repo", "member"} },
			bodies: map[string]string{"000002_other": "member changelog body"},
		},
		{
			name:   "plan documents",
			list:   []string{"plan", "file", "list", "000001_feat"},
			want:   []string{"context", "plan"},
			read:   func(n string) []string { return []string{"plan", "file", "read", "000001_feat", n} },
			bodies: map[string]string{"plan": "plan body", "context": "context body"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := addrNames(t, tc.list...)
			require.ElementsMatch(t, tc.want, names)
			for _, n := range names {
				require.Equal(t, tc.bodies[n], addrBody(t, tc.read(n)...))
			}
		})
	}

	t.Run("plan features", func(t *testing.T) {
		features := addrNames(t, "plan", "file", "list")
		require.Equal(t, []string{"000001_feat"}, features)
		require.ElementsMatch(t, []string{"context", "plan"}, addrNames(t, "plan", "file", "list", features[0]))
	})
}

// Write, set-document-status and delete given bare names act on the very
// document read returns.
func TestAddressMutations_ActOnTheDocumentReadReturns(t *testing.T) {
	cases := []struct {
		name       string
		addr       []string
		wantStatus map[string]any
	}{
		{"spec", []string{"spec", "file"}, map[string]any{"name": "000001_feat"}},
		{"changelog", []string{"changelog", "file"}, map[string]any{"name": "000001_feat"}},
		{"plan", []string{"plan", "file"}, map[string]any{"name": "000001_feat", "document": "context"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectDir, _ := addressProject(t)
			target := []string{"000001_feat"}
			if tc.name == "plan" {
				target = append(target, "context")
			}
			cmd := func(verb string, extra ...string) []string {
				out := append(append(append([]string{}, tc.addr...), verb), target...)
				return append(out, extra...)
			}

			require.NoError(t, os.WriteFile(filepath.Join(projectDir, "src.md"), []byte("rewritten"), 0o644))
			addrOK(t, cmd("write", "--from", "src.md")...)
			require.Equal(t, "rewritten", addrBody(t, cmd("read")...))

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(addrOK(t, cmd("set-document-status", "--document-status", "stale")...)), &got))
			delete(got, "path") // path values are covered elsewhere
			// stale is a closing status, so closed_date is stamped today.
			want := map[string]any{"error": false, "document_status": "stale", "closed_date": today().Format("2006-01-02")}
			for k, v := range tc.wantStatus {
				want[k] = v
			}
			require.Equal(t, want, got)
			meta, body, err := metadata.Split([]byte(addrOK(t, cmd("read")...)))
			require.NoError(t, err)
			require.Equal(t, metadata.StatusStale, meta.DocumentStatus)
			require.Equal(t, "rewritten", string(body))

			addrOK(t, cmd("delete")...)
			er, _ := addrErr(t, cmd("read")...)
			require.Equal(t, "not_found", er.Code)
		})
	}
}

// A name carrying an extension or a joined path is refused with the command
// correctly spelled, caller's flags included, and the store is left alone.
func TestAddressExtensionRefusal_RestatesCommandAndLeavesStoreUntouched(t *testing.T) {
	projectDir, memberDir := addressProject(t)
	roots := []string{filepath.Join(projectDir, "docs"), filepath.Join(memberDir, "changelog")}

	cases := []struct {
		args []string
		next string
	}{
		{[]string{"spec", "file", "write", "000001_feat.md", "--from", "src.md"}, "run `spek spec file write 000001_feat --from src.md`"},
		{[]string{"spec", "file", "read", "000001_feat.md"}, "run `spek spec file read 000001_feat`"},
		{[]string{"spec", "file", "delete", "000001_feat.md"}, "run `spek spec file delete 000001_feat`"},
		{[]string{"spec", "file", "set-document-status", "000001_feat.md", "--document-status", "final"}, "run `spek spec file set-document-status 000001_feat --document-status final`"},

		{[]string{"changelog", "file", "write", "000001_feat.md", "--from", "src.md"}, "run `spek changelog file write 000001_feat --from src.md`"},
		{[]string{"changelog", "file", "read", "000001_feat.md"}, "run `spek changelog file read 000001_feat`"},
		{[]string{"changelog", "file", "read", "000002_other.md", "--repo", "member"}, "run `spek changelog file read 000002_other --repo member`"},
		{[]string{"changelog", "file", "delete", "000001_feat.md"}, "run `spek changelog file delete 000001_feat`"},
		{[]string{"changelog", "file", "set-document-status", "000001_feat.md", "--document-status", "final"}, "run `spek changelog file set-document-status 000001_feat --document-status final`"},

		// A plan verb given a bare feature with an extension cannot recover
		// the document, so the correction leaves a placeholder for it.
		{[]string{"plan", "file", "write", "000001_feat.md", "--from", "src.md"}, "run `spek plan file write 000001_feat <document> --from src.md`"},
		{[]string{"plan", "file", "read", "000001_feat.md"}, "run `spek plan file read 000001_feat <document>`"},
		{[]string{"plan", "file", "delete", "000001_feat.md"}, "run `spek plan file delete 000001_feat <document>`"},
		{[]string{"plan", "file", "set-document-status", "000001_feat.md", "--document-status", "final"}, "run `spek plan file set-document-status 000001_feat <document> --document-status final`"},
		{[]string{"plan", "file", "list", "000001_feat.md"}, "run `spek plan file list 000001_feat`"},

		// A joined feature/document path splits into the two arguments.
		{[]string{"plan", "file", "write", "000001_feat/plan.md", "--from", "src.md", "--document-status", "final"}, "run `spek plan file write 000001_feat plan --document-status final --from src.md`"},
		{[]string{"plan", "file", "read", "000001_feat/plan.md"}, "run `spek plan file read 000001_feat plan`"},
		{[]string{"plan", "file", "delete", "000001_feat/plan.md"}, "run `spek plan file delete 000001_feat plan`"},
		{[]string{"plan", "file", "set-document-status", "000001_feat/plan.md", "--document-status", "final"}, "run `spek plan file set-document-status 000001_feat plan --document-status final`"},
		{[]string{"plan", "file", "read", "000001_feat", "plan.md"}, "run `spek plan file read 000001_feat plan`"},
	}
	for _, tc := range cases {
		t.Run(filepath.Join(tc.args[:4]...), func(t *testing.T) {
			before := addrSnapshot(t, roots...)
			er, _ := addrErr(t, tc.args...)
			require.Equal(t, "unexpected_extension", er.Code)
			require.Equal(t, tc.next, er.NextAction)
			require.Equal(t, before, addrSnapshot(t, roots...))
		})
	}
}

// A plan document verb given only a feature is refused with document_required,
// pointing at the feature's document listing, without leaking where the
// project lives on disk.
func TestAddressDocumentRequired_PlanVerbGivenOnlyFeature(t *testing.T) {
	projectDir, _ := addressProject(t)

	cases := []struct {
		args []string
		next string
	}{
		{[]string{"write", "000001_feat", "--from", "src.md"}, "run `spek plan file list 000001_feat` to see its documents, then name one, e.g. `spek plan file write 000001_feat plan --from src.md`"},
		{[]string{"read", "000001_feat"}, "run `spek plan file list 000001_feat` to see its documents, then name one, e.g. `spek plan file read 000001_feat plan`"},
		{[]string{"delete", "000001_feat"}, "run `spek plan file list 000001_feat` to see its documents, then name one, e.g. `spek plan file delete 000001_feat plan`"},
		{[]string{"set-document-status", "000001_feat", "--document-status", "final"}, "run `spek plan file list 000001_feat` to see its documents, then name one, e.g. `spek plan file set-document-status 000001_feat plan --document-status final`"},
	}
	for _, tc := range cases {
		t.Run(tc.args[0], func(t *testing.T) {
			er, raw := addrErr(t, append([]string{"plan", "file"}, tc.args...)...)
			require.Equal(t, "document_required", er.Code)
			require.Equal(t, tc.next, er.NextAction)
			require.NotContains(t, raw, projectDir)
		})
	}
}

// A correctly spelled address naming nothing stored points at the list
// command that shows what does exist.
func TestAddressNotFound_PointsAtListCommand(t *testing.T) {
	addressProject(t)

	cases := []struct {
		args []string
		next string
	}{
		{[]string{"spec", "file", "read", "000009_none"}, "run `spek spec file list` to see available names"},
		{[]string{"plan", "file", "read", "000001_feat", "research"}, "run `spek plan file list 000001_feat` to see available names"},
		{[]string{"plan", "file", "list", "000009_none"}, "run `spek plan file list` to see available names"},
		{[]string{"changelog", "file", "read", "000009_none", "--repo", "member"}, "run `spek changelog file list --repo member` to see available names"},
	}
	for _, tc := range cases {
		t.Run(filepath.Join(tc.args[:4]...), func(t *testing.T) {
			er, _ := addrErr(t, tc.args...)
			require.Equal(t, "not_found", er.Code)
			require.Equal(t, tc.next, er.NextAction)
		})
	}
}
