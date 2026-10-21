FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq git python3 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

# The code the project describes lives outside the directory the agent starts
# in. Only the repo registry says where, which is what the "where the code
# lives" rule sends the agent to.
COPY inventory/ /srv/inventory/
RUN cd /srv/inventory && git init -q \
    && git -c user.name=seed -c user.email=seed@example.com add -A \
    && git -c user.name=seed -c user.email=seed@example.com commit -qm "inventory service"

WORKDIR /app

# Unlike every other suite, the project is initialised for the agent here, at
# build time, so AGENTS.md (and CLAUDE.md for Claude) are already the agent's
# project instructions when its session starts.
COPY seed/ /tmp/seed/
RUN bash /tmp/seed/seed.sh && rm -rf /tmp/seed

# Record every spektacular call, whichever agent drives the run, to
# .spektacular/debug/, which comes back with the .spektacular artifact.
# Set after seeding, so the log holds only the agent's run.
ENV SPEKTACULAR_SESSION_LOG=on
