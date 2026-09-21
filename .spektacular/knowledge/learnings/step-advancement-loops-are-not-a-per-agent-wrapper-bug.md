---
tags: [workflow, agent, step, goto]
---

# Step-advancement loops are not a per-agent wrapper bug

When an agent gets stuck looping instead of advancing a spec/plan/implement
workflow, don't reach first for a per-agent (claude/bob/codex) wrapper or
install-time theory. The exact `goto <next_step>` command is rendered by the
CLI itself into the bottom of every step's instruction output — identical
bytes regardless of which agent is driving the workflow. Per-agent install
wrappers do not touch, generate, or drop this text.

So if looping behaviour differs across agents, the cause is not agent-specific
wiring in this codebase; it would be a model-behaviour difference in whether
the agent copies the literal provided command versus paraphrasing or
guessing a step name. Confirming or investigating that difference requires
observing the agent's actual behaviour (e.g. a session transcript or debug
log), not auditing the per-agent install code.
