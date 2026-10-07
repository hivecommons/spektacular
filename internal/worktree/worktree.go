// Package worktree gives each spec of an epic its own git worktrees while it
// is implemented, and merges them back when it is done.
//
// A spec gets one worktree in every repo its plan touches — the project's own
// repo, plus each registered repo a task names — all on branch spek/<spec>,
// under <project>/.spektacular/worktrees/<spec>/<repo>. The worktrees hold
// only code: Spektacular always runs from the main project, and a record
// beside the worktrees (record.json) maps each repo to its code root inside
// them, for the implement workflow to name. Merging is all or nothing across
// repos: a dry run in every repo first, and nothing merged when any would
// conflict or when the branch changes Spektacular's own files.
//
// Locations and branch names are fixed conventions, so `status` can find a
// spec's worktrees again from git alone.
package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/gitexec"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
)

// Dir is the folder, under the project's .spektacular directory, holding
// every spec's worktrees.
const Dir = "worktrees"

// BranchPrefix starts every spec branch.
const BranchPrefix = "spek/"

// Branch is the branch a spec's worktrees are on, in every repo.
func Branch(spec string) string { return BranchPrefix + spec }

// RepoWorktree is one repo's worktree for a spec.
type RepoWorktree struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	// Top is the main checkout the worktree belongs to: where the branch is
	// merged back.
	Top string `json:"-"`
}

// SpecWorktrees is every worktree of one spec.
type SpecWorktrees struct {
	Spec string
	// Project is the project root inside the project repo's worktree: where
	// the project's own code for the spec is changed.
	Project string
	// Repos lists every touched repo's worktree, the project repo first.
	Repos []RepoWorktree
}

// RecordFile is the name of a spec's worktree record, kept in the main
// project beside the spec's worktrees.
const RecordFile = "record.json"

// Record is a spec's worktree record: every touched repo's code root inside
// the spec's worktrees. It lives in the main project, under the folder
// holding the spec's worktrees, which is already kept out of git; nothing
// about it is ever written inside a worktree.
type Record struct {
	Spec string `json:"spec"`
	// Repos maps each registered repo name to the absolute directory holding
	// its code inside the spec's worktrees.
	Repos map[string]string `json:"repos"`
}

// MergeResult is the outcome of merging a spec back.
type MergeResult struct {
	Spec   string
	Merged bool
	// Conflicts maps a repo to the paths that would conflict; set only when
	// nothing was merged.
	Conflicts map[string][]string
	// Removed is true once every worktree and branch of the spec is gone.
	Removed bool
}

// Runner runs git in a directory, returning trimmed stdout and the exit
// code; an error means git could not run or failed outright (exit above 1).
type Runner interface {
	Run(dir string, args ...string) (out string, code int, err error)
}

type execRunner struct{}

func (execRunner) Run(dir string, args ...string) (string, int, error) {
	return gitexec.RunCode(dir, args...)
}

// NewRunner returns the Runner backed by the user's git binary.
func NewRunner() Runner { return execRunner{} }

// Manager creates, finds and merges a project's spec worktrees.
type Manager struct {
	ProjectRoot string
	Config      config.Config
	Repos       *repo.Set
	Git         Runner
	// Setup runs each touched repo's worktree setup command in a newly
	// created worktree; nil means the system shell.
	Setup SetupRunner
}

// setupRunner is the manager's setup runner, defaulting to the system shell.
func (m Manager) setupRunner() SetupRunner {
	if m.Setup == nil {
		return NewSetupRunner()
	}
	return m.Setup
}

// unavailableNext is the remediation for a repo that cannot hold a worktree.
const unavailableNext = "commit the repo's current state (run `git init` and make an initial commit if it has none), then retry; or set implement.worktrees: false in .spektacular/config.yaml to build runs you start yourself in the main checkouts"

// unavailable builds the worktree_unavailable refusal for the named repo
// ("" for the project itself), which cannot hold a worktree for the reason
// given. Nothing falls back to the main checkout silently.
func unavailable(name, reason string) error {
	who, resource := fmt.Sprintf("repo %q", name), name
	if name == "" {
		who, resource = "the project", "project"
	}
	return output.NewError("worktree_unavailable",
		fmt.Sprintf("%s %s, so the spec cannot be built in its own worktree", who, reason)).
		WithResource(resource).
		WithNextAction(unavailableNext)
}

