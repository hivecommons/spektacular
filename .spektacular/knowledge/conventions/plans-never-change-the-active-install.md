---
tags: [workflow, config, plan, skills, self-hosting]
---

# A plan never changes the active skills and configuration

Spektacular is developed with Spektacular, so the code a plan changes is also the tool that could
be driving that plan's workflows. Keep the two apart: no spec, plan or implement workflow may
change the Spektacular install it is running on.

- **Workflows run on an installed binary, never on the working tree.** This repo's
  `.spektacular/config.yaml` sets `command: spektacular`, so every skill, step instruction and
  managed AGENTS.md section tells the agent to run the installed binary. Never `go run .`, which
  compiles whatever the working tree holds at that moment, so a task that changes a step template,
  a callback, the workflow steps or the settings format silently changes the instructions every
  later task runs under, and a settings-format change stops the run outright.
- **A plan never touches the active install.** Its tasks never migrate or re-init this repo's
  `.spektacular/`, never regenerate the installed skills or managed sections, and never rewrite
  `.spektacular/config.yaml`. A plan that bumps the settings format or changes skill templates
  verifies the change with `go test` and throwaway projects only. Planning a "migrate this
  repository's own configuration" task is a sign the plan has crossed this line.
- **To run your changes for real, install them between workflows.** `make install-local` builds
  the binary and copies it to `/usr/local/bin`; the next skill's version check then brings this
  repo's settings and skills up to date. Never do it while a workflow is in progress.

This came from 000060 (epics and seeded specs), whose schema bump made `go run .` refuse to drive
its own implement run until this repo was migrated partway through.
