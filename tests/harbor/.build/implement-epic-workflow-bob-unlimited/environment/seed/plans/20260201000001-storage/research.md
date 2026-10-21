# Research: 20260201000001-storage

## Alternatives considered and rejected

Writing in place was rejected: a crash mid-write leaves a truncated file. A temporary file in the system temp directory was rejected: it may sit on another filesystem, where `os.replace` is not atomic.

## Drafting assumptions

- An empty `TODO_FILE` counts as unset.
- Invalid JSON in an existing file raises rather than reading as empty.
