# Context: 20260201000001-storage

## Task: Add the storage module with atomic save

`_path()` reads `os.environ.get("TODO_FILE")` on every call. `save()` uses `tempfile.mkstemp(dir=target.parent)`, writes with `json.dump`, then `os.replace(tmp, target)`; on any exception it unlinks the temporary file and re-raises.
