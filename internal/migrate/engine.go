package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"gopkg.in/yaml.v3"
)

// ErrNoAgent is the cause of a skills reinstall that cannot run because the
// project settings record no agent.
var ErrNoAgent = errors.New("the project settings record no agent to reinstall skills for")

// Installer reinstalls the agent skills for the named agent. The cmd layer
// supplies it, so this package never imports the agent layer.
type Installer func(agent string) error

// Options controls Apply.
type Options struct {
	ProjectRoot   string
	BinaryVersion string
	DryRun        bool
	Skills        bool      // reinstall stale skills; false for init, which installs itself
	Install       Installer // required when Skills and not DryRun
}

// Report describes what an upgrade found and did. Inspect and Apply return
// the same shape, which is what makes a preview comparable with the apply
// that follows it.
type Report struct {
	Status  string        `json:"status"` // "up_to_date" | "upgrade_needed" | "upgraded" | "unsupported_format"
	DryRun  bool          `json:"dry_run"`
	Files   []FileReport  `json:"files"`
	Skipped []SkippedRepo `json:"skipped_repos,omitempty"`
	Skills  SkillsReport  `json:"skills"`
}

// FileReport is one settings file's pending or applied upgrade.
type FileReport struct {
	Path    string   `json:"path"`
	Kind    Kind     `json:"kind"`
	From    int      `json:"from_schema"`
	To      int      `json:"to_schema"`
	Steps   []string `json:"steps"`
	Actions []Action `json:"actions"`
	Backup  string   `json:"backup,omitempty"` // set on apply only
}

// SkippedRepo is a registered repo whose settings were not upgraded.
type SkippedRepo struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Reason   string `json:"reason"`
}

// SkillsReport describes the freshness of the project's installed skills.
// Installed and Status describe the project before any reinstall.
type SkillsReport struct {
	Installed string `json:"installed_version,omitempty"`
	Current   string `json:"current_version"`
	Status    string `json:"status"` // "match" | "mismatch" | "missing"
	Agent     string `json:"agent,omitempty"`
	Reinstall bool   `json:"reinstall"` // preview: would reinstall; apply: did
}

// Pending reports whether anything is out of date: a settings file behind its
// current format, or stale skills.
func (r Report) Pending() bool {
	return len(r.Files) > 0 || r.Skills.Status != "match"
}

// Inspect reports what an upgrade of the project at projectRoot would do,
// without touching disk. The gate and `version check` use it, so they report
// exactly what `migrate` would change.
func Inspect(projectRoot, binaryVersion string) (Report, error) {
	return Apply(Options{ProjectRoot: projectRoot, BinaryVersion: binaryVersion, DryRun: true, Skills: true})
}

