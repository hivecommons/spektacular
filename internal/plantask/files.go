package plantask

import (
	"regexp"
	"slices"
	"strings"
)

// FileRef is one file a plan task says it changes, attributed to the
// registered repo it lives in. Path is repo-relative and carries no ":line"
// suffix.
type FileRef struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
}

var (
	contextTaskHeading = regexp.MustCompile(`^###\s+Task:\s*(.+?)\s*$`)
	backticked         = regexp.MustCompile("`([^`\n]+)`")
	// pathToken is an optional "<repo>:" prefix, a path of plain path
	// characters, and an optional ":N", ":N-M" or ":N,M" line suffix.
	pathToken = regexp.MustCompile(`^(?:([A-Za-z0-9_-]+):)?([A-Za-z0-9_./-]+?)(?::\d+(?:[-,]\d+)*)?$`)
	// fileExt is a lower-case extension, which tells "epic.go" from a Go
	// selector such as "epic.Member".
	fileExt = regexp.MustCompile(`\.[a-z0-9]{1,6}$`)
)

// TaskFiles reads, for each task in plan, the files its section of the plan's
// context document names, keyed by task title. Files are the backticked paths
// in the task's "### Task: <title>" section. A "<repo>:" prefix attributes a
// path to that repo when repos registers it; otherwise the path belongs to the
// task's own **Repo:**, or to the only registered repo when the task names
// none. An unregistered prefix stays part of the path.
//
// Like Parse it never fails: a task without a context section, or a plan
// without a context document, contributes nothing.
func TaskFiles(plan, context []byte, repos []string) map[string][]FileRef {
	sections := contextTaskSections(context)
	files := map[string][]FileRef{}
	for _, task := range Parse(plan).Tasks {
		body, ok := sections[task.Title]
		if !ok {
			continue
		}
		fallback := task.Repo
		if fallback == "" && len(repos) == 1 {
			fallback = repos[0]
		}
		var refs []FileRef
		for _, m := range backticked.FindAllStringSubmatch(body, -1) {
			ref, ok := fileRef(m[1], repos, fallback)
			if ok && !slices.Contains(refs, ref) {
				refs = append(refs, ref)
			}
		}
		if len(refs) > 0 {
			files[task.Title] = refs
		}
	}
	return files
}

// contextTaskSections splits a context document into its "### Task:" sections
// by title. A section runs to the next level-2 or level-3 heading; fenced code
// is skipped, so neither a heading nor a path inside an example counts.
func contextTaskSections(context []byte) map[string]string {
	sections := map[string]string{}
	var title string
	var body strings.Builder
	inFence := false
	flush := func() {
		if title != "" {
			sections[title] = body.String()
		}
		title = ""
		body.Reset()
	}
	for raw := range strings.SplitSeq(string(context), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "###") {
			flush()
			if m := contextTaskHeading.FindStringSubmatch(line); m != nil {
				title = m[1]
			}
			continue
		}
		if title != "" {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	flush()
	return sections
}

// fileRef turns one backticked token into a file reference, or reports that
// the token is not a file path (a command, a symbol, a directory).
func fileRef(token string, repos []string, fallback string) (FileRef, bool) {
	m := pathToken.FindStringSubmatch(token)
	if m == nil {
		return FileRef{}, false
	}
	prefix, path := m[1], m[2]
	if strings.HasSuffix(path, "/") || !(strings.Contains(path, "/") || fileExt.MatchString(path)) {
		return FileRef{}, false
	}
	repo := fallback
	if prefix != "" {
		if slices.Contains(repos, prefix) {
			repo = prefix
		} else {
			path = prefix + ":" + path
		}
	}
	return FileRef{Repo: repo, Path: path}, true
}
