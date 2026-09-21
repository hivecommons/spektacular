---
tags: [workflow, fsm, goto, step]
---

# Goto to the current step is a silent no-op

Calling `goto` on a spec/plan/implement workflow with a step name equal to the
*current* step does not error and does not change state. `Workflow.Goto`
detects `name == w.Current()` and takes the `renderStep` path, which
re-invokes the step's callback to re-emit its instruction without firing an
FSM transition — no error, no `completed_steps` change, no signal to the
caller that nothing advanced.

This is distinct from the "invalid transition" case (calling `goto` with a
step unreachable from the current state), which does return an FSM error.
The same-step case returns what looks like a normal, successful step
response — identical in shape to a real advance — so an agent that
mis-derives the next step name as the current one gets no feedback that it
failed to progress. Worth checking for when diagnosing an agent stuck
re-reading the same instruction repeatedly.
