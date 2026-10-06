# Working context: plan 000064_epic-worktree-store-isolation

- Planning the spec written earlier this session (user invoked /spek-plan right after it).
- Spec decisions (user): CLI always run from main project; worktree .spektacular
  never touched; implement step output gives child its worktree code roots;
  child runs standard spek-implement unchanged; epic merge refuses .spektacular changes.
- User global rule: never commit unless asked; user approved auto_commit for the
  spec workflow ("commit away"). Plan workflow auto-commit: ask again at finish.
- Discovery done. Key design: per-spec worktree record in main `.spektacular/worktrees/<spec>/`;
  cmd feeds code roots into implement steps (no git); auto-commit split (worktree code / path-scoped
  main artifacts under lock); status + finishedIn read main; merge guard on `.spektacular/` diffs.
