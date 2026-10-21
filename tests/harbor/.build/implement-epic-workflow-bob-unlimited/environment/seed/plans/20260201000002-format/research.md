# Research: 20260201000002-format

## Alternatives considered and rejected

Formatting inside the CLI was rejected: the export spec and later output would duplicate the layout.

## Drafting assumptions

- Todos always carry `id`, `title` and `done`; malformed todos are the caller's problem.
