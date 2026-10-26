#!/bin/bash
# Builds the project the agent starts in: initialised for the agent under
# test, with a registered code repository outside /app and one final spec,
# all written through the CLI of the binary under test so the fixture always
# meets the current store contracts instead of drifting from them.
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

SPEC=20260101000001-low-stock-alerts

cd /app

# The key difference from every other suite: init runs here, at build time,
# so the standing rules in AGENTS.md are the agent's project instructions
# from the first turn, instead of a file it writes partway through its run.
# init is never logged: a command is logged only where a project config
# already exists when it starts, and none does yet. Logging is then switched
# off for the rest of the seed.
spektacular init {{agent}} >/dev/null
set_log_level off

# The inventory service's code lives only in /srv/inventory. Nothing under
# /app says where, except the repo registry.
spektacular repo add --data '{"name":"inventory","location":"/srv/inventory","description":"stock-keeping service","role":"service","tags":["python"]}' >/dev/null

mkdir -p .spektacular/tmp
cp "$SEED/spec.md" ".spektacular/tmp/$SPEC.md"
spektacular spec file write "$SPEC" --document-status final \
  --from ".spektacular/tmp/$SPEC.md" >/dev/null
rm ".spektacular/tmp/$SPEC.md"

# The verifier checks the spec is unchanged at the end of the run against
# this digest, kept outside /app where the agent has no reason to look.
spektacular spec file read "$SPEC" | sha256sum | cut -d' ' -f1 > /tmp/spec.sha256

# The code question's answer must come from the registered repository, so
# fail the image build, not the agent run, if the code leaks into /app. (A
# bare "37" can turn up in any timestamp, so look for the code itself.)
if grep -rqE "REORDER_THRESHOLD|needs_reorder" /app; then
  echo "the reorder threshold appears under /app; the code question would not need the registry" >&2
  exit 1
fi

# Fail the image build, not the run, if any seed command reached the log:
# the agent must start with an empty session log.
if compgen -G "/app/.spektacular/logs/*.jsonl" >/dev/null; then
  echo "the seed's own spektacular commands were logged under /app/.spektacular/logs" >&2
  exit 1
fi
set_log_level standard
git init -q
git -c user.name=seed -c user.email=seed@example.com add -A
git -c user.name=seed -c user.email=seed@example.com commit -qm "initialise spektacular project"
