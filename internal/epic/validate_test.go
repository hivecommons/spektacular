package epic

import (
	"errors"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidGraphPasses(t *testing.T) {
	require.NoError(t, Validate([]EpicSpec{
		{Name: "auth", DependsOn: []string{}},
		{Name: "billing", DependsOn: []string{"auth"}},
		{Name: "reports", DependsOn: []string{"auth", "billing"}},
	}))
}

func TestValidate_ParallelWithAnotherMemberPasses(t *testing.T) {
	require.NoError(t, Validate([]EpicSpec{
		{Name: "auth", DependsOn: []string{}},
		{Name: "billing", DependsOn: []string{}, ParallelWith: []string{"auth"}},
	}))
}

func TestValidate_EmptyListPasses(t *testing.T) {
	require.NoError(t, Validate(nil))
	require.NoError(t, Validate([]EpicSpec{}))
}

func TestValidate_RefusesEachRule(t *testing.T) {
	cases := []struct {
		name         string
		specs        []EpicSpec
		wantResource string
		wantMessage  []string // each appears in the message
	}{
		{
			name: "missing name",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "", DependsOn: []string{}},
			},
			wantResource: "specs[1]",
			wantMessage:  []string{"entry 2", "no name"},
		},
		{
			name: "missing dependency list",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing"},
			},
			wantResource: "billing",
			wantMessage:  []string{`"billing"`, "no depends_on list"},
		},
		{
			name: "duplicate spec",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: []string{"auth"}},
				{Name: "auth", DependsOn: []string{}},
			},
			wantResource: "auth",
			wantMessage:  []string{`"auth"`, "more than once"},
		},
		{
			name: "unknown dependency",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: []string{"payments"}},
			},
			wantResource: "billing",
			wantMessage:  []string{`"billing"`, `"payments"`, "not a spec in this epic"},
		},
		{
			name: "cycle",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: []string{"reports"}},
				{Name: "reports", DependsOn: []string{"billing"}},
			},
			wantResource: "billing",
			wantMessage:  []string{`"billing"`, "dependency cycle", "billing -> reports -> billing"},
		},
		{
			name: "self dependency",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{"auth"}},
			},
			wantResource: "auth",
			wantMessage:  []string{`"auth"`, "dependency cycle", "auth -> auth"},
		},
		{
			name: "parallel_with names a spec outside the epic",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: []string{}, ParallelWith: []string{"payments"}},
			},
			wantResource: "billing",
			wantMessage:  []string{`"billing"`, `"payments"`, "parallel_with", "not a spec in this epic"},
		},
		{
			name: "parallel_with names the spec itself",
			specs: []EpicSpec{
				{Name: "auth", DependsOn: []string{}},
				{Name: "billing", DependsOn: []string{}, ParallelWith: []string{"billing"}},
			},
			wantResource: "billing",
			wantMessage:  []string{`"billing"`, "names itself in parallel_with"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.specs)
			require.Error(t, err)
			var er *output.ErrorResponse
			require.True(t, errors.As(err, &er), "want *output.ErrorResponse, got %T", err)
			require.Equal(t, "epic_invalid", er.Code)
			require.Equal(t, tc.wantResource, er.Resource)
			for _, want := range tc.wantMessage {
				require.Contains(t, er.Message, want)
			}
			require.Contains(t, er.NextAction, "specs list")
		})
	}
}
