#!/bin/bash
# Builds the epic the task implements: four final specs, a final plan for
# each, and an epic linking them, written through the CLI of the binary
# under test so the fixture always meets the current store contracts instead
# of drifting from them. A plan that breaks the task format fails here.
set -euo pipefail

SEED=/tmp/seed

# set_log_level rewrites log.level in the project's config.yaml. Seeding runs
# with logging off, so the session log holds only the agent's run, and the
# last step turns it to verbose, which keeps the responses and state
# snapshots the verifiers and tools read from .spektacular/logs/. The edit
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

# The CLI needs a project config before it will write anything, and the
# agent's own `spektacular init <agent>` must stay the step that installs its
# skills. So take the config a throwaway init writes, which is always in the
# format this binary expects, and nothing else. The agent is left unset, so
# the session log does not label a Bob or Codex run as Claude's.
mkdir -p /tmp/todo-app
(cd /tmp/todo-app && spektacular init claude >/dev/null)
mkdir -p /app/.spektacular/tmp
cp /tmp/todo-app/.spektacular/config.yaml /app/.spektacular/config.yaml
set_log_level off
rm -rf /tmp/todo-app
sed -i 's/^agent: .*/agent: ""/' /app/.spektacular/config.yaml

cd /app

stage() { cp "$1" .spektacular/tmp/staged.md; }

for path in "$SEED"/specs/*.md; do
  name=$(basename "$path" .md)
  stage "$path"
  spektacular spec file write "$name" --document-status final --from .spektacular/tmp/staged.md >/dev/null
done

for dir in "$SEED"/plans/*/; do
  name=$(basename "$dir")
  for doc in plan context research; do
    stage "$dir/$doc.md"
    spektacular plan file write "$name" "$doc" --document-status final --from .spektacular/tmp/staged.md >/dev/null
  done
done

# storage and format are independent and run side by side; commands joins
# them; export builds on commands. Each spec writes its own files, so no two
# merges conflict. The epic's name is minted by the CLI, so the agent and the
# verifier find it with `spektacular epic list`: it is the project's only epic.
stage "$SEED/epic.md"
spektacular epic write todo --document-status final --from .spektacular/tmp/staged.md \
  --data '{"specs":[
    {"name":"20260201000001-storage","depends_on":[]},
    {"name":"20260201000002-format","depends_on":[]},
    {"name":"20260201000003-commands","depends_on":["20260201000001-storage","20260201000002-format"]},
    {"name":"20260201000004-export","depends_on":["20260201000003-commands"]}
  ]}' >/dev/null
rm .spektacular/tmp/staged.md

# Fail the image build, not the agent run, if the fixture does not start
# where the task assumes: every spec planned, two ready to implement, two
# waiting on them.
epic=$(spektacular epic list | jq -r '.files[0].name')
states=$(spektacular status "$epic" --format json \
  | jq -c '[.specs[] | {(.name): (.run.plan.state + "/" + .run.implement.state)}] | add')
expected='{"20260201000001-storage":"done/ready","20260201000002-format":"done/ready","20260201000003-commands":"done/blocked","20260201000004-export":"done/blocked"}'
if [ "$states" != "$expected" ]; then
  echo "seeded epic is not in its expected starting state: $states" >&2
  spektacular status "$epic" --format json | jq '.epic.run' >&2
  exit 1
fi

# Fail the image build, not the run, if any seed command reached the log:
# the agent must start with an empty session log.
if compgen -G "/app/.spektacular/logs/*.jsonl" >/dev/null; then
  echo "the seed's own spektacular commands were logged under /app/.spektacular/logs" >&2
  exit 1
fi
set_log_level verbose
