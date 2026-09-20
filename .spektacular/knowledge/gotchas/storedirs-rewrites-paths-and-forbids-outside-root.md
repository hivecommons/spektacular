---
tags: [config, paths, storage, validation]
---

## Config.storeDirs() both rewrites a path and forbids it leaving the project root

`Config.storeDirs()` (`internal/config/config.go`) looks like a plain
enumeration of configured directories, so adding a new configured path to it
reads as registration. It is not: membership in that list is load-bearing
twice over, and both effects are invisible at the call site.

- `resolveStoreDirs` and `fileFormStoreDir` **re-express the value** on read and
  on write, so the path written back to `config.yaml` is not necessarily the
  path the author wrote.
- `Config.Validate` runs `validateStoreDir` over every entry, which **refuses
  any directory that resolves outside the project root** via `escapesRoot`.

Both are correct for the spec, plan and changelog stores, which have only ever
lived inside the project. They are wrong for any configured path that must
survive verbatim, or that legitimately points somewhere else on disk: the
author's path gets rewritten under them, and a location outside the project is
rejected outright with an error about a store directory they did not think they
were declaring.

A configured path with either of those requirements does not belong in
`storeDirs()`. Declare it in its own settings section, leave the value
untouched on load and write, and resolve it in the domain package that owns it
— joining a relative value onto `config.ProjectConfigDir(projectRoot)` and
stat-ing the result there, so an unreachable location is refused by the
component that actually uses it. `knowledge.sources[].config.location` has
worked this way since it was introduced, and
`design.sources[].config.location` follows it deliberately: pointing a design
source at a folder the team already keeps, anywhere on disk, is the whole point
of the feature, and `storeDirs()` would have made it impossible.

The tell that you are about to get this wrong is reaching for `storeDirs()`
because it is where the other directories are listed. Ask instead whether the
path may be rewritten and whether it must stay inside the project root. Two
noes mean it resolves elsewhere.
