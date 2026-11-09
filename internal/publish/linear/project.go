package linear

import (
	"context"
	"errors"
	"fmt"

	"github.com/hivecommons/spektacular/internal/publish"
)

func (t *Target) checkParentAs() error {
	switch t.ParentAs {
	case "", ParentIssue, ParentProject:
		return nil
	default:
		return fmt.Errorf("publish.linear.parent_as must be issue or project, not %q", t.ParentAs)
	}
}

func (t *Target) parentField() string {
	if t.ParentAs == ParentProject {
		return "projectId"
	}
	return "parentId"
}

type projectNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Status  struct {
		Type string `json:"type"`
	} `json:"status"`
}

const projectFields = `id name url content status { type }`

// ProjectFilter has no public description/content comparator. Page through
// this team's projects and inspect their markers, including archived projects
// so a completed RFC cannot accidentally be recreated. Content is Linear's
// full Markdown body; description is only its short summary.
const findProjectsQuery = `query($team: String!, $first: Int!, $after: String) {
  projects(filter: { accessibleTeams: { some: { key: { eqIgnoreCase: $team } } } }, first: $first, after: $after, includeArchived: true) {
    nodes { ` + projectFields + ` }
    pageInfo { hasNextPage endCursor }
  }
}`

func (t *Target) findProject(ctx context.Context, artifactID string) (*publish.Filed, error) {
	var rfc *publish.Filed
	var after any
	for {
		var out struct {
			Projects struct {
				Nodes    []projectNode `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"projects"`
		}
		if err := t.query(ctx, findProjectsQuery, map[string]any{"team": t.TeamKey, "first": t.pageSize(), "after": after}, &out); err != nil {
			return nil, fmt.Errorf("searching team %s for published projects: %w", t.TeamKey, err)
		}
		for _, n := range out.Projects.Nodes {
			m, ok := publish.ParseMarker(n.Content)
			if !ok || !m.IsRFC() || m.ArtifactID != artifactID {
				continue
			}
			f := filedProject(n, m)
			if rfc == nil || (rfc.Closed && !f.Closed) {
				rfc = &f
			}
		}
		if !out.Projects.PageInfo.HasNextPage || out.Projects.PageInfo.EndCursor == "" {
			return rfc, nil
		}
		after = out.Projects.PageInfo.EndCursor
	}
}

func filedProject(n projectNode, m publish.Marker) publish.Filed {
	issue := issueNode{ID: n.ID, Identifier: n.ID, URL: n.URL, Title: n.Name, Description: n.Content}
	issue.State.Type = n.Status.Type
	return filed(issue, m)
}

const createProjectMutation = `mutation($input: ProjectCreateInput!) {
  projectCreate(input: $input) { success project { ` + projectFields + ` } }
}`

const updateProjectMutation = `mutation($id: String!, $input: ProjectUpdateInput!) {
  projectUpdate(id: $id, input: $input) { success }
}`

func (t *Target) createProject(ctx context.Context, teamID, title, body string) (publish.Filed, error) {
	var out struct {
		ProjectCreate struct {
			Success bool        `json:"success"`
			Project projectNode `json:"project"`
		} `json:"projectCreate"`
	}
	input := map[string]any{"teamIds": []string{teamID}, "name": title, "content": description(body)}
	if err := t.query(ctx, createProjectMutation, map[string]any{"input": input}, &out); err != nil {
		return publish.Filed{}, err
	}
	if !out.ProjectCreate.Success || out.ProjectCreate.Project.ID == "" {
		return publish.Filed{}, errors.New("projectCreate reported no project")
	}
	m, _ := publish.ParseMarker(body)
	f := filedProject(out.ProjectCreate.Project, m)
	f.Body, f.BodyDigest = "", publish.Digest(body)
	return f, nil
}

func (t *Target) updateRFC(ctx context.Context, id, title, body string) error {
	if t.ParentAs != ParentProject {
		return t.updateIssue(ctx, id, title, body)
	}
	var out struct {
		ProjectUpdate struct {
			Success bool `json:"success"`
		} `json:"projectUpdate"`
	}
	input := map[string]any{"name": title, "content": description(body)}
	if err := t.query(ctx, updateProjectMutation, map[string]any{"id": id, "input": input}, &out); err != nil {
		return err
	}
	if !out.ProjectUpdate.Success {
		return errors.New("projectUpdate reported failure")
	}
	return nil
}
