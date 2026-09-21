---
tags: [fsm, workflow, state, callback]
---

# looplab/fsm: Cancel() only stops a transition from before_/leave_ callbacks

`Event.Cancel()` sets the event's returned error, but it only actually
*prevents* the state change when called from a `before_<EVENT>` or
`leave_<STATE>` callback. `FSM.Event()` checks/commits the transition
(`f.current = dst`) and runs `enter_state` *between* `beforeEventCallbacks`/
`leaveStateCallbacks` and `afterEventCallbacks` — so by the time an
`after_<EVENT>` callback calls `Cancel()`, the transition has already
committed and any `enter_state` side effect (e.g. this codebase's
`saveState` in `internal/workflow/workflow.go`) has already run. Canceling
from `after_<EVENT>` only affects the error returned to the caller; it does
not roll back the state change or anything `enter_state` already did.

To make a callback's failure actually prevent the transition (and any
`enter_state` persistence tied to it), register the callback as
`before_<EVENT>` (or `leave_<STATE>`), not `after_<EVENT>`. Verified against
`github.com/looplab/fsm` v1.0.3: `fsm.go`'s `Event()` calls
`beforeEventCallbacks` first and returns immediately on error, before ever
touching `f.current` or invoking `enterStateCallbacks`; `afterEventCallbacks`
runs last, after the transition and `enter_state` are already done.
