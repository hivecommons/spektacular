#!/bin/bash
# Builds the project the agent starts in: initialised for the agent under
# test, with a registered code repository outside /app and one final spec,
# all written through the CLI of the binary under test so the fixture always
# meets the current store contracts instead of drifting from them.
set -euo pipefail

SEED=/tmp/seed
SPEC=20260101000001-low-stock-alerts

cd /app

# The key difference from every other suite: init runs here, at build time,
# so the standing rules in AGENTS.md are the agent's project instructions
# from the first turn, instead of a file it writes partway through its run.
spektacular init {{agent}} >/dev/null

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

git init -q
git -c user.name=seed -c user.email=seed@example.com add -A
git -c user.name=seed -c user.email=seed@example.com commit -qm "initialise spektacular project"
