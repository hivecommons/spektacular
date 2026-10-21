---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Compact standing rules in AGENTS.md, full guidance on demand

## What was built

Spektacular installs eight standing rules into every project's `AGENTS.md`:

- where the code lives
- store access
- memory and context
- the knowledge, spec and design triggers
- draft presentation
- historical artifacts

Until now each one went in as its full text, about 22 KB in total, which every agent session read at start-up whatever the task.

Each rule is now installed as two pieces:

- **A short section in `AGENTS.md`**, under the same heading as before. It states the obligation that always applies (for example: run `repo list` before code work; never touch store files with your own tools; offer, never start, a spec) and when to read further. It ends with a link: ``Managed by `spektacular init`; full guidance: [.spektacular/guidance/<rule>.md](...)``.
- **A guidance file** at `.spektacular/guidance/<rule>.md` holding the rule's full text, unchanged apart from a title, a banner rewritten for a standalone file, and (for store access) a new paragraph. It is a plain file with no `@` import, so no agent loads it until it opens the link.

Spektacular's part of `AGENTS.md` is now 4,254 bytes for every agent, an 80% reduction.

In the `spektacular` repo:

- **One installer for every agent.** A single table-driven installer (`internal/agent/standing_rules.go`) replaces the five agents' hand-copied lists of eight section installers, and omp's rule writer reads the same table. The historical-artifacts rule's private copy of the section-placement logic is retired. Adding a rule is now one table entry plus its two templates.
- **Templates.** New short templates live in `templates/agents/short/`. The existing full templates are now the guidance documents.
- **Install order.** The installer writes each guidance file before the section that links to it, then replaces or places the section by heading.
- **`init` and `migrate`.** Both already reinstall the agent, so new projects get the compact form, and existing projects convert in place. The team's own `AGENTS.md` content stays exactly where it was. Outdated or missing guidance is restored, and re-running changes nothing. There is no new migration step or settings-format change.
- **Store-access exemption.** The store-access rule now says, in both forms, that the guidance files may be read with the agent's own tools and are never edited.
- **Agents.** omp's always-applied rule files carry the same short statements, so they are still identical to AGENTS.md for deduplication. CLAUDE.md still imports AGENTS.md exactly as before.
- **Tests.** Every existing phrase test is kept on the surface that must carry it. New tests pin, for all five agents:
  - the size and line budget;
  - link resolution and the absence of eager includes;
  - conversion of a frozen pre-change `AGENTS.md`;
  - idempotency;
  - guidance refresh through both `init` and `migrate`.
- **Harbor suite.** A new `standing-rules` harbor suite starts the agent in an already initialised project, so the rules are its real project instructions. It checks that Claude, Codex and Bob consult the repo registry, read a spec only through the CLI, and offer rather than start a spec. It landed first, so a baseline against the full-text rules can be run. The Bob harbor agent moved to `tests/harbor/agents/`, where the Makefile already looked, so Bob runs of every suite can start.

In the `docs` repo:

- The configuration reference has a new section, "Standing rules: AGENTS.md and .spektacular/guidance/".
- The migrate section explains how an upgrade converts a project.
- The omp text says the omp rules mirror the short sections.

## Why it matters

Every session in a Spektacular project used to spend about 22 KB of context on Spektacular's own rules before the user's work began. Sessions now start lighter and cheaper, with the agent's attention on the task. The agent loses no rule: the binding part of each rule is still always in front of it, and the detail is one link away when the situation arises.

## Deviations from the plan

- **Harbor suite verifier.** The verifier's spec-offer check accepts a few more phrasings than planned and matches "spec" only as a whole word. The seed's leak check looks for the code itself rather than a bare number.
- **README.** It lists the three new harbor make targets.
- **Managed-section installer.** It gained a render-data variant, so the short templates can receive the guidance path.
- **Error messages.** New guidance-write errors suggest re-running `init`.
- **Logging.** Each guidance file is logged on install.
- **Instruction-contract guards.** The cmd guards now include the guidance files, so the full text stays covered.
- **Legacy fixture.** It was captured from the pre-change binary's real output rather than rendered from git history, and is copied into `cmd/testdata/` for the cmd tests.
- **Manual runs still to do.** The harbor baseline and after runs, the live Claude `/context` check, and the Copilot and omp sessions need credentials and live agents. They are recorded in the test plan rather than run here.