// failed builds the worktree_failed refusal.
func failed(message, next string) error {
	return output.NewError("worktree_failed", message).WithNextAction(next)
}

// git runs a command that must succeed (exit 0).
func (m Manager) git(dir string, args ...string) (string, error) {
	out, code, err := m.Git.Run(dir, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return out, fmt.Errorf("git %s exited %d", args[0], code)
	}
	return out, nil
}

// TouchedRepos lists the registered repos a spec's plan names in its tasks'
// `**Repo:**` lines, in first-mention order. The project repo is added by
// Ensure, so it need not appear.
func TouchedRepos(cfg config.Config, st store.Reader, spec string) ([]string, error) {
	path := artifact.Address{Kind: artifact.KindPlan, Feature: spec, Document: "plan"}.StorePath(cfg.Plan.Config.Directory)
	raw, err := st.Read(path)
	if err != nil {
		return nil, output.NewError("plan_not_found", fmt.Sprintf("spec %q has no plan, so the repos it touches are not known", spec)).
			WithResource(spec).
			WithNextAction(fmt.Sprintf("plan the spec first: %s plan new --data '{\"name\":%q}'", cfg.Command, spec))
	}
	_, body, err := metadata.Split(raw)
	if err != nil {
		body = raw
	}
	registered := map[string]bool{}
	for _, e := range cfg.Repos {
		registered[e.Name] = true
	}
	var names []string
	seen := map[string]bool{}
	for _, t := range plantask.Parse(body).Tasks {
		if t.Repo != "" && registered[t.Repo] && !seen[t.Repo] {
			seen[t.Repo] = true
			names = append(names, t.Repo)
		}
	}
	return names, nil
}

// checkout is one main git checkout touched by a spec, with every registered
// repo that lives in it.
type checkout struct {
	top   string
	repos []string // registered repos in this work tree; the first names the worktree
}

// projectTop returns the project's git top level and the project root's path
// within it.
func (m Manager) projectTop() (top, rel string, err error) {
	root, err := filepath.EvalSymlinks(m.ProjectRoot)
	if err != nil {
		return "", "", err
	}
	top, err = m.git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", unavailable("", fmt.Sprintf("at %s is not in a git repository", root))
	}
	rel, err = filepath.Rel(top, root)
	if err != nil {
		return "", "", err
	}
	return top, rel, nil
}

// checkouts groups the project and the touched repos by git top level, the
// project's own checkout first. Repos sharing a work tree share one worktree.
func (m Manager) checkouts(touched []string) ([]checkout, error) {
	projTop, _, err := m.projectTop()
	if err != nil {
		return nil, err
	}
	var out []checkout
	index := map[string]int{}
	add := func(top, name string) {
		i, ok := index[top]
		if !ok {
			index[top] = len(out)
			out = append(out, checkout{top: top})
			i = len(out) - 1
		}
		if name != "" {
			for _, n := range out[i].repos {
				if n == name {
					return
				}
			}
			out[i].repos = append(out[i].repos, name)
		}
	}
	add(projTop, "")

	// Every registered repo in the project's own work tree belongs to the
	// project checkout, touched or not, so it resolves into the worktree too.
	for _, e := range m.Config.Repos {
		if top, ok := m.repoTop(e.Name); ok && top == projTop {
			add(projTop, e.Name)
		}
	}
	for _, name := range touched {
		src, ok := m.Repos.LocalSource(name)
		if !ok {
			return nil, failed(fmt.Sprintf("repo %q is touched by the plan but its code is not on disk, so it cannot get a worktree", name),
				fmt.Sprintf("make the repo available with `%s repo add`, or correct its registration, then retry", m.Config.Command))
		}
		top, err := m.git(src, "rev-parse", "--show-toplevel")
		if err != nil {
			return nil, unavailable(name, fmt.Sprintf("at %s is not in a git repository", src))
		}
		add(top, name)
	}
	return out, nil
}

// repoTop is a registered repo's git top level, when it is on disk and in
// git.
func (m Manager) repoTop(name string) (string, bool) {
	src, ok := m.Repos.LocalSource(name)
	if !ok {
		return "", false
	}
	top, err := m.git(src, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}
	return top, true
}

// worktreeRoot is where a spec's worktrees live.
func (m Manager) worktreeRoot(spec string) string {
	return filepath.Join(m.ProjectRoot, ".spektacular", Dir, spec)
}