// Apply upgrades the project's config.yaml, then every registered repo's
// repo.yaml present on disk, then (when opts.Skills) reinstalls stale agent
// skills. In dry-run mode it runs the same steps in memory and reports them.
// On failure it returns the partial report with the error: a
// *config.FormatError for a file newer than this build, or a *StepError
// naming where the upgrade stopped.
func Apply(opts Options) (Report, error) {
	rep := Report{DryRun: opts.DryRun, Files: []FileReport{}}
	u := &upgrader{opts: opts, created: map[string]bool{}}

	cfgDir := config.ProjectConfigDir(opts.ProjectRoot)
	projectPath := filepath.Join(cfgDir, config.ProjectConfigFileName)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		rep.Skills = SkillsReport{Current: opts.BinaryVersion, Status: "missing"}
		rep.Status = "up_to_date"
		return rep, nil
	}

	// Phase 1: project settings.
	doc, fr, err := u.upgradeFile(projectPath, KindProject)
	if fr != nil {
		rep.Files = append(rep.Files, *fr)
	}
	if err != nil {
		return finish(rep, err)
	}

	var proj struct {
		Agent         string             `yaml:"agent"`
		SkillsVersion string             `yaml:"skills_version"`
		Repos         []config.RepoEntry `yaml:"repos"`
	}
	if err := doc.Decode(&proj); err != nil {
		return finish(rep, fmt.Errorf("reading %s: %w", projectPath, err))
	}

	// Phase 2: every registered repo present on disk.
	for _, e := range proj.Repos {
		if e.Location == "" {
			e.Location = e.Local
		}
		dir := e.ResolvedLocation(opts.ProjectRoot)
		path := filepath.Join(dir, config.RepoConfigFileName)
		if u.created[path] {
			continue // created at the current format by a project step
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			rep.Skipped = append(rep.Skipped, SkippedRepo{Name: e.Name, Location: dir, Reason: "not on disk"})
			continue
		} else if err != nil {
			return finish(rep, fmt.Errorf("checking %s: %w", path, err))
		}
		_, fr, err := u.upgradeFile(path, KindRepo)
		if fr != nil {
			rep.Files = append(rep.Files, *fr)
		}
		var unreadable *unreadableError
		if errors.As(err, &unreadable) {
			// A repo.yaml that cannot be read is broken, not out of date:
			// it is left for the commands that load it to report, with
			// their own repair advice, and the rest of the upgrade goes on.
			rep.Skipped = append(rep.Skipped, SkippedRepo{Name: e.Name, Location: dir, Reason: "unreadable settings: " + unreadable.Err.Error()})
			continue
		}
		if err != nil {
			return finish(rep, err)
		}
	}

	// Phase 3: skills.
	installed := proj.SkillsVersion
	if installed == "" {
		installed = readLegacyVersion(cfgDir)
	}
	rep.Skills = SkillsReport{
		Installed: installed,
		Current:   opts.BinaryVersion,
		Status:    classifySkills(installed, opts.BinaryVersion),
		Agent:     proj.Agent,
	}
	if opts.Skills && rep.Skills.Status != "match" {
		if opts.DryRun {
			rep.Skills.Reinstall = true
			return finish(rep, nil)
		}
		const step = "reinstall agent skills"
		reached := currentSchema(KindProject)
		if proj.Agent == "" {
			return finish(rep, &StepError{Path: projectPath, Step: step, ReachedSchema: reached, Cause: ErrNoAgent})
		}
		if err := opts.Install(proj.Agent); err != nil {
			return finish(rep, &StepError{Path: projectPath, Step: step, ReachedSchema: reached, Cause: err})
		}
		// Recorded only once the install has succeeded, so a failed install
		// leaves skills reported as stale.
		root := docRoot(doc)
		setScalar(root, "skills_version", opts.BinaryVersion)
		setWrittenBy(root, opts.BinaryVersion)
		out, err := encode(doc)
		if err == nil {
			err = writeAtomic(projectPath, out)
		}
		if err != nil {
			return finish(rep, &StepError{Path: projectPath, Step: "record installed skills version", ReachedSchema: reached, Cause: err})
		}
		rep.Skills.Reinstall = true
	}
	return finish(rep, nil)
}

// UpgradeRepoFile brings the single repo.yaml at path to the current format,
// for a repo that is being set up but is not (yet) in a project's registry,
// so Apply would not reach it. It returns nil when the file was already
// current, and a *config.FormatError when it is newer than this build.
func UpgradeRepoFile(path, binaryVersion string) (*FileReport, error) {
	dir := filepath.Dir(path)
	u := &upgrader{
		opts:    Options{ProjectRoot: filepath.Dir(dir), BinaryVersion: binaryVersion},
		created: map[string]bool{},
	}
	_, fr, err := u.upgradeFile(path, KindRepo)
	return fr, err
}

// finish sets the report's overall status.
func finish(rep Report, err error) (Report, error) {
	if fe, ok := config.IsFormatError(err); ok && fe.Newer() {
		rep.Status = "unsupported_format"
		return rep, err
	}
	switch {
	case len(rep.Files) == 0 && !rep.Skills.Reinstall:
		rep.Status = "up_to_date"
	case rep.DryRun:
		rep.Status = "upgrade_needed"
	default:
		rep.Status = "upgraded"
	}
	return rep, err
}

type upgrader struct {
	opts Options
	// created records files a step creates (or, in a preview, would create)
	// at the current format, so the repo phase does not report them as
	// missing or pending.
	created map[string]bool
}

