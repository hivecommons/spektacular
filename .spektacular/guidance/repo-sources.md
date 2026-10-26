# Where the Code Lives — Read This First

> Managed by `spektacular init` and refreshed by `spektacular migrate` — edit
> `templates/agents/repo-sources.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

**STOP. The directory you are running in is not necessarily the code.**

This is a Spektacular project. The code it describes may live anywhere on
disk — a sibling directory, somewhere else entirely on the filesystem, or a
clone this project manages. A project directory holding only
`.spektacular/`, `knowledge/`, and `repos/` is normal, and is **not** an
empty or broken repository: the code is elsewhere, and the project knows
where.

**Before any code-touching work — searching, reading, grepping, editing,
running tests, verifying, or answering a question about how something
works — run:**

```
spektacular repo list
```

Every registered repo comes back with a `root`: the absolute path where
that repo's code actually lives. Work in that `root`. Cite files by paths
under it. Run tests from it.

These rules are not negotiable:

- **Never assume the directory you started in holds a repo's code.** Check
  with `spektacular repo list` first, in every session, before you open the
  first file.
- **Never guess a path** from the project's name, a repo's name, or a
  directory that looks plausible. The registry is the only authority on
  where code lives.
- **Pass the relevant repo's `root` explicitly to every sub-agent you
  launch.** A sub-agent inherits your working directory, not your knowledge
  of where the code is.
- **If a repo's `root` is not present on disk, stop and tell the user.**
  Report the path the registry gave and that it is missing. Do not
  substitute a directory that looks close, and do not silently carry on in
  the project directory instead.

This rule outranks your default assumptions about working directories, and
it binds everywhere in this project — inside spec, plan and implement
workflows, and equally in ad-hoc questions, unrelated skills, and general
exploration. It is not limited to workflow steps.
