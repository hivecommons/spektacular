package cmd

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

func TestStatusRunSource_DirtyOnlyChecksTouchedRepos(t *testing.T) {
	f := worktreeProject(t, true)
	cfg, err := loadConfig()
	require.NoError(t, err)
	src := statusRunSource(cfg, f.proj, store.NewSourceStore(f.proj, "project"))

	// The registered docs repo has untracked work, but no task touches it.
	wtWriteFile(t, f.site, "untracked.txt", "user work\n")
	require.Empty(t, src.Dirty([]string{"testproj"}))
	require.Equal(t, []string{"docs"}, src.Dirty([]string{"testproj", "docs"}))
	require.Empty(t, src.Dirty(nil), "no plans means no touched repos")

	// Tracked modifications are included as well as untracked files.
	wtWriteFile(t, f.proj, "main.txt", "user change\n")
	require.Equal(t, []string{"testproj", "docs"}, src.Dirty([]string{"testproj", "docs"}))
	require.Equal(t, []string{"testproj"}, src.Dirty([]string{"testproj", "testproj"}))

	// Repos sharing a checkout still report only the touched names.
	cfg.Repos = append(cfg.Repos, config.RepoEntry{Name: "alias", Location: "."})
	src = statusRunSource(cfg, f.proj, store.NewSourceStore(f.proj, "project"))
	require.Equal(t, []string{"alias"}, src.Dirty([]string{"alias"}))
	require.Equal(t, []string{"testproj", "alias"}, src.Dirty([]string{"alias", "testproj"}))

	// Reporting never cleans or commits the user's changes.
	require.Contains(t, gittest.RunGit(t, f.proj, "status", "--porcelain"), "main.txt")
	require.Contains(t, gittest.RunGit(t, f.site, "status", "--porcelain"), "untracked.txt")
}
