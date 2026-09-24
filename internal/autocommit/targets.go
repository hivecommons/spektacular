package autocommit

import (
	"os"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/repo"
)

// Target is one git work tree that one or more registered repos resolve to.
// Two repos registered inside the same work tree share a Target, so they
// produce one commit between them rather than one each.
type Target struct {
	Repos []string // registered repo names sharing this work tree, registry order
	Dir   string   // absolute work-tree top level
}

// Targets resolves the project's registered repos to the git work trees
// that hold their code, in registry order. Resolution never clones and
// never fetches: it reads the registry, asks each entry where its code is
// on disk, and asks git for the work tree containing it.
//
// Three kinds of entry are skipped silently, because none of them is
// something to commit and none is an error: a repo whose source is not on
// disk yet, a directory that has since gone missing, and a directory that
// is not inside a git work tree.
func Targets(cfg config.Config, projectRoot string, git Git) ([]Target, error) {
	// A nil GitRunner is safe here: LocalSource resolves from the registry,
	// repo.yaml and the filesystem, and never invokes git.
	set, err := repo.New(cfg, projectRoot, nil)
	if err != nil {
		return nil, err
	}

	var targets []Target
	byDir := map[string]int{} // work-tree top level -> index into targets

	for _, e := range set.Entries() {
		src, ok := set.LocalSource(e.Name)
		if !ok {
			continue
		}
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		top, ok, err := git.TopLevel(src)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if i, seen := byDir[top]; seen {
			targets[i].Repos = append(targets[i].Repos, e.Name)
			continue
		}
		byDir[top] = len(targets)
		targets = append(targets, Target{Repos: []string{e.Name}, Dir: top})
	}

	return targets, nil
}
