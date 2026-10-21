#!/bin/bash
# Builds the epic the task plans: three final specs and an epic linking them,
# written through the CLI of the binary under test so the fixture always
# meets the current store contracts instead of drifting from them.
set -euo pipefail

SEED=/tmp/seed

# The CLI needs a project config before it will write anything, and the
# agent's own `spektacular init <agent>` must stay the step that installs its
# skills. So take the config a throwaway init writes, which is always in the
# format this binary expects, and nothing else. The agent's init later keeps
# this config and sets its own agent in it.
mkdir -p /tmp/todo-app
(cd /tmp/todo-app && spektacular init claude >/dev/null)
mkdir -p /app/.spektacular/tmp
cp /tmp/todo-app/.spektacular/config.yaml /app/.spektacular/config.yaml
rm -rf /tmp/todo-app
# Leave the agent unset until the agent's init: the session log names its
# files after the configured agent, and the agent's own `init` is the first
# call it logs, so a seeded "claude" would label a Bob or Codex run's shared
# log as Claude's.
sed -i 's/^agent: .*/agent: ""/' /app/.spektacular/config.yaml

cd /app

for path in "$SEED"/specs/*.md; do
  name=$(basename "$path" .md)
  cp "$path" ".spektacular/tmp/$name.md"
  spektacular spec file write "$name" --document-status final \
    --from ".spektacular/tmp/$name.md" >/dev/null
  rm ".spektacular/tmp/$name.md"
done

# storage and help are independent; commands builds on storage. The epic's
# name is minted by the CLI, so the agent and the verifier find it with
# `spektacular epic list`: it is the project's only epic.
cp "$SEED/epic.md" .spektacular/tmp/epic.md
spektacular epic write todo --document-status final --from .spektacular/tmp/epic.md \
  --data '{"specs":[
    {"name":"20260101000001-storage","depends_on":[]},
    {"name":"20260101000002-commands","depends_on":["20260101000001-storage"]},
    {"name":"20260101000003-help","depends_on":[]}
  ]}' >/dev/null
rm .spektacular/tmp/epic.md

# Fail the image build, not the agent run, if the fixture does not start
# where the task assumes: two specs ready to plan and one waiting.
epic=$(spektacular epic list | jq -r '.files[0].name')
states=$(spektacular status "$epic" --format json \
  | jq -c '[.specs[] | {(.name): .run.plan.state}] | add')
expected='{"20260101000001-storage":"ready","20260101000002-commands":"blocked","20260101000003-help":"ready"}'
if [ "$states" != "$expected" ]; then
  echo "seeded epic is not in its expected starting state: $states" >&2
  exit 1
fi
