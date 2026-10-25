FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq git python3 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/0.9.5/install.sh | sh

ENV PATH="/root/.local/bin:$PATH"
RUN uv python install 3.12 && uv tool install pytest==8.4.1

COPY spektacular /usr/local/bin/spektacular
RUN chmod +x /usr/local/bin/spektacular

WORKDIR /app

# The todo app the epic's specs describe, committed so the plans have a real
# repository to target.
COPY app/ /app/
RUN git init -q \
    && git -c user.name=seed -c user.email=seed@example.com add -A \
    && git -c user.name=seed -c user.email=seed@example.com commit -qm "todo skeleton"

COPY seed/ /tmp/seed/
RUN bash /tmp/seed/seed.sh && rm -rf /tmp/seed