// upgradeFile brings one settings file to its kind's current format. It
// returns the (possibly upgraded, in memory) document and a report of the
// upgrade, which is nil when the file was already current.
func (u *upgrader) upgradeFile(path string, kind Kind) (*yaml.Node, *FileReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, &unreadableError{Path: path, Err: err}
	}
	doc, err := parseDoc(raw)
	if err != nil {
		return nil, nil, &unreadableError{Path: path, Err: err}
	}
	root := docRoot(doc)

	from := 1
	if s, ok := getScalar(root, "schema"); ok {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, nil, &unreadableError{Path: path, Err: fmt.Errorf("schema %q is not a whole number", s)}
		}
		from = config.NormaliseSchema(n)
	}
	want := currentSchema(kind)
	if from > want {
		return doc, nil, &config.FormatError{Path: path, Kind: string(kind), Found: from, Want: want}
	}
	if from == want {
		return doc, nil, nil
	}
	steps, err := stepsFrom(kind, from)
	if err != nil {
		return nil, nil, err
	}

	fr := &FileReport{Path: path, Kind: kind, From: from, To: want, Steps: []string{}, Actions: []Action{}}
	sc := &StepContext{ProjectRoot: u.opts.ProjectRoot, FileDir: filepath.Dir(path), BinaryVersion: u.opts.BinaryVersion}
	reached := from
	for _, s := range steps {
		fail := func(cause error) (*yaml.Node, *FileReport, error) {
			return doc, fr, &StepError{Path: path, Step: s.Description, ReachedSchema: reached, Cause: cause}
		}
		acts, err := s.Run(sc, doc)
		if err != nil {
			return fail(err)
		}
		fr.Steps = append(fr.Steps, s.Description)
		fr.Actions = append(fr.Actions, acts...)
		for _, a := range acts {
			if a.Op == "create" {
				u.created[a.Path] = true
			}
		}
		setSchema(root, s.From+1)
		setWrittenBy(root, u.opts.BinaryVersion)

		if !u.opts.DryRun {
			if fr.Backup == "" {
				backup, err := writeBackup(path, raw, from)
				if err != nil {
					return fail(err)
				}
				fr.Backup = backup
			}
			if err := perform(acts); err != nil {
				return fail(err)
			}
			out, err := encode(doc)
			if err != nil {
				return fail(err)
			}
			if err := writeAtomic(path, out); err != nil {
				return fail(err)
			}
		}
		reached = s.From + 1
	}
	return doc, fr, nil
}

// perform carries out a step's file side effects. Creating a file that
// already exists and removing one that is already gone are no-ops.
func perform(acts []Action) error {
	for _, a := range acts {
		switch a.Op {
		case "create":
			if _, err := os.Stat(a.Path); err == nil {
				continue
			}
			if err := writeAtomic(a.Path, a.Content); err != nil {
				return err
			}
		case "remove":
			if err := os.Remove(a.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s: %w", a.Path, err)
			}
		}
	}
	return nil
}

// writeBackup keeps a byte-identical copy of a settings file before its
// first rewrite, named after the format it started at
// (config.yaml.v1.old). An existing backup with the same bytes is reused;
// one with different bytes is never overwritten, and a numbered name is
// used instead.
func writeBackup(path string, raw []byte, from int) (string, error) {
	base := fmt.Sprintf("%s.v%d.old", path, from)
	name := base
	for i := 1; ; i++ {
		existing, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			break
		}
		if err == nil && bytes.Equal(existing, raw) {
			return name, nil
		}
		name = fmt.Sprintf("%s.%d", base, i)
	}
	if err := os.WriteFile(name, raw, 0644); err != nil {
		return "", fmt.Errorf("writing backup %s: %w", name, err)
	}
	return name, nil
}

// writeAtomic writes content via a temp file in the same folder and a
// rename, so a failure mid-write never leaves a truncated settings file.
func writeAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(content)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmpPath, 0644)
	}
	if werr == nil {
		werr = os.Rename(tmpPath, path)
	}
	if werr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}

// readLegacyVersion returns the trimmed content of an older project's
// standalone skills-version file, or "" when there is none.
func readLegacyVersion(cfgDir string) string {
	data, err := os.ReadFile(filepath.Join(cfgDir, legacyVersionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// classifySkills compares the installed skills version with the running
// build: empty is "missing", equal is "match", anything else "mismatch".
func classifySkills(installed, current string) string {
	switch installed {
	case "":
		return "missing"
	case current:
		return "match"
	default:
		return "mismatch"
	}
}
