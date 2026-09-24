package repo

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/store"
)

// Bounds on what an examination reads. The examiner runs while a user waits
// between two questions, so every limit here is a functional requirement
// rather than a tuning knob: widening them is a change to what the
// examination is allowed to be, not an improvement.
const (
	// examineMaxReadmeBytes caps how much of a README is read from disk.
	examineMaxReadmeBytes = 8192
	// examineMaxReadmeRunes caps the prose handed back to a caller.
	examineMaxReadmeRunes = 600
	// examineMaxTopLevel caps the top-level entries reported.
	examineMaxTopLevel = 40
	// examineMaxHistogramFiles caps the files counted for the language
	// histogram, which walks no deeper than two directory levels.
	examineMaxHistogramFiles = 2000
	// examineMaxLanguages caps the languages reported, strongest first.
	examineMaxLanguages = 5
)

// Evidence is what a repo states about itself: the opening prose of its
// README, the identity and summary declared by whatever manifest its
// language uses, what sits at its top level, and which languages it is
// written in. It is deliberately shallow — no source file is read for its
// contents, and nothing here is inferred by analysing code.
//
// Every field is optional. Readable reports whether the examination found
// anything worth stating, so a caller branches on evidence rather than on
// an error: a repo that offers nothing readable is an ordinary case, not a
// failure.
type Evidence struct {
	Readable  bool
	Readme    string
	Manifest  string
	Identity  string
	Summary   string
	TopLevel  []string
	Languages []string
}

// Examine reports what the repo rooted at dir says about itself. It never
// fails and never returns an error: an unreadable, empty or missing
// directory yields the zero Evidence with Readable false, which is what
// keeps such a repo addable rather than turning it into an error path.
//
// The examination is bounded by construction — one README up to a fixed
// length, at most one manifest, one directory listing, and an extension
// histogram over no more than two directory levels with a hard cap on files
// counted. It honours the repo's own .spektacular_ignore so vendored and
// build directories cannot dominate the histogram. It reads no source file
// contents and never recurses into a source tree.
func Examine(dir string) Evidence {
	var ev Evidence

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ev
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ev
	}

	ignore := store.LoadIgnore(dir)

	ev.Readme = readReadme(dir, entries)
	ev.Manifest, ev.Identity, ev.Summary = readManifest(dir, entries)
	ev.TopLevel = topLevelEntries(entries, ignore)
	ev.Languages = detectLanguages(dir, entries, ignore)

	ev.Readable = ev.Readme != "" || ev.Identity != "" || ev.Summary != "" || len(ev.Languages) > 0
	return ev
}

// readReadme returns the opening prose of the repo's README, with its
// leading heading skipped so the caller gets a sentence rather than a title.
// A README that is nothing but a heading yields the empty string, which is
// the honest answer: it states no prose about itself.
func readReadme(dir string, entries []os.DirEntry) string {
	name := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(e.Name()), "README") {
			name = e.Name()
			break
		}
	}
	if name == "" {
		return ""
	}

	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, examineMaxReadmeBytes)
	n, err := f.Read(buf)
	if n == 0 || (err != nil && n == 0) {
		return ""
	}

	var prose []string
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		line = strings.TrimSpace(line)
		// Skip the title, badge lines and any other heading: the caller
		// wants what the repo says about itself, not how it is decorated.
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[!") ||
			strings.HasPrefix(line, "![") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "===") || strings.HasPrefix(line, "<") {
			if len(prose) > 0 {
				break
			}
			continue
		}
		prose = append(prose, line)
	}

	return truncateRunes(strings.Join(prose, " "), examineMaxReadmeRunes)
}

// manifestReader extracts a declared name and, where the format carries
// one, a declared description from a manifest's contents.
type manifestReader func(content string) (identity, summary string)

// manifests are the language manifests the examination recognises, in the
// order they are looked for. Only the first one present is read. Every
// reader here is hand-rolled: no manifest-parsing dependency exists, and
// adding one would widen the examination beyond what it is allowed to do.
var manifests = []struct {
	name string
	read manifestReader
}{
	{"go.mod", readGoMod},
	{"package.json", readPackageJSON},
	{"Cargo.toml", readTOMLPackage},
	{"pyproject.toml", readTOMLPackage},
	{"pom.xml", readPomXML},
	{"Gemfile", func(string) (string, string) { return "", "" }},
}

// readManifest finds the first recognised manifest at the repo's top level
// and returns its filename alongside whatever identity and summary it
// declares. A manifest that is present but declares neither still names
// itself, since its presence is a fact about the repo.
func readManifest(dir string, entries []os.DirEntry) (manifest, identity, summary string) {
	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			present[e.Name()] = true
		}
	}

	for _, m := range manifests {
		if !present[m.name] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, m.name))
		if err != nil {
			continue
		}
		identity, summary = m.read(string(raw))
		return m.name, identity, summary
	}
	return "", "", ""
}

// readGoMod takes the module path's last segment as the identity. go.mod
// declares no description, so the summary is always empty.
func readGoMod(content string) (string, string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "module ")
		if !ok {
			continue
		}
		module := strings.TrimSpace(rest)
		if i := strings.Index(module, "//"); i >= 0 {
			module = strings.TrimSpace(module[:i])
		}
		module = strings.Trim(module, `"`)
		if module == "" {
			return "", ""
		}
		return path.Base(module), ""
	}
	return "", ""
}

// readPackageJSON reads the declared name and description.
func readPackageJSON(content string) (string, string) {
	var pkg struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		return "", ""
	}
	return pkg.Name, pkg.Description
}

