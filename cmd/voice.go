package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/steps/spec"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/voice"
)

// voiceFetchTimeout bounds one fetch from a voice source, MCP handshake
// included.
const voiceFetchTimeout = 2 * time.Minute

var voiceCmd = &cobra.Command{
	Use:   "voice",
	Short: "Start a spec from a voice note (Plaud, Wispr Flow, a transcript file, or stdin), optionally publishing it as an RFC",
	Long: `Voice fetches one spoken note from a voice interface, drafts it onto the spec
scaffold and hands it on. Without --publish it starts the normal spec workflow
seeded with the note, so the driving agent runs the interview over the gaps.
With --publish it writes the drafted spec as final and files it as an RFC issue
with one child issue per requirement — the same hand-off ` + "`publish`" + ` makes — so a
hive can pick the work up without an interview. That needs a note whose summary
already carries Requirements and Acceptance Criteria sections (a recorder's
custom summary template shaped like the spec scaffold does this); otherwise
voice refuses and names what is missing.

Sources:
  plaud      Plaud's official MCP server (npx -y @plaud-ai/mcp@latest; sign in once with "install")
  wisprflow  Wispr Flow's local history (flow.sqlite via sqlite3) or a folder of exported notes
  file       a .md/.txt/.srt/.vtt note, or the newest one in a folder (--path)
  stdin      the note text piped or pasted in

Examples:
  spektacular voice --from plaud --latest
  spektacular voice --from plaud --title "cluster drift" --publish --repo acme/demo
  spektacular voice --from file --path memo.srt
  pbpaste | spektacular voice --from stdin --publish --dry-run`,
	Args: cobra.NoArgs,
	RunE: runVoice,
}

type voiceResult struct {
	Source     string   `json:"source"`
	Title      string   `json:"title"`
	URI        string   `json:"uri"`
	Material   string   `json:"material"`
	SpecName   string   `json:"spec_name,omitempty"`
	SpecPath   string   `json:"spec_path,omitempty"`
	Missing    []string `json:"missing_sections,omitempty"`
	RFCURL     string   `json:"rfc_url,omitempty"`
	Children   []string `json:"children,omitempty"`
	NextAction string   `json:"next_action,omitempty"`
}

