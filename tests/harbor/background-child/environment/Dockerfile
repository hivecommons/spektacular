FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq git python3 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

# The probe needs no project and no spektacular binary: it only measures how
# the agent's harness treats one long background child.
WORKDIR /app

# Set for parity with every other suite, so the metrics recorder reads the
# same environment. Nothing in this probe calls spektacular.
ENV SPEKTACULAR_SESSION_LOG=on
