package autocommit

import (
	"testing"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

func TestValidateMessage(t *testing.T) {
	cases := []struct {
		name       string
		message    string
		specName   string
		milestones []int
		wantCode   string // empty ⇒ the message is valid
	}{
		{
			name:     "an empty message is rejected",
			message:  "",
			specName: "000057_git-commit",
			wantCode: "commit_message_required",
		},
		{
			name:     "a whitespace-only message is rejected",
			message:  "  \n\t ",
			specName: "000057_git-commit",
			wantCode: "commit_message_required",
		},
		{
			name:     "a message that does not name the spec is rejected",
			message:  "Add the commit engine\n\nIt commits things.\n",
			specName: "000057_git-commit",
			wantCode: "commit_message_invalid",
		},
		{
			name:     "a message naming the spec is accepted",
			message:  "Add the commit engine for 000057_git-commit\n",
			specName: "000057_git-commit",
		},
		{
			name:     "the spec name is matched case-sensitively",
			message:  "Add the commit engine for 000057_GIT-COMMIT\n",
			specName: "000057_git-commit",
			wantCode: "commit_message_invalid",
		},
		{
			name:       "a required milestone that is not named is rejected",
			message:    "Add the commit engine for 000057_git-commit\n",
			specName:   "000057_git-commit",
			milestones: []int{2},
			wantCode:   "commit_message_invalid",
		},
		{
			name:       "a milestone is matched case-insensitively",
			message:    "Add the commit engine for 000057_git-commit\n\nCompletes milestone 2.\n",
			specName:   "000057_git-commit",
			milestones: []int{2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMessage(tc.message, tc.specName, tc.milestones)
			if tc.wantCode == "" {
				require.NoError(t, err)
				return
			}
			var resp *output.ErrorResponse
			require.ErrorAs(t, err, &resp)
			require.Equal(t, tc.wantCode, resp.Code)
		})
	}
}

func TestPreWorkflowMessage(t *testing.T) {
	require.Equal(t,
		"Save uncommitted changes before implement workflow for 000057_git-commit\n\n"+
			"These are the user's changes from before the implement workflow for "+
			"000057_git-commit started. Spektacular committed them separately at the "+
			"user's request so they are not mixed with the agent's work.\n",
		PreWorkflowMessage("implement", "000057_git-commit"))
}
