#!/bin/bash
# Builds the goal-check fixture: one final spec and plan whose single task has
# three acceptance criteria, the task's work already in the working tree, and
# the implement workflow walked past `verify` to `check_goal`, so the agent
# resumes at the goal check itself and its reviewer is the first to judge the
# criteria, not the agent's own verification. Everything is written through
# the CLI of the binary under test, so the fixture always meets the current
# store contracts.
#
# The scenario (seed/scenario, rendered by the Makefile) decides the README:
#   unmet - the work implements criteria 1 and 2 only: the README's Usage
#           section has no `todo count` line, while every test passes.
#   met   - the work meets all three criteria.
set -euo pipefail

SEED=/tmp/seed

# set_log_level rewrites log.level in the project's config.yaml. Seeding runs
# with logging off, so the logs hold only the agent's run, and the last step
# turns it back to the default level (standard), which records everything
# the verifiers and tools read from .spektacular/logs/ with no special
# setting. The edit
# fails the image build, not the run, if the config has no log.level line.
set_log_level() {
  python3 - /app/.spektacular/config.yaml "$1" <<'PY'
import re
import sys

path, level = sys.argv[1], sys.argv[2]
lines = open(path).read().split("\n")
in_log = False
for i, line in enumerate(lines):
    if re.match(r"\S", line):
        in_log = line.rstrip() == "log:"
    elif in_log and re.match(r"^ +level:", line):
        lines[i] = re.sub(r"level:.*", "level: " + level, line)
        break
else:
    sys.exit("the seeded config has no log.level line")
open(path, "w").write("\n".join(lines))
PY
}

SCENARIO=$(cat "$SEED/scenario")
AGENT=$(cat "$SEED/agent")
PLAN=20260301000000-todo-count

case "$SCENARIO" in
  unmet|met) ;;
  *) echo "unknown scenario: $SCENARIO" >&2; exit 1 ;;
esac

# init runs here, at build time, not in the agent's session: an agent loads
# its helpers (Claude's .claude/agents/, for one) only when a session starts,
# so a spek-goal-reviewer installed partway through a run is never found.
# init names the project and its repo after the directory, app; the plan's
# task names todo-app, so both are renamed. implement.worktrees is turned off
# so the run works in /app itself.
cd /app
# init is never logged: a command is logged only where a project config
# already exists when it starts, and none does yet. Logging is then switched
# off for the rest of the seed.
spektacular init "$AGENT" >/dev/null
set_log_level off
sed -i -e 's/^name: app$/name: todo-app/' -e 's/^    - name: app$/    - name: todo-app/' .spektacular/config.yaml
grep -q '^    - name: todo-app$' .spektacular/config.yaml || { echo "the seeded config does not register todo-app" >&2; exit 1; }
mkdir -p .spektacular/tmp
python3 - /app/.spektacular/config.yaml <<'PY'
import sys
path = sys.argv[1]
text = open(path).read()
old = "implement:\n    worktrees: true\n"
assert old in text, "the seeded config has no implement.worktrees"
open(path, "w").write(text.replace(old, "implement:\n    worktrees: false\n", 1))
PY

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

# Walk the implement workflow to check_goal: the agent resumes there.
spektacular implement new --data "{\"name\":\"$PLAN\"}" >/dev/null
for step in analyze implement test verify check_goal; do
  spektacular implement goto --data "{\"step\":\"$step\",\"name\":\"$PLAN\"}" >/dev/null
done

# Fail the image build, not the agent run, if the fixture does not start
# where the task assumes.
python3 -m unittest discover -s tests >/dev/null 2>&1 || { echo "the seeded work's tests fail" >&2; exit 1; }
step=$(jq -r .current_step .spektacular/state.json)
if [ "$step" != check_goal ]; then
  echo "the seeded implement run is at $step, not check_goal" >&2
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

# Fail the image build, not the run, if any seed command reached the log:
# the agent must start with an empty session log.
if compgen -G "/app/.spektacular/logs/*.jsonl" >/dev/null; then
  echo "the seed's own spektacular commands were logged under /app/.spektacular/logs" >&2
  exit 1
fi
set_log_level standard
