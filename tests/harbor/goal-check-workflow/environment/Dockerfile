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

# The todo app the task extends, committed so the task's work shows as the
# diff the goal check's reviewer reads.
RUN git config --global user.name "todo developer" \
    && git config --global user.email "dev@todo.example"
COPY app/ /app/
RUN git init -q && git add -A && git commit -qm "todo app"

COPY seed/ /tmp/seed/
RUN bash /tmp/seed/seed.sh && rm -rf /tmp/seed
