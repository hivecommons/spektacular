package agentusage

import (
	"encoding/json"
	"os"
)

// rawMeta is a Claude Code helper's agent-<id>.meta.json.
type rawMeta struct {
	ToolUseID     string `json:"toolUseId"`
	ParentAgentID string `json:"parentAgentId"`
	Description   string `json:"description"`
}

// readMeta fills a helper transcript's launch link, parent helper and
// description from its metadata file. A missing or unreadable file leaves
// them empty: the transcript is still read.
func readMeta(path string, t *Transcript) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var m rawMeta
	if json.Unmarshal(data, &m) != nil {
		return
	}
	t.LaunchID, t.ParentID, t.Description = m.ToolUseID, m.ParentAgentID, m.Description
}
