FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

# Record every spektacular call, whichever agent drives the run, to
# .spektacular/debug/, which comes back with the .spektacular artifact.
ENV SPEKTACULAR_SESSION_LOG=on

WORKDIR /app

RUN mkdir -p /app/.spektacular/specs
COPY 20260101000000-user-auth.md /app/.spektacular/specs/20260101000000-user-auth.md

# Seed a distinctive always-apply convention relevant to the JWT auth spec so
# the plan workflow has a convention to surface into plan.md's ## Conventions
# section. Named differently from init's starter (conventions.md) so the
# idempotent `spektacular init` re-create does not clobber it.
RUN mkdir -p /app/.spektacular/knowledge/conventions
COPY auth-audit-logging.md /app/.spektacular/knowledge/conventions/auth-audit-logging.md
