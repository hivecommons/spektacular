**`artifact.Kind` and `artifact.Address`** (new). An address is what a caller passes and what a list prints. It never carries an extension or a path separator, and it says nothing about how a store persists the document.

```go
type Kind string // "spec" | "plan" | "changelog"

type Address struct {
    Kind     Kind
    Feature  string // bare feature name, e.g. "000059_normalise-artifact-addressing"
    Document string // plan only: "plan", "context", "research", "test-plan", ...; empty otherwise
}

// Parse validates positional arguments for a document verb of kind.
// Spec/changelog: exactly [feature]. Plan: [feature, document].
func Parse(kind Kind, args []string) (Address, error)

// StorePath is the file provider's location of the addressed document
// under a store directory: <dir>/<feature>.md or <dir>/<feature>/<document>.md.
func (a Address) StorePath(dir string) string

// FeatureDir is <dir>/<feature>, used by `plan file list <feature>`.
func FeatureDir(dir, feature string) string

// NameFromEntry turns a listed file entry into its bare address segment;
// ok is false for entries that are not addressable documents.
func NameFromEntry(entryName string, isDir bool, want EntryKind) (name string, ok bool)
```

**Refusal errors** (new, typed so the command layer renders the envelope and next action):

```go
const (
    ErrCodeUnexpectedExtension = "unexpected_extension"
    ErrCodeDocumentRequired    = "document_required"
)

// ExtensionError: the input carried an extension or a joined path.
// Corrected holds the correctly spelled address the next action restates.
type ExtensionError struct { Input string; Corrected Address }

// DocumentRequiredError: a plan document verb got only a feature.
type DocumentRequiredError struct { Feature string }
```

**Store resolver contract** (changed). It returns the location base alongside the store and directory:

```go
// base is store-relative: the directory reported paths are made relative to
// (".spektacular" for central stores, "" for a repo-routed changelog store).
resolveStore(repoName string) (st store.Store, storeDir string, base string, err error)
```

**List output** (changed wire format, same keys). Every entry is `{ "name": <bare address segment>, "path": <location relative to the declaring config folder>, "modified_at"?, "created_date"?, "document_status"?, "closed_date"? }`.

- `spec file list` and `changelog file list [--repo]`: `name` is the feature; `path` is e.g. `specs/<feature>.md`, `changelog/<feature>.md`, or with `--repo`, `changelog/<project>/<feature>.md`.
- `plan file list`: `name` is the feature; `path` is `plans/<feature>`.
- `plan file list <feature>`: `name` is the document; `path` is `plans/<feature>/<document>.md`.

**`set-document-status` output** (changed): `{ "name": <feature>, "document"?: <document, plan only>, "path": <config-relative location>, "document_status", "closed_date"? }`. Previously `{ "path": <raw argument>, ... }`.

**Status results** (changed). `plan.StatusResult` and `implement.StatusResult` gain `PlanDocument string \`json:"plan_document"\`` (always `"plan"`). `PlanPath` changes meaning from an absolute host path to a location relative to the config folder (`plans/<feature>/plan.md`). The JSON output schemas for both status commands are updated to match.

**Error envelope** (unchanged shape). Both refusals use the existing `output.ErrorResponse` with `code`, `message`, `resource` (the address as typed) and `next_action`. No store interface (`store.Store`, `Reader`, `Writer`) changes.