// recordPath is where spec's worktree record lives under projectRoot.
func recordPath(projectRoot, spec string) string {
	return filepath.Join(projectRoot, ".spektacular", Dir, spec, RecordFile)
}

// ReadRecord loads spec's worktree record from the main project at
// projectRoot, without running git. ok is false when the spec has no
// record. Only absolute code roots are kept.
func ReadRecord(projectRoot, spec string) (Record, bool, error) {
	raw, err := os.ReadFile(recordPath(projectRoot, spec))
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		return Record{}, false, fmt.Errorf("reading the worktree record for %s: %w", spec, err)
	}
	roots := map[string]string{}
	for name, root := range r.Repos {
		if filepath.IsAbs(root) {
			roots[name] = filepath.Clean(root)
		}
	}
	r.Repos = roots
	return r, true, nil
}

// Record loads spec's worktree record; see ReadRecord.
func (m Manager) Record(spec string) (Record, bool, error) {
	return ReadRecord(m.ProjectRoot, spec)
}

// Ensure gives spec a worktree in the project's repo and in every repo of
// touched, returning them; the bool is true when any was created. Existing
// worktrees are reused, so a resumed run finds its own. It also keeps the
// worktrees out of the project's git, and writes the spec's worktree record
// in the main project. Nothing is ever written inside a worktree.
func (m Manager) Ensure(spec string, touched []string) (SpecWorktrees, bool, error) {
	cos, err := m.checkouts(touched)
	if err != nil {
		return SpecWorktrees{}, false, err
	}
	_, projRel, err := m.projectTop()
	if err != nil {
		return SpecWorktrees{}, false, err
	}
	if err := m.excludeFromProject(); err != nil {
		return SpecWorktrees{}, false, err
	}

	root, err := filepath.EvalSymlinks(m.ProjectRoot)
	if err != nil {
		return SpecWorktrees{}, false, err
	}
	base := filepath.Join(root, ".spektacular", Dir, spec)
	result := SpecWorktrees{Spec: spec}
	created := false
	record := Record{Spec: spec, Repos: map[string]string{}}
	isTouched := map[string]bool{}
	for _, name := range touched {
		isTouched[name] = true
	}

	// Every checkout must have a commit to branch from before any worktree
	// is made, so a refusal leaves nothing half created.
	for _, co := range cos {
		if _, code, err := m.Git.Run(co.top, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
			return SpecWorktrees{}, false, err
		} else if code != 0 {
			name := ""
			if len(co.repos) > 0 {
				name = co.repos[0]
			}
			return SpecWorktrees{}, false, unavailable(name, fmt.Sprintf("at %s has no commits yet", co.top))
		}
	}

	for i, co := range cos {
		dirName := "project"
		if len(co.repos) > 0 {
			dirName = co.repos[0]
		}
		path := filepath.Join(base, dirName)
		made, branchMade, err := m.ensureOne(co.top, path, spec)
		if err != nil {
			return SpecWorktrees{}, false, err
		}
		// A newly created worktree holds only tracked files, so each touched
		// repo in it is prepared with its own setup command before it is
		// handed out. An existing worktree was prepared when it was made.
		if made {
			if err := m.setUp(co, path, spec, branchMade, isTouched); err != nil {
				return SpecWorktrees{}, false, err
			}
		}
		created = created || made
		result.Repos = append(result.Repos, RepoWorktree{Repo: dirName, Path: path, Branch: Branch(spec), Top: co.top})
		if i == 0 {
			result.Project = filepath.Join(path, projRel)
		}
		// Each repo's code, relative to its own checkout, re-rooted in the
		// worktree.
		for _, name := range co.repos {
			if root, ok := m.codeRootIn(co.top, path, name); ok {
				record.Repos[name] = root
			}
		}
	}

	if err := writeRecord(filepath.Join(base, RecordFile), record); err != nil {
		return SpecWorktrees{}, false, err
	}

	return result, created, nil
}

// codeRootIn re-roots repo name's code, which lives in the main checkout
// top, into that checkout's worktree at path.
func (m Manager) codeRootIn(top, path, name string) (string, bool) {
	src, ok := m.Repos.LocalSource(name)
	if !ok {
		return "", false
	}
	src, err := filepath.EvalSymlinks(src)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(top, src)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.Join(path, rel), true
}

