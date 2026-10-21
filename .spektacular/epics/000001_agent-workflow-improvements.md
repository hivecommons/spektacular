---
created_date: "2026-10-21"
document_status: draft
specs:
    - name: 000069_compact-agents-md
      depends_on: []
    - name: 000068_task-goal-validation
      depends_on:
        - 000069_compact-agents-md
---

# Agent workflow improvements

Two improvements to how Spektacular guides coding agents, grouped so they can be planned and implemented together.

- **000069_compact-agents-md** — AGENTS.md keeps a short rule per managed section, with the full guidance loaded on demand from files under `.spektacular/guidance/`. Its first milestone also moves the Bob harbor agent to `tests/harbor/agents/`.
- **000068_task-goal-validation** — each plan task's goal is checked by an independent reviewer during implementation.

Both plans change the per-agent install files in `internal/agent/`, and 000068's Bob harbor runs rely on 000069's move of the Bob harbor agent, so 000069 goes first.
