#!/bin/bash
# Reference solution for the goal-check-workflow harbor task.
#
# It documents the CLI sequence a correct run follows from the seeded verify
# step and proves the verifier passes on it: stage the work, record the
# reviewer's verdicts, and act on the decision. In the unmet scenario the
# first reply judges the README criterion not_met, the run goes back to
# implement, the README line is added and the second check accepts. The
# verdicts here are scripted; the real test is a harbor run with an agent.
set -euo pipefail

cd /app
spektacular init claude >/dev/null
PLAN=20260301000000-todo-count
TASK=5d1e7a3c-2b4f-4c6d-8e9a-0b1c2d3e4f50
STAGE=.spektacular/tmp/$PLAN/goal-check

goto() { spektacular implement goto --data "{\"step\":\"$1\",\"name\":\"$PLAN\"}" >/dev/null; }

stage_work() {
  mkdir -p "$STAGE"
  git diff HEAD > "$STAGE/todo-app.diff"
  git status --porcelain --untracked-files=all > "$STAGE/todo-app.status"
}

# record <verdict for criterion 3>: criteria 1 and 2 are met.
record() {
  jq -n --arg v3 "$1" '{verdicts: [
    {criterion: 1, verdict: "met", evidence: "todo/cli.py: _count prints len(load()), registered as the count subcommand"},
    {criterion: 2, verdict: "met", evidence: "tests/test_count.py covers an empty list and a list of two"},
    {criterion: 3, verdict: $v3, evidence: (if $v3 == "met" then "README.md Usage section shows python3 -m todo count" else "README.md Usage section shows only todo add and todo list; no todo count line" end)}
  ]}' > "$STAGE/reply.json"
  spektacular implement goal record --data "{\"name\":\"$PLAN\",\"item\":\"$TASK\"}" --from "$STAGE/reply.json" >/dev/null
  rm -rf "$STAGE"
}

goto verify
goto check_goal
stage_work
if grep -q "todo count" README.md; then
  record met
else
  record not_met
  goto implement
  python3 - README.md <<'PY'
import sys
path = sys.argv[1]
text = open(path).read()
old = "    python3 -m todo list             # todo list: list the todos\n"
new = old + "    python3 -m todo count            # todo count: print how many todos there are\n"
open(path, "w").write(text.replace(old, new, 1))
PY
  for step in test verify check_goal; do goto "$step"; done
  stage_work
  record met
fi

goto update_plan
spektacular plan file read "$PLAN" plan \
  | sed -e 's/^#### - \[ \] Task: Add the count command/#### - [x] Task: Add the count command/' -e 's/^- \[ \] /- [x] /' \
  > .spektacular/tmp/plan.md
spektacular plan file write "$PLAN" plan --from .spektacular/tmp/plan.md >/dev/null
rm .spektacular/tmp/plan.md
goto update_changelog
goto test_plan
printf '# %s\n\nAdded the todo count command, tested and documented.\n' "$PLAN" > .spektacular/tmp/changelog.md
spektacular changelog file write "$PLAN" --from .spektacular/tmp/changelog.md >/dev/null
rm .spektacular/tmp/changelog.md
for step in update_feature_changelog reconcile_spec finished; do goto "$step"; done
