# Design documents

The worked designs this project's features are built to: a settled CLI surface, a command's
input and output shape, a data format, or a worked example of one.

These are declared as the `design` source in `.spektacular/config.yaml` and reached through the
CLI rather than by reading files directly:

```bash
go run . design list --source design
go run . design read --data '{"source":"design","path":"<path>"}'
go run . design write --data '{"source":"design","path":"<path>"}' --from <file>
go run . design author --data '{"source":"design","path":"<path>"}' --from <file>
```

A spec references a design rather than restating it:

```bash
go run . design ref add --data '{"spec":"<spec>","source":"design","path":"<path>"}'
go run . design ref list --data '{"spec":"<spec>"}'
```

Spektacular owns the reference, not the document, and never imposes a structure on one or
requires a shipped design to match the code. Two kinds of document can sit in here. One written
through `design write` is stored exactly as supplied: nothing is added to it and nothing is
reformatted. One written through `design author` is a design Spektacular wrote with the user,
and carries the same lifecycle record every spec and plan carries, including the specs that
reference it. See the Design Documents page on the documentation site for the concept.

This README is excluded from design listings by `.spektacular_ignore`, so it does not show up
as a design document.
