package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// An epic's planning summary is kept in the epic store, in a folder named
// after the epic: <epics>/<epic>/summary.md. `epic list` and ID allocation
// both ignore the folder, and `epic delete` removes it with the epic. It is
// written one section at a time, so repeating "plan this epic" or a review
// edit to one plan never disturbs the other sections.

var epicSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Read and write the planning summary kept with an epic",
	RunE:  runUnknownSubcommand,
}

var epicSummaryReadCmd = &cobra.Command{
	Use:   "read <epic>",
	Short: "Read an epic's planning summary, writing its bytes to stdout unchanged",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicSummaryRead,
}

var epicSummaryWriteCmd = &cobra.Command{
	Use:   "write <epic>",
	Short: "Replace one section of an epic's planning summary: the decisions, or one member spec's section",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicSummaryWrite,
}

type epicSummaryWriteInput struct {
	Section string `json:"section"`
}

var epicSummaryWriteInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"section": {Type: "string", Description: `"decisions", or the name of a spec in the epic`},
	},
}

var epicSummaryWriteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"epic":     {Type: "string"},
		"section":  {Type: "string"},
		"sections": {Type: "array", Items: &schemaProp{Type: "string"}, Description: "every section in the order the summary renders them"},
	},
}

var epicSummaryReadOutputSchema = &schemaObj{
	Type:       "object",
	Properties: map[string]*schemaProp{"content": {Type: "string", Description: "the summary's raw bytes, written to stdout unchanged"}},
}

// summaryDir is the folder kept beside the named epic for its summary.
func summaryDir(cfg config.Config, name string) string {
	return artifact.FeatureDir(cfg.Epic.Config.Directory, name)
}

// summaryPath is where the named epic's planning summary is stored.
func summaryPath(cfg config.Config, name string) string {
	return summaryDir(cfg, name) + "/summary.md"
}

// readSummary reads and parses an epic's summary; an epic with none reads as
// an empty summary, reported through found.
func readSummary(st store.Store, cfg config.Config, name string) (s epic.Summary, found bool, err error) {
	raw, err := st.Read(summaryPath(cfg, name))
	if errors.Is(err, store.ErrNotFound) {
		return epic.Summary{}, false, nil
	}
	if err != nil {
		return epic.Summary{}, false, err
	}
	s, err = epic.ParseSummary(raw)
	if err != nil {
		return epic.Summary{}, false, output.NewError("epic_summary_malformed", fmt.Sprintf("the planning summary of epic %q cannot be read: %v", name, err)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("repair the summary's frontmatter so `%s epic summary read %s` parses, then reissue", cfg.Command, name))
	}
	return s, true, nil
}

// writeSummary stamps the created date on a first write and writes the
// summary inside t, rendered in the epic's list order.
func writeSummary(t *docTxn, cfg config.Config, name string, s epic.Summary, order []string) error {
	if s.CreatedDate == "" {
		s.CreatedDate = time.Now().UTC().Format(metadata.DateFormat)
	}
	return t.write(summaryPath(cfg, name), s.Render(name, order))
}

// appendOrdering adds lines to the summary's "Order added for shared files"
// log inside t, creating the summary when the epic has none. Only `epic order`
// writes that section.
func appendOrdering(t *docTxn, cfg config.Config, epicName string, order []string, lines []string) error {
	s, _, err := readSummary(t.st, cfg, epicName)
	if err != nil {
		return err
	}
	all := append(strings.Split(s.Ordering, "\n"), lines...)
	if s.Ordering == "" {
		all = lines
	}
	s.Ordering = strings.Join(all, "\n")
	return writeSummary(t, cfg, epicName, s, order)
}

func runEpicSummaryRead(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: epicSummaryReadOutputSchema}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	if _, _, err := readEpic(cfg, st, name); err != nil {
		return err
	}
	raw, err := st.Read(summaryPath(cfg, name))
	if errors.Is(err, store.ErrNotFound) {
		return output.NewError("epic_summary_not_found", fmt.Sprintf("epic %q has no planning summary yet", name)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("plan the epic with the spek-plan-epic skill (\"plan this epic\"), which writes its summary, or write a section with `%s epic summary write %s --data '{\"section\":\"decisions\"}' --from <path>`", cfg.Command, name))
	}
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(raw)
	return err
}

func runEpicSummaryWrite(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  epicSummaryWriteInputSchema,
			Output: epicSummaryWriteOutputSchema,
			Flags:  map[string]*schemaProp{"from": {Type: "string"}},
		}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	fromPath, _ := cmd.Flags().GetString("from")
	if fromPath == "" {
		return output.NewError("epic_from_required",
			"--from is required: a summary section is read from a file, never from prose on the command line").
			WithNextAction(fmt.Sprintf("stage the section body under .spektacular/tmp/ and reissue `%s epic summary write %s --data '{\"section\":\"decisions\"}' --from <that file>`", cfg.Command, name))
	}
	var input epicSummaryWriteInput
	if dataStr, _ := cmd.Flags().GetString("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return output.NewError("bad_input", fmt.Sprintf("parsing --data: %v", err)).
				WithNextAction(`reissue with --data as a JSON object naming the section, e.g. '{"section":"decisions"}'`)
		}
	}
	if input.Section == "" {
		return output.NewError("bad_input", "--data must name the section to write").
			WithNextAction(`reissue with --data '{"section":"decisions"}' or '{"section":"<spec in the epic>"}'`)
	}
	current, _, err := readEpic(cfg, st, name)
	if err != nil {
		return err
	}
	order := current.SpecNames()
	validSections := fmt.Sprintf("write %q or one of the epic's specs (%s); the %q section is written only by `%s epic order`",
		epic.SectionDecisions, strings.Join(order, ", "), epic.SectionOrdering, cfg.Command)
	if _, member := current.Member(input.Section); input.Section != epic.SectionDecisions && !member {
		reason := fmt.Sprintf("%q is not a section of epic %q's summary", input.Section, name)
		if input.Section == epic.SectionOrdering {
			reason = fmt.Sprintf("the %q section of epic %q's summary is written only by `%s epic order`", epic.SectionOrdering, name, cfg.Command)
		}
		return output.NewError("epic_summary_section_invalid", reason).
			WithResource(input.Section).
			WithNextAction(validSections)
	}
	content, err := os.ReadFile(fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	body := strings.Trim(string(stripLeadingFrontmatterBlocks(content)), "\n")
	if err := epic.ValidSectionBody(body); err != nil {
		return output.NewError("epic_summary_section_invalid", fmt.Sprintf("the body for section %q cannot go in the summary: %v", input.Section, err)).
			WithResource(input.Section).
			WithNextAction("demote the body's # and ## headings to ### or deeper, then reissue the same command")
	}

	s, _, err := readSummary(st, cfg, name)
	if err != nil {
		return err
	}
	if input.Section == epic.SectionDecisions {
		s.Decisions = body
	} else {
		s.SetSection(input.Section, body)
	}
	t := newDocTxn(st)
	if err := writeSummary(t, cfg, name, s, order); err != nil {
		return t.fail(err)
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"epic":     name,
		"section":  input.Section,
		"sections": s.SectionNames(order),
	})
}

func init() {
	epicSummaryWriteCmd.Flags().StringP("data", "d", "", `JSON input naming the section (e.g. '{"section":"decisions"}')`)
	epicSummaryWriteCmd.Flags().String("from", "", "Read the section's body from the file at <path> (relative to cwd)")
	epicSummaryCmd.AddCommand(epicSummaryReadCmd, epicSummaryWriteCmd)
	epicCmd.AddCommand(epicSummaryCmd)
}
