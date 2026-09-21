---
tags: [harbor, testing, e2e, workflow]
---

## Harbor e2e scenarios can only assert against the final spec, never intermediate working files

The spec workflow's per-section working files under
`.spektacular/work/{{spec_name}}/` are deleted by the verification step
(`templates/steps/spec/08-verification.md`'s `rm -rf
.spektacular/work/{{spec_name}}` cleanup) once the final spec is committed
and confirmed.

The harbor test harness (`tests/harbor/spec-workflow/`) only inspects the
container's filesystem state *after* the agent's run finishes. By that
point the working files are already gone — any assertion that wants to
prove something happened at the working-file level (a specific section's
draft content, a correction that landed in one working file versus another)
is impossible to write directly against the working file.

Future e2e scenarios in this harness that want to prove an effect on
intermediate state can only assert against the *persisted, final assembled
spec* under `.spektacular/specs/` — checking that the effect is visible in
the finished output — even when the actual mechanism being tested operates
on a working file that no longer exists by the time the test runs.
