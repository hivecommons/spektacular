FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq python3 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

WORKDIR /app

# The agent's run is recorded at Spektacular's default log level, with no
# special setting, under .spektacular/logs/, which comes back with the
# .spektacular artifact.
