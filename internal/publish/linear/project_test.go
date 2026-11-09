package linear_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/linear"
	"github.com/hivecommons/spektacular/internal/publish/linear/lineartest"
)

func TestProject_CreateRepublishUpdateAndReconcile(t *testing.T) {
	f := lineartest.New(t)
	target := f.Target()
	target.ParentAs = linear.ParentProject
	target.Assignee = lineartest.UserEmail
	ctx := context.Background()
	p := testPublication(t)
	opts := publish.PublishOptions{ParentLabel: "epic", Labels: []string{"spek", "unknown"}}

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Nil(t, existing)
	res, err := target.Publish(ctx, p, existing, opts)
	require.NoError(t, err)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, "project-1", res.RFCKey)
	require.Equal(t, "https://linear.app/acme/project/project-1", res.RFCURL)
	require.Len(t, f.ProjectSnapshot(), 1)
	rfc := f.ProjectSnapshot()[0]
	require.Equal(t, p.RFC.Title, rfc.Name)
	require.Equal(t, lineartest.TeamID, rfc.TeamID)
	require.Contains(t, rfc.Content, "ENG-1 — Schedule")
	require.Contains(t, rfc.Content, "ENG-2 — Content")
	require.Len(t, res.Warnings, 1, "unknown child label skipped; project never receives issue labels")
	for _, is := range f.Snapshot() {
		require.Equal(t, rfc.ID, is.ProjectID)
		require.Empty(t, is.ParentID)
		require.Equal(t, []string{lineartest.LabelID("spek")}, is.LabelIDs)
		require.Equal(t, lineartest.UserID, is.AssigneeID)
		require.Equal(t, lineartest.StateID(lineartest.Todo), is.StateID, "children are in the state Hive reads")
		marker, ok := publish.ParseMarker(is.Description)
		require.True(t, ok)
		require.Equal(t, res.RFCURL, marker.RFC)
		require.NotEmpty(t, marker.Task)
		require.Contains(t, is.Description, "## Acceptance criteria")
	}
	require.Equal(t, []string{"ENG-1 blocks ENG-2"}, f.RelationList())

	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.Empty(t, publish.Diff(p, existing), "project content survives Markdown normalization")
	require.Equal(t, rfc.ID, existing.Children["R1"].Parent)
	res, err = target.Publish(ctx, p, existing, opts)
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.Empty(t, res.Linked)
	require.Equal(t, []string{"R1", "R2", "rfc"}, res.Unchanged)
	require.Equal(t, 1, f.Count("projectCreate"))
	require.Equal(t, 1, f.Count("projectUpdate"), "only the first checklist rewrite")
	require.Zero(t, f.Count("issueUpdate"))

	p.Children[1].AcceptanceCriteria = append(p.Children[1].AcceptanceCriteria, "Include every merged item exactly once.")
	res, err = target.Publish(ctx, p, existing, opts)
	require.NoError(t, err)
	require.Equal(t, []string{"R2"}, res.Updated)
	require.Equal(t, 1, f.Count("issueUpdate"), "update one child without touching the project")
	require.Equal(t, 1, f.Count("projectUpdate"))

	// A detached child is repaired independently of its content digest.
	f.Update("ENG-1", func(is *lineartest.Issue) { is.ProjectID = "" })
	existing, err = target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	res, err = target.Publish(ctx, p, existing, opts)
	require.NoError(t, err)
	require.Empty(t, res.Updated)
	require.Equal(t, []string{"ENG-1 under project-1"}, res.Linked)
	require.Equal(t, rfc.ID, f.Snapshot()[0].ProjectID)

	p.RFC.Title = "RFC: Improved digest"
	res, err = target.Publish(ctx, p, existing, opts)
	require.NoError(t, err)
	require.Equal(t, []string{"rfc"}, res.Updated)
	require.Equal(t, p.RFC.Title, f.ProjectSnapshot()[0].Name)
}

func TestProject_SearchPagesAndIncludesArchived(t *testing.T) {
	f := lineartest.New(t)
	// First project has the wrong artifact and the second belongs to another team.
	f.Projects = append(f.Projects,
		&lineartest.Project{ID: "other", TeamID: lineartest.TeamID, Content: "unrelated"},
		&lineartest.Project{ID: "other-team", TeamID: "another-team", Content: "unrelated"})
	target := f.Target()
	target.ParentAs = linear.ParentProject
	target.TeamKey = "eng"
	target.PageSize = 1
	ctx := context.Background()
	p := testPublication(t)
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	rfc := f.ProjectSnapshot()[2]
	f.UpdateProject(rfc.ID, func(p *lineartest.Project) { p.Archived, p.StatusType = true, "completed" })
	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing.RFC)
	require.True(t, existing.RFC.Closed)
	require.Equal(t, rfc.ID, existing.RFC.Key)
	require.Len(t, existing.Children, 2)
	require.Equal(t, 2, f.Count("projects"), "only projects from the configured team are paged")
}

func TestProject_RefusesUnknownSettingsBeforeMutation(t *testing.T) {
	for _, setting := range []string{"team", "state", "assignee", "parent_as"} {
		t.Run(setting, func(t *testing.T) {
			f := lineartest.New(t)
			target := f.Target()
			target.ParentAs = linear.ParentProject
			switch setting {
			case "team":
				target.TeamKey = "NOPE"
			case "state":
				target.State = "No such state"
			case "assignee":
				target.Assignee = "missing@acme.test"
			case "parent_as":
				target.ParentAs = "epic"
			}
			_, err := target.Publish(context.Background(), testPublication(t), nil, publish.PublishOptions{})
			require.Error(t, err)
			require.Zero(t, f.Count("projectCreate"))
			require.Zero(t, f.Count("issueCreate"))
		})
	}
}

func TestProject_ChangingParentModeRefusesDuplicateRFC(t *testing.T) {
	for _, mode := range []string{linear.ParentIssue, linear.ParentProject} {
		t.Run(mode, func(t *testing.T) {
			f := lineartest.New(t)
			target := f.Target()
			target.ParentAs = mode
			p := testPublication(t)
			_, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
			require.NoError(t, err)
			if mode == linear.ParentIssue {
				target.ParentAs = linear.ParentProject
			} else {
				target.ParentAs = linear.ParentIssue
			}
			_, err = target.Find(context.Background(), p.ArtifactID)
			require.ErrorContains(t, err, "already published as")
			require.ErrorContains(t, err, "parent_as: "+mode)
		})
	}
}

func TestProject_FailedCreateAndChecklistUpdateCanBeRetried(t *testing.T) {
	for _, op := range []string{"projectCreate", "projectUpdate"} {
		t.Run(op, func(t *testing.T) {
			f := lineartest.New(t)
			target := f.Target()
			target.ParentAs = linear.ParentProject
			f.SetFailure(op)
			p := testPublication(t)
			res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
			require.ErrorContains(t, err, op+" refused")
			if op == "projectCreate" {
				require.Empty(t, res.RFCURL)
				require.Zero(t, f.Count("issueCreate"))
			} else {
				require.NotEmpty(t, res.RFCURL)
			}
			f.SetFailure("")
			existing, err := target.Find(context.Background(), p.ArtifactID)
			require.NoError(t, err)
			_, err = target.Publish(context.Background(), p, existing, publish.PublishOptions{})
			require.NoError(t, err)
			require.Len(t, f.ProjectSnapshot(), 1)
			require.Len(t, f.Snapshot(), 2)
			require.True(t, strings.HasSuffix(strings.TrimSpace(f.ProjectSnapshot()[0].Content), "target=linear -->"))
		})
	}
}
