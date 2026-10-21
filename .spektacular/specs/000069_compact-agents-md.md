---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Feature: 000069_compact-agents-md

## Overview

Every conversation an agent has in a Spektacular project starts by reading the full project instructions, and Spektacular's own rules make up about 22 KB of that, whether or not they matter to the task at hand. This feature shrinks each of those rules to a short statement of what must always be done and when to look further, with the full guidance kept in separate files that the agent reads only when that situation comes up. Teams get lighter, cheaper sessions that keep the agent's attention on their work, without the agent losing any of the rules it must follow.

## Requirements

- [ ] **Each rule is stated briefly in the project instructions**
  For every Spektacular rule installed into the project instructions, the system must keep a short statement of the binding rule and of the situation in which the full guidance applies, rather than the full guidance itself.

- [ ] **Full guidance is available on demand**
  For every rule shortened this way, the full guidance must remain available within the project, and the short statement must tell the agent where to find it.

- [ ] **Detail is read only when needed**
  The full guidance must not be loaded into an agent's session automatically; an agent must be able to open it when the rule's situation arises.

- [ ] **No rule is lost or weakened**
  Every requirement the current full guidance places on an agent must still be stated either in the short statement or in the linked guidance, and the binding part must be in the short statement.

- [ ] **New projects get the compact form**
  Initialising a project for any supported agent must produce the compact project instructions and the linked guidance.

- [ ] **Existing projects can be converted**
  Upgrading an existing project must convert its project instructions to the compact form and install the linked guidance.

- [ ] **Other content is left alone**
  Installing, upgrading or converting must not change any content in the project instructions that Spektacular did not install.

- [ ] **Guidance is kept current**
  Re-running initialisation or upgrade must bring the linked guidance up to date with the installed Spektacular version, and must not duplicate it.

- [ ] **Works for every supported agent**
  Projects using Claude, Codex, Copilot, omp or Bob must all receive the compact form, and agents in each must still follow every rule.

- [ ] **Documented**
  The project's documentation website must describe the compact layout, where the linked guidance lives and how upgrading converts an existing project.

## Constraints

- The linked full guidance must live in files under `.spektacular/`, written by `spektacular init` and kept current by `spektacular migrate`.
- The linked guidance must not be pulled in through an eager include mechanism such as Claude Code's `@file` import.
- Each rule must remain binding everywhere it binds today, including in sub-agents and in every workflow, ad-hoc question and skill.
- The project's rule that Spektacular-managed files are reached only through the Spektacular CLI must explicitly exempt the linked guidance files, so agents can read them with their own file tools.
- omp's always-applied rule files must state the same binding rules as the compact form, whether they carry the full text or point at the linked guidance.
- CLAUDE.md must continue to import AGENTS.md exactly as it does today.
- Any change to the installed instruction text must be mirrored in the end-to-end agent test suites (the harbor suites) that depend on it, per the project's testing-architecture knowledge entry.

## Acceptance Criteria

- [ ] **The project instructions shrink**
  After initialising a new project, the Spektacular-installed portion of the project instructions is under 5 KB, down from about 22 KB.

- [ ] **Every rule has a short form and a link**
  After initialising, each Spektacular rule appears in the project instructions as a short statement of at most 6 lines (at most 15 for the rule on where the code lives, which must be acted on first) that names the situation it applies to and links to a guidance file that exists in the project.

- [ ] **The binding part is inline**
  For every rule, the obligation an agent must always honour is stated in the short statement itself, not only in the linked file.

- [ ] **Nothing is lost**
  For every rule, every instruction in the current full guidance can be found either in its short statement or in its linked guidance file.

- [ ] **Every agent gets the compact form**
  Initialising a project for each of Claude, Codex, Copilot, omp and Bob produces the compact project instructions and the linked guidance files; for omp, its rule files state the same binding rules.

- [ ] **Guidance is not loaded eagerly**
  A new session in an initialised project does not include the linked guidance files' content until the agent opens one.

- [ ] **Agents still follow the rules**
  In end-to-end runs with each supported agent, the agent finds the code through the repo registry, reaches Spektacular-managed files only through the CLI, and offers rather than starts a spec, with no drop in the end-to-end suites' pass rates compared with before the change.

- [ ] **Existing projects convert**
  Upgrading a project initialised before this change leaves its project instructions in the compact form, with the linked guidance files present.

- [ ] **User content survives**
  A project whose instructions contain content of its own before and after Spektacular's sections still contains that content, unchanged and in the same place, after initialising or upgrading.

- [ ] **Re-running is idempotent**
  Running initialisation or upgrade twice in a row leaves the project instructions and the guidance files identical to running it once.

- [ ] **Outdated guidance is refreshed**
  After installing a newer Spektacular whose guidance differs and running upgrade, every guidance file matches the content shipped with the new version.

- [ ] **Guidance files can be read directly**
  An agent can open a linked guidance file with its own file tools without the project's rules telling it not to.

- [ ] **Documented on the website**
  The documentation website describes the compact layout, the location of the linked guidance files and how upgrading converts an existing project.

## Technical Approach

- Keep the existing per-section templates as the source of each rule's full guidance, and add a short form per section for the project instructions; the "Managed by spektacular init" banner can shrink with them.
- Use a single folder with one file per section, linked from AGENTS.md with ordinary relative markdown links so every agent can follow them.
- Write each short form as the rule plus its trigger ("before X, read Y"), so the agent knows when to open the linked file without having read it.
- For omp, either point its rule files at the same guidance or keep giving them the full text, since its rules are already per file; the plan should decide.

## Success Metrics

- The tokens an agent spends on Spektacular's standing instructions at the start of every session fall by at least 75%.
- No end-to-end suite's pass rate drops for any agent after the change.

## Non-Goals

- Restructuring the workflow skills or step templates, which are already loaded on demand.
