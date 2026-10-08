**Worktree record** is the per-spec file in the main project. It replaces `repo.Overlay`, and its shape is the overlay's, moved:

```go
// worktree package
type Record struct {
    Spec  string            `json:"spec"`
    Repos map[string]string `json:"repos"` // registered repo name -> absolute code root in the spec's worktree
}

func (m Manager) Record(spec string) (Record, bool, error) // no git; (zero, false, nil) when absent
```

It is written by `Ensure`, removed by `Merge`, and read by `Record` (and by a package-level helper that takes the project root, so cmd can read it without building a manager).

**Spec-scoped repo view.** `repo.New` loses its implicit overlay. A sibling constructor applies a location map explicitly:

```go
func New(cfg config.Config, projectRoot string, git GitRunner) (*Set, error)            // registered locations only
func NewWithLocations(cfg config.Config, projectRoot string, git GitRunner, locs map[string]string) (*Set, error)
```

`OverlayFile`, `Overlay` and `readOverlay` are removed.

**Code roots in the workflow config.** `workflow.Config` gains a runtime-only field. It is not persisted, like the other config fields:

```go
type CodeRoot struct{ Repo, Root string }
type Config struct { /* … */ CodeRoots []CodeRoot } // empty unless the spec has a worktree record
```

Implement steps expose it to templates as `has_worktree_roots` (bool) and `worktree_roots` (a list of `{repo, root}`). These keys are distinct from the banned `repos` roster key.

**Auto-commit targets.** `autocommit.Targets` keeps its signature. A spec-scoped variant takes the record's location map, so the code targets resolve to the worktrees. The main-checkout artifact commit reuses the existing path-scoped commit-under-lock helper that plan lanes use. Its input is a list of `{dir, paths}` covering the spec's plan directory, spec file, changelog records and implement lane files.

**Merge refusal.** A new error code, `epic_merge_touches_spektacular`. Its message lists the offending paths per repo, and its `next_action` says to undo those commits on `spek/<spec>` in the named worktree and record the change through the CLI from the project root. `MergeResult` is unchanged.

**Status.** No type changes. `RunPart.Root` keeps naming the spec's project worktree when one exists. Only where the lane and finished evidence are read changes.
