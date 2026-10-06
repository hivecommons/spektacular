# Working context: implement 000064_epic-worktree-store-isolation

- User said "run this until you are done" and left (walking dog): drive the whole
  implement workflow autonomously, no check-ins. Prefer adapting over stopping.
- User global rule: never commit unless asked. Project has auto_commit: full and the
  CLI refuses milestone transitions without a message; decision: let the workflow's
  local milestone commits happen (never push) and flag this in the final report.
- read_plan: structure ok, no drift found, spec fully covered, no Changelog yet (first-task mode).
- Repo roots: spektacular = /home/nicj/code/github.com/hivecommons/spektacular;
  docs = /home/nicj/code/github.com/hivecommons/spektacular-website.
- `go test ./...` fails on a root-owned dir under tests/harbor/jobs; use
  `go test -shuffle=on . ./cmd/... ./internal/... ./templates/...`.
- DESIGN DECISION (deviation, record in changelog): worktree Record maps repo name ->
  *code root* (LocalSource re-rooted), not the .spektacular location the overlay held.
  Reason: spec constraint "worktree .spektacular never read or written"; with code
  roots nobody reads repo.yaml inside a worktree. Consequence for Task 3: the
  spec-scoped repo constructor overrides code sources (LocalSource/Resolve.Source),
  call it `repo.NewWithCodeRoots`, and autocommit `TargetsWithCodeRoots`.
- Record at <project>/.spektacular/worktrees/<spec>/record.json; worktree.ReadRecord(root, spec).
- Helpers in session scratchpad: tick.sh "<task title>" (ticks via CLI), addlog.sh <entry>.
- Task 1 done (record). Next: Task 2 merge guard.
