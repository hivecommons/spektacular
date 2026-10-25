package templates_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// Agents write durable knowledge themselves and then report it; nothing they
// are installed with tells them to offer it and wait for a decision. These are
// the phrases the old offer-then-wait flow used, banned from every source that
// installs agent instructions about capturing knowledge: the standing rules,
// the workflow steps and partials, and the two epic orchestrator skills. The
// spek-knowledge skill keeps its tag audit's per-entry confirmation, so it is
// checked by its own tests rather than here.
var bannedKnowledgeOfferPhrases = []string{
	"offer to save",
	"offer it now",
	"save it only if the user accepts",
	"knowledge candidates",
	"explicit acceptance",
	"not offered again",
	"worth saving to the knowledge base",
	"never write unprompted",
	"never write to the knowledge base unprompted",
	"never write to a knowledge source unprompted",
	"propose-then-confirm",
}

// knowledgeCaptureSources lists the template trees and files that install
// agent instructions about capturing knowledge.
var knowledgeCaptureSources = []string{
	"agents",
	"steps/plan",
	"steps/implement",
	"partials",
	"skills/workflows/spek-plan-epic",
	"skills/workflows/spek-implement-epic",
}

func TestKnowledgeCaptureSourcesCarryNoOfferWording(t *testing.T) {
	checked := 0
	for _, root := range knowledgeCaptureSources {
		err := fs.WalkDir(templates.FS, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			raw, err := fs.ReadFile(templates.FS, path)
			if err != nil {
				return err
			}
			body := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
			for _, phrase := range bannedKnowledgeOfferPhrases {
				require.NotContains(t, body, phrase, "%s still carries the knowledge-offer wording %q", path, phrase)
			}
			checked++
			return nil
		})
		require.NoError(t, err, root)
	}
	require.Greater(t, checked, 20, "the walk must reach the installed instruction sources")
}

// The rewritten instructions say what replaces the offer: write the entry,
// then tell the user which entry, where and why.
func TestKnowledgeCaptureSourcesSayWriteThenReport(t *testing.T) {
	for path, phrases := range map[string][]string{
		"agents/knowledge-trigger.md": {
			"write the entry yourself",
			"tell the user what you wrote: which entry (its path), where it lives (its tier and store), and why it is worth keeping",
		},
		"agents/short/knowledge-trigger.md": {
			"write it via `spek-knowledge` yourself, then tell the user which entry, where (tier, store, path) and why",
		},
		"agents/memory-context.md": {
			"Then tell the user which entry you wrote (its path), its tier and store, and why it is worth keeping.",
		},
		"partials/implement-knowledge-written.md": {
			"### Report the knowledge written",
		},
		"partials/orchestrated-stop.md": {
			"list each entry you wrote (tier, store, path, why) in your closing summary",
		},
		"skills/workflows/spek-plan-epic/SKILL.md": {
			"Report every knowledge entry the children wrote (tier, store, path, why)",
		},
		"skills/workflows/spek-implement-epic/SKILL.md": {
			"Report every knowledge entry the children wrote (tier, store, path, why)",
		},
	} {
		raw, err := fs.ReadFile(templates.FS, path)
		require.NoError(t, err, path)
		body := strings.Join(strings.Fields(string(raw)), " ")
		for _, phrase := range phrases {
			require.Contains(t, body, phrase, path)
		}
	}
}
