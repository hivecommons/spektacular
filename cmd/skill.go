package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/templates"
	"github.com/spf13/cobra"
)

// SkillResult is returned by the skill command.
type SkillResult struct {
	Name         string `json:"name"`
	Title        string `json:"title"`
	Instructions string `json:"instructions"`
}

var skillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Fetch a skill definition from the Spektacular Skill Library",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkill,
}

func runSkill(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name": {Type: "string"},
				},
				Required: []string{"name"},
			},
			Output: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name":         {Type: "string"},
					"title":        {Type: "string"},
					"instructions": {Type: "string"},
				},
			},
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	name := args[0]
	filename := "skills/skill_" + name + ".md"

	content, err := templates.FS.ReadFile(filename)
	if err != nil {
		// List available skills for the error message.
		available := listSkills()
		return fmt.Errorf("unknown skill %q — available skills: %s", name, strings.Join(available, ", "))
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(SkillResult{
		Name:         name,
		Title:        skillTitle(name),
		Instructions: string(content),
	})
}

// listSkills returns the names of all embedded skills.
func listSkills() []string {
	entries, err := templates.FS.ReadDir("skills")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "skill_") && strings.HasSuffix(n, ".md") {
			names = append(names, n[6:len(n)-3])
		}
	}
	return names
}

// skillListCmd lists all available skills.
var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available skills",
	RunE: func(cmd *cobra.Command, _ []string) error {
		names := listSkills()
		data, _ := json.MarshalIndent(map[string][]string{"skills": names}, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	},
}

func skillTitle(name string) string {
	words := strings.Split(name, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func init() {
	skillCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema and exit")
	skillCmd.AddCommand(skillListCmd)
}
