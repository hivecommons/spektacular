---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
designs:
    - source: design
      path: plan-task-graph.md
epic: 000001_agent-workflow-improvements
---

# Feature: 000068_task-goal-validation

## Overview

When an agent implements a planned task today, the same agent that wrote the code also decides whether the task's acceptance criteria are met, and it decides mostly from whether the tests passed. This feature turns each task's acceptance criteria into a goal that an independent reviewer checks before the task can be marked done. The reviewer is a separate, fast and inexpensive helper agent that starts with a fresh context and only reads the work. It decides, criterion by criterion, whether each one is met, not yet met, or impossible as written. Teams using any supported coding agent can then trust that a ticked criterion really holds, and work that falls short goes back for another attempt instead of being signed off.

## Requirements

- [ ] **Each task has a goal**
  The system must treat a task's acceptance criteria, together with its stated scope limits (such as touching only the files that belong to the task), as that task's goal.

- [ ] **Independent check before a task is marked done**
  The system must have every criterion of the current task judged by a reviewer agent other than the one that implemented the task, after the task's verification commands have run and before any criterion is marked complete.

- [ ] **Fresh context**
  The reviewer must start with no access to the implementing agent's conversation. It judges only from the task's goal, the verification results and the work itself.

- [ ] **Reads the work itself**
  The reviewer must be able to inspect the changes and the files they touch directly, rather than relying on the implementing agent's description of what it did.

- [ ] **Read-only**
  The reviewer must not change code, tests, plans or any other file.

- [ ] **A verdict and evidence for every criterion**
  The reviewer must return, for each criterion, exactly one verdict (met, not yet met, or impossible as written) with evidence: where in the work it looked, or why it judged as it did.

- [ ] **Only "met" criteria are ticked**
  The system must mark a criterion complete only when the reviewer judged it met.

- [ ] **"Not yet met" goes back for another attempt**
  When any criterion is not yet met, the system must send the reviewer's reasons back to the implementing agent and have it try again, then check again. The task gets at most two retries after the first check.

- [ ] **Stop and ask when attempts run out**
  When criteria are still not met after the last allowed retry, the system must stop and ask the user whether to keep fixing, accept the shortfall, or skip the task. In an orchestrated run (an epic), this stop must reach the user as a question from that spec's run, and that spec's work must not be merged while the question is open.

- [ ] **"Impossible" stops at once**
  When any criterion is judged impossible as written, the system must stop without retrying and ask the user how to proceed, as it does today when the plan and reality disagree.

- [ ] **Fast, inexpensive model where possible**
  For each supported coding agent that lets a project choose a helper agent's model, the system must by default run the reviewer on a smaller, cheaper model tier than that agent's default working model. Where the user's own agent settings override a helper's model, those settings win.

- [ ] **Works with every supported agent**
  Projects using any supported coding agent (Claude, Codex, Copilot, omp, Bob) must get the check. Where an agent cannot select a separate model for the reviewer, the reviewer runs on that agent's available model.

- [ ] **On by default, and can be switched off**
  The check must be enabled by default, and a project must be able to switch it off in its configuration.

- [ ] **Model can be overridden per agent**
  A project must be able to override which model the reviewer uses, separately for each coding agent.

- [ ] **The outcome is recorded**
  The system must record every task's goal (its criteria and scope limits), its verdicts and the number of attempts it took, so the outcome can be inspected after the run.

- [ ] **Documented**
  The workflow and configuration reference on the project's documentation website must describe the check, its verdicts, its configuration and each agent's model limitations.

## Constraints

- The check must be performed by a sub-agent separate from the agent that implemented the task.
- Goals must be scoped to a single plan task.
- Goals must be built from the plan task shape settled in the `design` source at `plan-task-graph.md`: a task's acceptance criteria are its `- [ ]` / `- [x]` checkboxes. This spec changes that design's rule that criteria are "ticked by implement only when the criterion passed verification" to "ticked only when judged met"; the design document must be updated to match.
- Plans written before the task format must keep working: implement keeps running on them, as the referenced design guarantees.
- With the check switched off, implement must behave exactly as it does today, with criteria ticked from the verification results.
- Plan documents, and any record of goals and verdicts that Spektacular stores, must be read and written only through the Spektacular CLI, never edited in place, per the project's store-access rule.
- The goal concept and its verdicts must be owned by Spektacular and must not be delegated to any coding agent's native goal feature (Claude Code's or Codex's `/goal`) or to an agent-specific hook, so every supported agent (Claude, Codex, Copilot, omp, Bob) behaves the same way. Native goal features are user-invoked or absent in most agents.
- Changes to the implement workflow's steps must include the matching surfaces of every harbor suite they touch (implement and implement-epic), per the project's binding testing-architecture knowledge entry.

## Acceptance Criteria

- [ ] **The goal includes the task's criteria and scope**
  When a task is checked, the recorded goal for that task lists every acceptance criterion from the plan, plus the task's scope limits, and nothing from any other task.

- [ ] **Nothing is ticked until the check has run**
  In an implement run where verification passes, no criterion of the task is marked complete in the plan until the recorded outcome holds a verdict for every one of the task's criteria.

- [ ] **The reviewer is a separate agent**
  In a recorded run, the verdicts come from a helper agent launched for the check, not from the main implementing agent. The run's agent log shows that helper starting and returning.

- [ ] **The reviewer doesn't see the implementer's conversation**
  The input handed to the reviewer holds only the task's goal, the verification results and pointers to the work. It contains none of the implementing agent's conversation.

