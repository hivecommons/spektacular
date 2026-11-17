---
tags: [linear, publish, schema, search]
---

# Linear project Markdown lives in content

Linear's Project `description` is a short summary. Store the full rendered RFC,
its digest and publication marker in `ProjectCreateInput.content` and
`ProjectUpdateInput.content`; read back `Project.content`. Issue descriptions
continue to use `description`. Project issue membership uses `projectId`, while
sub-issues use `parentId`, and project labels are separate from issue labels.

The public ProjectFilter has no description/content comparator. To recover a
publication without relying on frontmatter, page projects with
`accessibleTeams: { some: { key: { eqIgnoreCase: $team } } }`, include archived
projects, and parse their content markers locally. Avoid the internal
`searchableContent` filter. See Linear's official schema:
https://github.com/linear/linear/blob/master/packages/sdk/src/schema.graphql
