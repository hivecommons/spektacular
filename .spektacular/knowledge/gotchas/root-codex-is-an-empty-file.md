---
tags: [codex, install, layout]
---

# This repository's root `.codex` is a tracked empty file

The spektacular repository tracks `.codex` at its root as an empty regular file, not a
directory. Anything that installs Codex files into this repository, such as
`spektacular init codex` writing `.codex/agents/` or other `.codex/` content, fails here because
the path is a file.

Plans never re-init or migrate this repository, so the workflows do not hit it. Exercise Codex
installs in a throwaway project instead, and do not "fix" the file as a side effect of
unrelated work.
