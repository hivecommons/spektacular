{{#has_worktree_roots}}
### Where the code lives

This spec is built in its own worktrees. Each repo's code lives here:

{{#worktree_roots}}
- **{{repo}}**: `{{{root}}}`
{{/worktree_roots}}

Every code edit, build and check happens in these locations, never in a main checkout. Give every sub-agent you launch (task implementers, test authors, verifiers) these exact locations, and tell it to work only in them. Run `{{config.command}}` itself from the project root, not from a worktree.

A new worktree holds only tracked files: ignored dependency folders such as `node_modules` are not there. Prepare dependencies inside the worktree (a repo's declared setup command has already run). Never install into, symlink from or share dependencies with a main checkout.
{{/has_worktree_roots}}
{{^has_worktree_roots}}
### Where the code lives

Run `{{config.command}} repo list`. Each repo's code lives at its `root`. Give every sub-agent you launch those exact locations for the repos its work touches, and tell it to work only there.
{{/has_worktree_roots}}