// readTOMLPackage scans a TOML manifest's [package] or [project] table for
// name and description. It is a deliberate line scan rather than a TOML
// parse: no TOML library is available, and only two keys are wanted.
func readTOMLPackage(content string) (string, string) {
	var identity, summary string
	inTable := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inTable = line == "[package]" || line == "[project]" || line == "[tool.poetry]"
			continue
		}
		if !inTable {
			continue
		}
		if v, ok := tomlValue(line, "name"); ok && identity == "" {
			identity = v
		}
		if v, ok := tomlValue(line, "description"); ok && summary == "" {
			summary = v
		}
	}
	return identity, summary
}

// tomlValue reads a simple `key = "value"` assignment. Anything more
// elaborate (arrays, inline tables, multi-line strings) is not a name or a
// description and is skipped.
func tomlValue(line, key string) (string, bool) {
	rest, ok := strings.CutPrefix(line, key)
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	rest, ok = strings.CutPrefix(rest, "=")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if len(rest) < 2 || !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	end := strings.Index(rest[1:], `"`)
	if end < 0 {
		return "", false
	}
	return rest[1 : 1+end], true
}

// readPomXML reads the project's artifactId and description. It scans for
// the elements rather than unmarshalling the whole POM, since a POM's
// modules and dependencies carry the same element names.
func readPomXML(content string) (string, string) {
	return xmlElement(content, "artifactId"), xmlElement(content, "description")
}

// xmlElement returns the text of the first <name>...</name> element.
func xmlElement(content, name string) string {
	openTag, closeTag := "<"+name+">", "</"+name+">"
	start := strings.Index(content, openTag)
	if start < 0 {
		return ""
	}
	start += len(openTag)
	end := strings.Index(content[start:], closeTag)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(content[start : start+end])
}

// topLevelEntries lists what sits at the repo's top level, directories
// first and each sorted by name, excluding dotfiles and anything the repo's
// own exclusion file rules out.
func topLevelEntries(entries []os.DirEntry, ignore store.IgnoreMatcher) []string {
	var dirs, files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || ignore.Ignored(name, e.IsDir()) {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, name+"/")
		} else {
			files = append(files, name)
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)

	out := append(dirs, files...)
	if len(out) > examineMaxTopLevel {
		out = out[:examineMaxTopLevel]
	}
	return out
}

// extensionLanguages maps a file extension to the language it indicates.
// Only extensions that identify a language on their own are listed: a
// generic one like .json or .md says nothing about what a repo is written
// in and would drown out the signal.
var extensionLanguages = map[string]string{
	".go":    "Go",
	".rs":    "Rust",
	".py":    "Python",
	".rb":    "Ruby",
	".java":  "Java",
	".kt":    "Kotlin",
	".swift": "Swift",
	".c":     "C",
	".h":     "C",
	".cc":    "C++",
	".cpp":   "C++",
	".hpp":   "C++",
	".cs":    "C#",
	".ts":    "TypeScript",
	".tsx":   "TypeScript",
	".js":    "JavaScript",
	".jsx":   "JavaScript",
	".mjs":   "JavaScript",
	".php":   "PHP",
	".ex":    "Elixir",
	".exs":   "Elixir",
	".scala": "Scala",
	".sh":    "Shell",
	".lua":   "Lua",
	".dart":  "Dart",
	".astro": "Astro",
	".vue":   "Vue",
	".css":   "CSS",
	".scss":  "CSS",
	".html":  "HTML",
	".sql":   "SQL",
	".tf":    "Terraform",
	".hcl":   "HCL",
}

// vendoredDirs are directory names that hold dependencies or build output
// rather than the repo's own code. They are skipped when weighing languages
// so a repo is described by what it is written in, not by what it depends
// on. A repo's own .spektacular_ignore is honoured on top of this; this set
// exists because most repos do not carry one and node_modules would
// otherwise decide the answer.
var vendoredDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"out":          true,
	"obj":          true,
	"coverage":     true,
	"__pycache__":  true,
	"venv":         true,
	"third_party":  true,
	"testdata":     true,
}

// detectLanguages weighs the languages present by counting files with a
// recognised extension across the repo's top two directory levels, and
// returns them strongest first. It reads no file contents, never recurses
// past the second level, and stops after a fixed number of files, so the
// call stays inside a conversational pause however large the repo is.
func detectLanguages(dir string, entries []os.DirEntry, ignore store.IgnoreMatcher) []string {
	counts := map[string]int{}
	seen := 0

	count := func(rel string) {
		if lang, ok := extensionLanguages[strings.ToLower(filepath.Ext(rel))]; ok {
			counts[lang]++
		}
		seen++
	}

	for _, e := range entries {
		if seen >= examineMaxHistogramFiles {
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || ignore.Ignored(name, e.IsDir()) {
			continue
		}
		if !e.IsDir() {
			count(name)
			continue
		}
		if vendoredDirs[name] {
			continue
		}
		// Second level, and no deeper.
		children, err := os.ReadDir(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, c := range children {
			if seen >= examineMaxHistogramFiles {
				break
			}
			if c.IsDir() || strings.HasPrefix(c.Name(), ".") {
				continue
			}
			rel := name + "/" + c.Name()
			if ignore.Ignored(rel, false) {
				continue
			}
			count(rel)
		}
	}

	if len(counts) == 0 {
		return nil
	}

	langs := make([]string, 0, len(counts))
	for lang := range counts {
		langs = append(langs, lang)
	}
	// Strongest first, name-ordered within a tie so the result is stable.
	sort.Slice(langs, func(i, j int) bool {
		if counts[langs[i]] != counts[langs[j]] {
			return counts[langs[i]] > counts[langs[j]]
		}
		return langs[i] < langs[j]
	})
	if len(langs) > examineMaxLanguages {
		langs = langs[:examineMaxLanguages]
	}
	return langs
}

// truncateRunes shortens s to at most max runes, cutting at the last word
// boundary so the result reads as prose rather than a severed word.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "..."
}
