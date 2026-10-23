FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq python3 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

# Record every spektacular call, whichever agent drives the run, to
# .spektacular/debug/, which comes back with the .spektacular artifact.
ENV SPEKTACULAR_SESSION_LOG=on

# The repo the agent will be asked to add. It states enough about itself that
# every proposal after the first question can be drawn from it: a README with
# opening prose, a manifest declaring a name and a description that differ from
# the folder name, and source files establishing the language.
RUN mkdir -p /srv/harbour/src
RUN printf '# Harbour\n\nTide-gate scheduling for dockside cranes. Plans crane movements against tide windows so a berth is never blocked.\n' > /srv/harbour/README.md
RUN printf '{\n  "name": "harbour",\n  "description": "Tide-gate scheduling for dockside cranes",\n  "version": "1.0.0"\n}\n' > /srv/harbour/package.json
RUN printf 'export const berth = 1\n' > /srv/harbour/src/index.ts
RUN printf 'export const tide = 2\n' > /srv/harbour/src/tide.ts

WORKDIR /app