func runVoice(cmd *cobra.Command, _ []string) error {
	from, _ := cmd.Flags().GetString("from")
	latest, _ := cmd.Flags().GetBool("latest")
	title, _ := cmd.Flags().GetString("title")
	path, _ := cmd.Flags().GetString("path")
	name, _ := cmd.Flags().GetString("name")
	publishIt, _ := cmd.Flags().GetBool("publish")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	format, _ := cmd.Flags().GetString("format")
	command, _ := cmd.Flags().GetString("command")

	if from == "" {
		return output.NewError("voice_source_required", "no voice source was given").
			WithNextAction("re-run with --from " + strings.Join(voice.SourceNames, "|"))
	}
	if format != publishFormatPretty && format != publishFormatJSON {
		return output.NewError("voice_format_unsupported", fmt.Sprintf("format %q is not supported", format)).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", publishFormatPretty, publishFormatJSON))
	}
	if !latest && title == "" && (from == "plaud" || from == "wisprflow") {
		return output.NewError("voice_query_required", "say which note to take").
			WithNextAction("re-run with --latest, or --title <part of the recording's name>")
	}

	root, err := projectRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	src, err := voice.Open(from, voice.Options{Command: command, Stdin: cmd.InOrStdin()})
	if err != nil {
		return output.NewError("voice_source_unknown", err.Error()).
			WithNextAction("re-run with --from " + strings.Join(voice.SourceNames, "|"))
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, voiceFetchTimeout)
	defer cancel()
	note, err := src.Fetch(ctx, voice.Query{Latest: latest, Title: title, Path: path})
	if err != nil {
		return output.NewError("voice_fetch_failed", err.Error()).WithResource(from).
			WithNextAction("fix the source and retry; or export the note and re-run with --from file --path <file>")
	}

	draft := voice.DraftNote(note)
	if name == "" {
		name = voice.Slug(draft.Title)
	}
	st := store.NewSourceStore(root, "project")
	res := voiceResult{Source: from, Title: draft.Title, URI: note.URI, Missing: draft.Missing}

	if !publishIt {
		res.Material = filepath.Join(".spektacular", "tmp", "voice", name)
		if !dryRun {
			if err := writeVoiceMaterial(root, res.Material, note, draft); err != nil {
				return err
			}
		}
		data, _ := json.Marshal(map[string]any{
			"name":     name,
			"sources":  []map[string]string{{"uri": note.URI}},
			"material": res.Material,
		})
		nc := &cobra.Command{RunE: runSpecNew}
		nc.Flags().Bool("schema", false, "")
		nc.Flags().Bool("dry-run", dryRun, "")
		nc.Flags().String("data", string(data), "")
		nc.Flags().Bool("force", false, "")
		nc.Flags().String("stdin", "", "")
		nc.Flags().String("file", "", "")
		nc.SetOut(cmd.OutOrStdout())
		nc.SetIn(cmd.InOrStdin())
		return runSpecNew(nc, nil)
	}

	if len(draft.Missing) > 0 {
		return output.NewError("voice_not_spec_shaped",
			fmt.Sprintf("the note %q has no %s section, so it cannot be published without an interview", draft.Title, strings.Join(draft.Missing, " or "))).
			WithResource(note.URI).
			WithNextAction(fmt.Sprintf("re-run without --publish to start the interview from this note; or record with a summary template whose headings are the spec sections (%s)", strings.Join(voice.Sections, ", ")))
	}

	resolved, err := spec.ResolveIdentifier(spec.IdentifierRequest{
		Name: name, Method: cfg.Spec.IDMethod, SpecDir: cfg.Spec.Config.Directory, Store: st,
		Now: specIdentifierNow, RandomID: specIdentifierRandomID,
	})
	if err != nil {
		return err
	}
	res.SpecName = resolved.Name
	res.SpecPath = spec.SpecFilePath(cfg.Spec.Config.Directory, resolved.Name)
	if st.Exists(res.SpecPath) {
		return output.NewError("voice_spec_exists", fmt.Sprintf("a spec named %q already exists", resolved.Name)).
			WithResource(res.SpecPath).
			WithNextAction("re-run with --name <other name>, or publish the existing spec with `" + cfg.Command + " publish " + resolved.Name + "`")
	}
	sources, err := stampSources(cfg, []sourceInput{{URI: note.URI}}, time.Now().UTC())
	if err != nil {
		return err
	}
	final := metadata.StatusFinal
	content, err := metadata.Merge(nil, []byte(draft.Body), metadata.UpdateOptions{Sources: &sources, DocumentStatus: &final})
	if err != nil {
		return err
	}
	if dryRun {
		res.NextAction = fmt.Sprintf("re-run without --dry-run to write %s and file it as an RFC with %d child issues", res.SpecPath, len(requirementTitles(draft.Body)))
		if format == publishFormatJSON {
			return output.Write(cmd.OutOrStdout(), map[string]any{"dry_run": true, "voice": res, "children": requirementTitles(draft.Body)}, globalFields)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Would write %s from %s (%s) and publish it; children:\n", res.SpecPath, from, note.URI)
		for _, c := range requirementTitles(draft.Body) {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", c)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n%s", draft.Body)
		return nil
	}
	if err := st.Write(res.SpecPath, content); err != nil {
		return err
	}
	res.Material = filepath.Join(".spektacular", "work", resolved.Name, "voice")
	if err := writeVoiceMaterial(root, res.Material, note, draft); err != nil {
		return err
	}

	pc := &cobra.Command{RunE: runPublish}
	publishFlags(pc)
	for _, f := range []string{"target", "repo"} {
		if v, _ := cmd.Flags().GetString(f); v != "" {
			_ = pc.Flags().Set(f, v)
		}
	}
	_ = pc.Flags().Set("format", publishFormatJSON)
	var pubOut bytes.Buffer
	pc.SetOut(&pubOut)
	if err := runPublish(pc, []string{resolved.Name}); err != nil {
		return err
	}
	var pub struct {
		RFCURL   string `json:"rfc_url"`
		Children []struct {
			URL string `json:"url"`
		} `json:"children"`
	}
	if err := json.Unmarshal(pubOut.Bytes(), &pub); err == nil {
		res.RFCURL = pub.RFCURL
		for _, c := range pub.Children {
			if c.URL != "" {
				res.Children = append(res.Children, c.URL)
			}
		}
	}
	if format == publishFormatJSON {
		out := map[string]any{}
		_ = json.Unmarshal(pubOut.Bytes(), &out)
		out["voice"] = res
		return output.Write(cmd.OutOrStdout(), out, globalFields)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Spec %s written to %s from %s (%s)\n", resolved.Name, res.SpecPath, from, note.URI)
	if res.RFCURL != "" {
		fmt.Fprintf(w, "RFC: %s\n", res.RFCURL)
	}
	for _, c := range res.Children {
		fmt.Fprintf(w, "  child: %s\n", c)
	}
	return nil
}

// requirementTitles lists the bold titles of a drafted body's requirements:
// the child issues publish would file.
func requirementTitles(body string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(line[3:]) == "Requirements"
			continue
		}
		if !in || !strings.HasPrefix(line, "- [ ] **") {
			continue
		}
		rest := strings.TrimPrefix(line, "- [ ] **")
		if i := strings.Index(rest, "**"); i > 0 {
			out = append(out, rest[:i])
		}
	}
	return out
}

// writeVoiceMaterial stages the note, transcript and drafted spec where the
// agent and the spec workflow can read them.
func writeVoiceMaterial(root, dir string, note voice.Note, draft voice.Draft) error {
	abs := filepath.Join(root, dir)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"note.md":       note.Summary,
		"transcript.md": note.Transcript,
		"draft.md":      draft.Body,
	}
	for name, body := range files {
		if strings.TrimSpace(body) == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(abs, name), []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	voiceCmd.Flags().String("from", "", "Voice source: "+strings.Join(voice.SourceNames, ", "))
	voiceCmd.Flags().Bool("latest", false, "Take the most recent note")
	voiceCmd.Flags().String("title", "", "Take the newest note whose title contains this text")
	voiceCmd.Flags().String("path", "", "File or folder to read (file source), or a flow.sqlite/export folder (wisprflow)")
	voiceCmd.Flags().String("name", "", "Spec name; defaults to a slug of the note's title")
	voiceCmd.Flags().Bool("publish", false, "Write the drafted spec as final and file it as an RFC with child issues")
	voiceCmd.Flags().BoolP("dry-run", "n", false, "Fetch and draft, but write nothing and make no network calls")
	voiceCmd.Flags().String("repo", "", "With --publish: GitHub owner/repo to file in")
	voiceCmd.Flags().String("target", "", "With --publish: work source (github, jira, linear)")
	voiceCmd.Flags().String("format", publishFormatPretty, "Output format: pretty or json")
	voiceCmd.Flags().String("command", "", "MCP server command for MCP-backed sources (plaud default: "+voice.DefaultPlaudCommand+")")
	rootCmd.AddCommand(voiceCmd)
}