// writeRecord writes a spec's worktree record to path.
func writeRecord(path string, r Record) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// setUp runs the declared setup command of every touched repo in checkout
// co's newly created worktree at path, in the repo's code root there. The
// command is read from the repo's registration in the main project, never
// from the copy inside the worktree. When one fails, the worktree is removed,
// along with its branch when this run created it, so a retry starts clean.
func (m Manager) setUp(co checkout, path, spec string, branchMade bool, isTouched map[string]bool) error {
	for _, name := range co.repos {
		if !isTouched[name] {
			continue
		}
		meta, err := m.Repos.Footprint(name)
		if err != nil {
			return err
		}
		command := strings.TrimSpace(meta.WorktreeSetup)
		if command == "" {
			continue
		}
		dir, ok := m.codeRootIn(co.top, path, name)
		if !ok {
			continue
		}
		if _, err := m.setupRunner().Run(dir, command); err != nil {
			m.discard(co.top, path, spec, branchMade)
			return output.NewError("worktree_setup_failed",
				fmt.Sprintf("the worktree setup command for %s (%q) failed in %s: %v", name, command, dir, err)).
				WithResource(name).
				WithNextAction("fix the command in that repo's repo.yaml (worktree_setup), or the repo itself, then run the same command again")
		}
	}
	return nil
}

// discard removes a worktree that could not be prepared, and its branch when
// this run created it. It is best effort: the setup failure is what gets
// reported.
func (m Manager) discard(top, path, spec string, branchMade bool) {
	_, _, _ = m.Git.Run(top, "worktree", "remove", "--force", path)
	if branchMade {
		_, _, _ = m.Git.Run(top, "branch", "-D", Branch(spec))
	}
}

// ensureOne makes sure top has a worktree for spec at path, on its branch.
// made is true when the worktree was created now, and branchMade when its
// branch was too.
func (m Manager) ensureOne(top, path, spec string) (made, branchMade bool, err error) {
	existing, err := m.worktrees(top)
	if err != nil {
		return false, false, err
	}
	for _, w := range existing {
		if w.path == path {
			return false, false, nil
		}
	}
	branch := Branch(spec)
	_, code, err := m.Git.Run(top, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return false, false, err
	}
	args := []string{"worktree", "add", "-b", branch, path, "HEAD"}
	if code == 0 {
		args = []string{"worktree", "add", path, branch}
	}
	if _, err := m.git(top, args...); err != nil {
		return false, false, failed(fmt.Sprintf("could not create the worktree for %s in %s: %v", spec, top, err),
			"fix the cause git reported, then retry")
	}
	return true, code != 0, nil
}

