---
tags: [storage, workflow, step, paths, filesystem]
---

# Working with Files from Steps

Steps interact with the project's files through a `store.Store` interface — never via `os` directly.

## How the Store Reaches a Step

`store.Store` is the third parameter of every `StepCallback`:

```go
type StepCallback func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error)
```

The store is set once at workflow construction and passed to every step automatically. Steps that don't need it simply ignore the parameter. Steps that require it should guard against `nil`:

```go
if st == nil {
    return "", fmt.Errorf("store required for this step")
}
```

## Interface

`Store` is the read half and the write half together, so a backend that cannot be written to can be
held as a `Reader` alone rather than as a `Store` that breaks its own contract:

```go
type Reader interface {
    Read(path string) ([]byte, error)               // ErrNotFound if missing
    List(path string) ([]DirEntry, error)           // ErrNotFound if dir missing
    Exists(path string) bool
    Search(terms []string, opts SearchOptions) ([]Hit, error)
}

type Writer interface {
    Write(path string, content []byte) error        // creates or overwrites; makes parent dirs
    Delete(path string) error                       // idempotent on missing
}

type Store interface {
    Reader
    Writer
    Root() string                                   // absolute path to the store root
}
```

All paths are **store-relative**. The store commands build is rooted at the **project root** —
`store.NewSourceStore(root, "project")` — so a store-relative path is a project-root-relative one
like `.spektacular/specs/my-feature.md`, not a path relative to `.spektacular/`. The store rejects
paths that escape the root (e.g. `../secret`).

## Path Conventions

File locations are **constants, not data**. Do not store paths in `workflow.Data`. Instead, derive them from the configured directory and the spec name using a typed helper:

```go
// In internal/steps/spec/steps.go
func SpecFilePath(dir, name string) string {
    return dir + "/" + name + ".md"
}
```

`dir` comes from the workflow config (`cfg.SpecDir`, itself from `config.yaml`, defaulting to
`.spektacular/specs`) — never hard-code it.

Use `st.Root()` only when you need the absolute path for output shown to agents:

```go
absPath := filepath.Join(st.Root(), SpecFilePath(cfg.SpecDir, name))
```

## Common Patterns

### Create a file

```go
return st.Write(SpecFilePath(cfg.SpecDir, name), []byte(content))
```

### Read a file

```go
content, err := st.Read(SpecFilePath(cfg.SpecDir, name))
if errors.Is(err, store.ErrNotFound) {
    // handle missing
}
```

### Check existence before acting

```go
if !st.Exists(SpecFilePath(cfg.SpecDir, name)) {
    return fmt.Errorf("spec %q not found", name)
}
```

### List files in a directory

```go
entries, err := st.List(cfg.SpecDir)
// entries is []store.DirEntry — each has Name (the child name, not a full path)
// and IsDir, so a caller can tell a file from a subdirectory and recurse
```

## Injecting the Store

The store is constructed in `cmd/` and passed to `workflow.New`:

```go
wf := workflow.New(steps, statePath, wfCfg, store.NewSourceStore(root, "project"), out)
```

`NewSourceStore` (`internal/store/ignore.go`) is the standard constructor: it wraps a
`NewFileStore(root, label)` with the exclusions declared by the root's own `.spektacular_ignore`.
Build stores through it rather than reaching for `NewFileStore` directly, so listings and searches
honour those exclusions.

Pass `nil` for workflows that only query state and never touch files (e.g. `spec status`, `spec steps`).

## Future Backends

The `Store` interface is backend-agnostic. A future HTTP or database backend swaps in at the `workflow.New` call site — no step code changes.
