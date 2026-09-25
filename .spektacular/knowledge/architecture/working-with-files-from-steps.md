---
tags: [storage, workflow, step, paths, filesystem, cli, metadata, template, addressing]
---

# Working with Files from Steps and Commands

Steps and commands interact with the project's files through a `store.Store` interface — never via `os` directly.

This binds `cmd/` exactly as it binds `internal/steps/`. A command holding a store is under the same rule as a step holding one: if the subject of the question is an artifact the store owns, the store answers it.

## What "through the store" covers

Everything the store knows about an artifact, not just its bytes:

| Question | Ask the store | Never |
|---|---|---|
| What does it contain? | `st.Read(path)` | `os.ReadFile(...)` |
| Does it exist? | `st.Exists(path)` | `os.Stat(...)` |
| What is in this directory? | `st.List(path)` | `os.ReadDir(...)` |
| When was it changed? | `st.Stat(path)` | `os.Stat(...).ModTime()` |

`st.Root()` is never used to build a location for a store document, whether to read it or to show it to an agent. Using it to rebuild a filesystem location and then read that location is the same violation as calling `os` directly; it just takes two lines instead of one.

```go
// Not fine: Root() used to reach the bytes behind the store's back.
info, err := os.Stat(filepath.Join(st.Root(), storePath))

// Not fine either: Root() used to hand an agent a path to open.
absPath := filepath.Join(st.Root(), SpecFilePath(cfg.SpecDir, name))
```

## Agent-facing output names documents, never paths

Nothing an agent or an external caller reads may contain a path to a store document: not a step instruction, not a command result, not a `next_action`. A store's documents may not be on disk at all, and a host path invites the reader to open the file with its own tools instead of going through Spektacular.

Instead, output names the document by its address and the CLI command that reads it:

- a spec: `spektacular spec file read <feature>`
- a plan document: `spektacular plan file read <feature> <document>`
- a changelog record: `spektacular changelog file read <feature> [--repo <name>]`

In templates that means the address variables (`{{spec_name}}`, `{{plan_name}}`) inside a CLI read, never a path variable.

Where output also reports *where* a document is stored, such as a list's `path` or a status `plan_path`, that location is relative to the folder holding the configuration file that declares the store (for example `specs/<feature>.md`). It is informational only, never an address and never something to open.

## Why this is stricter than it looks

The interface is the seam a non-filesystem backend swaps in at. A store backed by an HTTP API has no meaningful `Root()`, so code that reaches around the interface does not fail loudly there — it **degrades silently**. `Root()` returns empty, the `os` call errors, the caller falls back to whatever default it has, and a field that should have been unavailable is instead quietly wrong. A wrong answer is worse than a missing one, especially for a value an external caller is making decisions on.

That is the real cost of the shortcut: not that it is untidy, but that it converts "this backend cannot tell you" into "here is a plausible-looking wrong value".

## When the contract cannot answer

If the store genuinely cannot answer a question the caller needs — the interface has no method for it — the fix is to **extend the contract**, not to route around it. Add the method to `Reader` or `Writer`, implement it in `FileStore`, delegate it in `ignoreStore`, and let a future backend fill it from its own metadata. `Stat` was added exactly this way.

A gap in the interface is a reason to widen the interface. It is never a licence to use `os`.

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
    Stat(path string) (FileInfo, error)             // ErrNotFound if missing
    Search(terms []string, opts SearchOptions) ([]Hit, error)
}

type FileInfo struct {
    ModTime   time.Time                             // last modification time
    CreatedAt time.Time                             // zero where the backend cannot report it
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

These helpers are the file provider's layout, used inside Spektacular to reach the store. They are never rendered into agent-facing output. See "Agent-facing output names documents, never paths" above.

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
// entries is []store.DirEntry — each has Name (the child name, not a full path),
// IsDir, so a caller can tell a file from a subdirectory and recurse, and
// ModTime, so listing N artifacts reports when each last changed without N
// further Stat calls
```

### Ask when a file last changed

```go
info, err := st.Stat(SpecFilePath(cfg.SpecDir, name))
// info.ModTime is the backend's modification time; info.CreatedAt is zero on
// FileStore, which has no portable birth time. Never os.Stat a store path —
// a non-filesystem backend has no meaningful Root() to join it against.
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
