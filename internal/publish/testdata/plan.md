# Plan: nightly-digest

## Milestones & Tasks

### Milestone 1: Digest can be sent

#### - [ ] Task: Add the digest scheduler
**Id:** sched-1
**Repo:** api
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] a digest job runs at the configured hour
- [ ] the job is idempotent within a day

#### - [ ] Task: Render the digest email
**Id:** render-2
**Repo:** api
**Depends on:**
- sched-1 — Add the digest scheduler
**Execution:** agent

**Acceptance criteria**:
- [ ] filed, merged and parked items each have a section

### Milestone 2: Operators can opt out

#### - [ ] Task: Add the opt-out toggle
**Id:** optout-3
**Repo:** web
**Depends on:**
- render-2 — Render the digest email
**Execution:** human — needs a design review

**Acceptance criteria**:
- [ ] the settings page shows the toggle
