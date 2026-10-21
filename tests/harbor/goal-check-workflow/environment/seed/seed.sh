#!/bin/bash
# Builds the goal-check fixture: one final spec and plan whose single task has
# three acceptance criteria, the task's work already in the working tree, and
# the implement workflow walked to `verify`, so the agent resumes at the
# point the goal check follows. Everything is written through the CLI of the
# binary under test, so the fixture always meets the current store contracts.
#
# The scenario (seed/scenario, rendered by the Makefile) decides the README:
#   unmet - the work implements criteria 1 and 2 only: the README's Usage
#           section has no `todo count` line, while every test passes.
#   met   - the work meets all three criteria.
set -euo pipefail

SEED=/tmp/seed
SCENARIO=$(cat "$SEED/scenario")
PLAN=20260301000000-todo-count

case "$SCENARIO" in
  unmet|met) ;;
  *) echo "unknown scenario: $SCENARIO" >&2; exit 1 ;;
esac

# The project config a throwaway init writes, with no agent recorded and
# implement.worktrees off, so the run works in /app itself and the agent's own
# `spektacular init <agent>` stays the step that installs its skills.
mkdir -p /tmp/todo-app
(cd /tmp/todo-app && spektacular init claude >/dev/null)
mkdir -p /app/.spektacular/tmp
cp /tmp/todo-app/.spektacular/config.yaml /app/.spektacular/config.yaml
rm -rf /tmp/todo-app
sed -i 's/^agent: .*/agent: ""/' /app/.spektacular/config.yaml
python3 - /app/.spektacular/config.yaml <<'PY'
import sys
path = sys.argv[1]
text = open(path).read()
old = "implement:\n    worktrees: true\n"
assert old in text, "the seeded config has no implement.worktrees"
open(path, "w").write(text.replace(old, "implement:\n    worktrees: false\n", 1))
PY

cd /app
stage() { cp "$1" .spektacular/tmp/staged.md; }

stage "$SEED/spec.md"
spektacular spec file write "$PLAN" --document-status final --from .spektacular/tmp/staged.md >/dev/null
for doc in plan context research; do
  stage "$SEED/$doc.md"
  spektacular plan file write "$PLAN" "$doc" --document-status final --from .spektacular/tmp/staged.md >/dev/null
done
rm .spektacular/tmp/staged.md

# The task's work, left uncommitted so the staged diff shows it.
cp "$SEED/work/todo/cli.py" todo/cli.py
cp "$SEED/work/tests/test_count.py" tests/test_count.py
if [ "$SCENARIO" = met ]; then
  python3 - README.md <<'PY'
import sys
path = sys.argv[1]
text = open(path).read()
old = "    python3 -m todo list             # todo list: list the todos\n"
assert old in text
new = old + "    python3 -m todo count            # todo count: print how many todos there are\n"
open(path, "w").write(text.replace(old, new, 1))
PY
fi

# Walk the implement workflow to verify: the agent resumes there.
spektacular implement new --data "{\"name\":\"$PLAN\"}" >/dev/null
for step in analyze implement test verify; do
  spektacular implement goto --data "{\"step\":\"$step\",\"name\":\"$PLAN\"}" >/dev/null
done

# Fail the image build, not the agent run, if the fixture does not start
# where the task assumes.
python3 -m unittest discover -s tests >/dev/null 2>&1 || { echo "the seeded work's tests fail" >&2; exit 1; }
step=$(jq -r .current_step .spektacular/state.json)
if [ "$step" != verify ]; then
  echo "the seeded implement run is at $step, not verify" >&2
  exit 1
fi
state=$(spektacular status "$PLAN" --format json | jq -r --arg p "$PLAN" '.specs[] | select(.name == $p) | .run.implement.state')
if [ "$state" != in_progress ]; then
  echo "the seeded plan's implement state is $state, not in_progress" >&2
  exit 1
fi
if [ "$SCENARIO" = unmet ] && grep -q "todo count" README.md; then
  echo "the unmet scenario's README already documents todo count" >&2
  exit 1
fi
