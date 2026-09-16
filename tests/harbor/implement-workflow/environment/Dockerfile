FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

WORKDIR /app

# --- Colocated project + repo footprint at /app -----------------------------
#
# The auth-service project lives at /app. It is both a project and (through
# the "auth" member entry pointing at ".") a colocated repository. Both the
# project config and the repo config live under the same .spektacular/
# directory, which is what happens naturally when a repo is a member of a
# project that shares its working tree. init preserves an existing
# config.yaml/repo.yaml, so pre-seeding here is safe.
RUN mkdir -p /app/.spektacular/specs /app/.spektacular/plans/20260101000000-jwt-auth
COPY config.yaml /app/.spektacular/config.yaml
COPY repo.yaml /app/.spektacular/repo.yaml
COPY spec.md /app/.spektacular/specs/20260101000000-jwt-auth.md
COPY plan.md /app/.spektacular/plans/20260101000000-jwt-auth/plan.md
COPY test-plan.md /app/.spektacular/plans/20260101000000-jwt-auth/test-plan.md
COPY state.json /app/.spektacular/state.json

# --- Separate member repo: footprint under the project, code elsewhere ------
#
# The "docs" member repository keeps its Spektacular files in the project
# folder at /app/repos/docs/.spektacular/ (the registry's `location`, which
# names the folder holding repo.yaml), while
# its code lives at /opt/docs-repo, declared by `source: file:///opt/docs-repo`
# in its repo.yaml. The code checkout carries NO .spektacular/ directory and
# must stay that way: any repo-routed changelog write
# (`changelog file write --repo docs`) lands under
# /app/repos/docs/.spektacular/changelog/auth-service/<name>.md, and nothing
# Spektacular writes may touch /opt/docs-repo.
RUN mkdir -p /app/repos/docs/.spektacular/knowledge /app/repos/docs/.spektacular/changelog
COPY docs-repo.yaml /app/repos/docs/.spektacular/repo.yaml
RUN mkdir -p /opt/docs-repo/docs && printf '# auth-service docs\n' > /opt/docs-repo/README.md \
    && printf '# API reference\n' > /opt/docs-repo/docs/api.md

# The pre-seeded spec, plan, and test-plan files each carry draft
# frontmatter directly (see environment/*.md). The finished step's
# metadata.Close(StatusFinal) call updates them to final. No
# runtime frontmatter injection is needed.