- [ ] **Catches a criterion that only the files can disprove**
  Take a task whose criterion is not met in the files, while the implementing agent's summary claims it is and every verification command passes (for example, a required documentation line is missing). The reviewer returns "not yet met" for that criterion.

- [ ] **Leaves the work unchanged**
  The working tree is the same before and after the reviewer runs: no file is added, changed or removed.

- [ ] **Every criterion gets exactly one verdict with evidence**
  For a task with N criteria, the recorded outcome holds exactly N verdicts. Each is one of met, not yet met or impossible, and each carries non-empty evidence.

- [ ] **Only "met" criteria are ticked**
  Take a task with a mix of verdicts that the user decides to accept. After the plan is updated, exactly the criteria judged met are ticked, and the rest stay unticked.

- [ ] **Not-yet-met work is retried with the reasons**
  When a criterion is first judged not yet met, the implementing agent receives the reviewer's reasons and works on the task again. A second check follows, and the recorded attempt count rises.

- [ ] **Stops and asks once attempts run out**
  When a criterion is still not met after two retries, the run stops and asks the user to keep fixing, accept or skip. In an epic run, the orchestrator receives a question from that spec's run, and the spec's work is not merged while the question is open.

- [ ] **"Impossible" stops without retrying**
  When a criterion is judged impossible, the run stops immediately after the check that returned it and asks the user how to proceed. The recorded attempt count shows no retry.

- [ ] **Uses a cheaper model where the agent supports it**
  For each agent that lets a project choose a helper's model (Claude, Codex, Copilot, omp), a run's agent log shows the reviewer running on a smaller, cheaper model tier than the agent's default working model, with no project configuration needed.

- [ ] **Every agent gets the check**
  An implement run under each supported agent, Bob included, produces recorded verdicts for every task.

- [ ] **Switching it off restores today's behaviour**
  With the check switched off in the project configuration, an implement run records no verdicts, and criteria are ticked from the verification results as they are today.

- [ ] **A model override takes effect for that agent only**
  When a project sets a reviewer model for one agent, that agent's reviewer uses the configured model, and other agents keep their defaults.

- [ ] **Plans written before the task format still work**
  An implement run on a plan written before the task format completes, with each phase's acceptance criteria judged the same way a task's are.

- [ ] **The outcome can be inspected afterwards**
  After a run, the recorded outcome for each task can be read back, showing its goal, each criterion's verdict and evidence, and the attempt count.

- [ ] **Documented on the website**
  The workflow reference on the project's documentation website describes the check and its three verdicts, and the configuration reference lists the on/off setting, the per-agent model override and each agent's model limitations.

## Technical Approach

- Add the check as its own step in the implement workflow, between running the verification commands and updating the plan: verification keeps producing the evidence (command results), the new step judges it, and the plan update ticks only what was judged met. Folding the judgement into verification was rejected because that sub-agent needs a shell and so cannot be read-only.
- Model the check on Anthropic's Managed Agents Outcomes and Claude Code's `/goal`: the task's criteria act as the rubric, and a grader in its own context returns a per-criterion verdict with feedback that drives a bounded retry.
- Give the reviewer the diff and the paths of the files the task touched, and let it open them itself, rather than a summary from the implementing agent (`/goal`'s evaluator judges only from the transcript).
- Have `spektacular init` install a named reviewer helper in each agent's native sub-agent format, with a fast default model and read-only tools where supported: Claude `.claude/agents/` (`haiku`, read-only tools); Codex `.codex/agents/` (a fast model; Codex has no read-only sandbox for sub-agents, so instruct the reviewer not to edit); Copilot `.github/agents/` (`model:`, tools list); omp `.omp/agents/` (the `@smol` role alias, which maps to the user's own cheap model). Where no named helper exists, fall back to a generic fresh-context sub-agent, as the spec workflow's verification review already does.
- Copilot also loads `.claude/agents/`, so a project initialised for both could see two reviewer definitions; the plan should decide how to avoid the duplicate.
- Copilot ignores a helper's model when the session model is `Auto`; document this rather than work around it.
- Bob has no documented per-helper model setting. Try its built-in read-only `explore` sub-agent (described as running on a lighter model) through the harbor Bob agent before settling on a fallback; otherwise run the reviewer on Bob's default model.
- Store the recorded outcome where harbor verifiers can assert on it from CLI state and the session log, independent of the agent that ran.
- Measure the reviewer with a harbor scenario seeding a task whose criterion is subtly unmet while its tests pass, since a false "met" is the costly failure.

## Success Metrics

- In the harbor scenario seeded with a subtly unmet criterion whose tests pass, the reviewer returns "not yet met" for that criterion in all of at least 10 runs per agent, on each agent's default reviewer model (no false "met").
- In the harbor scenario where every criterion is genuinely met, the reviewer returns "met" for all of them on the first attempt in at least 9 of 10 runs, so the check does not routinely send finished work back.
- The reviewer's model spend is under 10% of the implementing agent's spend for the same task, on agents where a fast model can be chosen.
- Adding the check lengthens a single-task implement run by under 2 minutes per task in the harbor implement suite.

## Non-Goals

- Spec-level goals (checking a spec's own success criteria when the spec is reconciled, or gating an epic's merge on them) are a possible follow-up, not part of this spec.
- Running the verification commands stays as it is today; the check adds a judgement and does not replace how commands are run.
