---
tags: [testing, ci, permissions, filesystem, root]
---

# chmod-based test sabotage is ignored by root, and CI runs as root

Forcing an I/O failure with `os.Chmod(path, 0444)` works on a developer machine and does nothing
in CI. The suite runs as root inside the Dagger container, and root bypasses the permission bits
entirely, so the write the test meant to block succeeds.

It surprises because it **fails open**. The sabotage is silent: nothing errors, the code under
test simply takes the success path, and the test either passes vacuously or fails somewhere far
from its cause with a confusing message. Verified directly, not inferred:

```
uid=1000  chmod 0444 then write -> permission denied
uid=0     chmod 0444 then write -> <nil>
```

## What to do instead

Replace the target file with an empty directory. `os.ReadFile` and `os.WriteFile` both fail with
EISDIR on a directory, and that is a kind-of-file error rather than a permission check, so no uid
is exempt:

```go
require.NoError(t, os.Remove(path))
require.NoError(t, os.Mkdir(path, 0o755))
```

`cmd/design_ref_test.go`'s `replaceWithDirectory` is the worked example. Note the store's write is
`os.MkdirAll` then `os.WriteFile`, so sabotaging the file itself is what bites; making its parent
directory read-only does not stop a write to an existing file even as a normal user.

## Prefer this over skipping

Several tests here guard the problem with `if os.Geteuid() == 0 { t.Skip(...) }`. That keeps CI
green by dropping the assertion in the one environment that gates merges, which is the wrong
trade when the behaviour under test is a guarantee rather than a nicety. Reach for the directory
technique first; it keeps the assertion live everywhere and makes the skip unnecessary.
