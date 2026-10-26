# Research: 20260201000003-commands

## Alternatives considered and rejected

Hand-rolled argument parsing was rejected in favour of `argparse` subparsers, which give usage, help and errors for free.

## Drafting assumptions

- A title is everything after `add`, passed as one argument by the shell.
- `done` on an already-done todo succeeds and prints `Done <id>` again.