// excludeFromProject lists the worktree folder in the project repository's
// info/exclude, so the nested worktrees, and the spec records beside them,
// are never swept into the main copy's commits. It is idempotent, and works for projects whose
// .gitignore predates worktrees without rewriting any tracked file.
func (m Manager) excludeFromProject() error {
	top, rel, err := m.projectTop()
	if err != nil {
		return err
	}
	common, err := m.git(top, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(top, common)
	}
	prefix := "/"
	if rel != "." {
		prefix = "/" + filepath.ToSlash(rel) + "/"
	}
	want := []string{
		prefix + ".spektacular/" + Dir + "/",
	}
	path := filepath.Join(common, "info", "exclude")
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := map[string]bool{}
	for _, l := range strings.Split(string(current), "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, w := range want {
		if !lines[w] {
			add = append(add, w)
		}
	}
	if len(add) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	text := string(current)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "# Spektacular epic worktrees\n" + strings.Join(add, "\n") + "\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

type listed struct {
	path   string
	branch string
}

// worktrees parses `git worktree list --porcelain` in top.
func (m Manager) worktrees(top string) ([]listed, error) {
	out, err := m.git(top, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var all []listed
	var cur listed
	flush := func() {
		if cur.path != "" {
			all = append(all, cur)
		}
		cur = listed{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			cur.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return all, nil
}

// List finds every spec worktree under the project, from the project's
// checkout and every registered repo's, grouped by spec and sorted by spec
// name. Only worktrees on a spek/ branch under the project's worktree folder
// count.
func (m Manager) List() ([]SpecWorktrees, error) {
	projTop, projRel, err := m.projectTop()
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(m.ProjectRoot)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, ".spektacular", Dir) + string(filepath.Separator)

	tops := []string{projTop}
	seen := map[string]bool{projTop: true}
	for _, e := range m.Config.Repos {
		if top, ok := m.repoTop(e.Name); ok && !seen[top] {
			seen[top] = true
			tops = append(tops, top)
		}
	}

	bySpec := map[string]*SpecWorktrees{}
	for _, top := range tops {
		ws, err := m.worktrees(top)
		if err != nil {
			return nil, err
		}
		for _, w := range ws {
			if !strings.HasPrefix(w.branch, BranchPrefix) || !strings.HasPrefix(w.path, base) {
				continue
			}
			spec := strings.TrimPrefix(w.branch, BranchPrefix)
			sw, ok := bySpec[spec]
			if !ok {
				sw = &SpecWorktrees{Spec: spec}
				bySpec[spec] = sw
			}
			rw := RepoWorktree{Repo: filepath.Base(w.path), Path: w.path, Branch: w.branch, Top: top}
			if top == projTop {
				sw.Project = filepath.Join(w.path, projRel)
				sw.Repos = append([]RepoWorktree{rw}, sw.Repos...)
			} else {
				sw.Repos = append(sw.Repos, rw)
			}
		}
	}
	specs := make([]string, 0, len(bySpec))
	for s := range bySpec {
		specs = append(specs, s)
	}
	sort.Strings(specs)
	out := make([]SpecWorktrees, 0, len(specs))
	for _, s := range specs {
		out = append(out, *bySpec[s])
	}
	return out, nil
}

// Find returns spec's worktrees; ok is false when it has none.
func (m Manager) Find(spec string) (SpecWorktrees, bool, error) {
	all, err := m.List()
	if err != nil {
		return SpecWorktrees{}, false, err
	}
	for _, sw := range all {
		if sw.Spec == spec {
			return sw, true, nil
		}
	}
	return SpecWorktrees{}, false, nil
}

// Merge merges spec's branch into every touched repo's current branch, as one
// unit. It first checks every repo: none may have a merge in progress, a
// spec worktree with uncommitted work, or uncommitted changes in its main
// copy that the merge would touch; the branch may change nothing under a
// .spektacular directory in any repo; and a dry run must merge cleanly. When
// any repo would conflict, nothing is merged anywhere and the conflicts are
// returned per repo. Otherwise each repo is merged with --no-ff, and every
// worktree and branch of the spec is removed.
func (m Manager) Merge(spec string) (MergeResult, error) {
	sw, ok, err := m.Find(spec)
	if err != nil {
		return MergeResult{}, err
	}
	if !ok {
		return MergeResult{}, output.NewError("worktree_not_found", fmt.Sprintf("spec %q has no worktrees to merge", spec)).
			WithResource(spec).
			WithNextAction(fmt.Sprintf("create them with `%s epic worktree --data '{\"spec\":%q}'` and implement the spec there first", m.Config.Command, spec))
	}
	branch := Branch(spec)
	result := MergeResult{Spec: spec}

	for _, rw := range sw.Repos {
		if _, code, err := m.Git.Run(rw.Top, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err != nil {
			return result, err
		} else if code == 0 {
			return result, failed(fmt.Sprintf("a merge is already in progress in %s (repo %s)", rw.Top, rw.Repo),
				"finish or abort that merge, then retry")
		}
		if dirty, err := m.git(rw.Path, "status", "--porcelain"); err != nil {
			return result, err
		} else if dirty != "" {
			return result, failed(fmt.Sprintf("the worktree for %s in repo %s has uncommitted work (%s)", spec, rw.Repo, rw.Path),
				"commit or discard the work in the spec's worktree, then retry")
		}
		if overlap, err := m.dirtyOverlap(rw.Top, branch); err != nil {
			return result, err
		} else if len(overlap) > 0 {
			return result, failed(fmt.Sprintf("uncommitted changes in %s (repo %s) would be overwritten by merging %s: %s", rw.Top, rw.Repo, spec, strings.Join(overlap, ", ")),
				"commit or stash those changes, then retry")
		}
	}

	if err := m.refuseSpektacularChanges(sw); err != nil {
		return result, err
	}

	conflicts := map[string][]string{}
	for _, rw := range sw.Repos {
		out, code, err := m.Git.Run(rw.Top, "merge-tree", "--write-tree", "--name-only", "--no-messages", "HEAD", branch)
		if err != nil {
			return result, err
		}
		if code == 1 {
			// The first line is the tree object; the rest are the paths.
			lines := strings.Split(out, "\n")
			var paths []string
			for _, l := range lines[1:] {
				if l = strings.TrimSpace(l); l != "" {
					paths = append(paths, l)
				}
			}
			conflicts[rw.Repo] = paths
		}
	}
	if len(conflicts) > 0 {
		result.Conflicts = conflicts
		return result, nil
	}

	var merged []string
	for _, rw := range sw.Repos {
		if _, err := m.git(rw.Top, "merge", "--no-ff", "--no-edit", branch); err != nil {
			_, _, _ = m.Git.Run(rw.Top, "merge", "--abort")
			return result, output.NewError("epic_merge_conflict",
				fmt.Sprintf("merging %s into repo %s failed after a clean dry run (%v); already merged: %s", spec, rw.Repo, err, listOrNone(merged))).
				WithResource(spec).
				WithNextAction("report this to the user and stop; do not resolve it yourself")
		}
		merged = append(merged, rw.Repo)
	}
	result.Merged = true

	for _, rw := range sw.Repos {
		// Forced, because the worktree may still hold ignored files such as
		// build output; its committed work was checked clean above and is
		// merged.
		if _, err := m.git(rw.Top, "worktree", "remove", "--force", rw.Path); err != nil {
			return result, err
		}
		if _, err := m.git(rw.Top, "branch", "-d", branch); err != nil {
			return result, err
		}
	}
	if err := os.Remove(recordPath(m.ProjectRoot, spec)); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	_ = os.Remove(m.worktreeRoot(spec))
	result.Removed = true
	return result, nil
}

// spektacularPathspec matches every path under a .spektacular directory at
// any depth of a checkout, the checkout's top included.
const spektacularPathspec = ":(glob)**/.spektacular/**"

// refuseSpektacularChanges refuses a merge when the spec's branch changes
// anything under a .spektacular directory in any repo. Spektacular's files
// are only ever written in the project itself, so such a change is a
// mistake to undo, never something to merge. Every repo is checked first,
// so the refusal names all the offending paths at once.
func (m Manager) refuseSpektacularChanges(sw SpecWorktrees) error {
	branch := Branch(sw.Spec)
	var parts []string
	var first RepoWorktree
	for _, rw := range sw.Repos {
		out, err := m.git(rw.Top, "diff", "--name-only", "HEAD..."+branch, "--", spektacularPathspec)
		if err != nil {
			return err
		}
		var paths []string
		for _, p := range strings.Split(out, "\n") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			continue
		}
		if len(parts) == 0 {
			first = rw
		}
		parts = append(parts, fmt.Sprintf("%s: %s", rw.Repo, strings.Join(paths, ", ")))
	}
	if len(parts) == 0 {
		return nil
	}
	return output.NewError("epic_merge_touches_spektacular",
		fmt.Sprintf("%s's branch changes Spektacular's files, which are only ever written in the project, so nothing was merged in any repo: %s",
			sw.Spec, strings.Join(parts, "; "))).
		WithResource(sw.Spec).
		WithNextAction(fmt.Sprintf("undo those commits on %s in the spec's worktrees (for example `git -C %s revert <commit>`), record the change through `%s` from the project root instead, then retry `%s epic merge --data '{\"spec\":%q}'`",
			branch, first.Path, m.Config.Command, m.Config.Command, sw.Spec))
}

// dirtyOverlap lists uncommitted paths in top's main copy that branch
// changes, which a merge would refuse to overwrite. It asks git for bare
// names — changes against HEAD, staged or not, and untracked files — rather
// than parsing porcelain status, whose leading status columns do not survive
// output trimming.
func (m Manager) dirtyOverlap(top, branch string) ([]string, error) {
	changed, err := m.git(top, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := m.git(top, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if changed == "" && untracked == "" {
		return nil, nil
	}
	incomingOut, err := m.git(top, "diff", "--name-only", "HEAD..."+branch)
	if err != nil {
		return nil, err
	}
	incoming := map[string]bool{}
	for _, p := range strings.Split(incomingOut, "\n") {
		if p != "" {
			incoming[p] = true
		}
	}
	var overlap []string
	for _, p := range strings.Split(changed+"\n"+untracked, "\n") {
		if p = strings.TrimSpace(p); p != "" && incoming[p] {
			overlap = append(overlap, p)
		}
	}
	return overlap, nil
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
