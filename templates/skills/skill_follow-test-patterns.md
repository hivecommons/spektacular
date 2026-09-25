# Follow Test Patterns

Guide for writing tests that match the project's existing conventions. Use this skill when the implement workflow reaches its `test` step and needs to delegate test authoring to a sub-agent.

## Instructions

Write tests for the code the main agent just implemented in the current task. Do **not** invent new conventions — match the shape of the tests that already exist in the project.

### Step 1: Discover the project's test conventions

Before writing anything, look at how existing tests in this project are written. Pick a neighboring `*_test.go` (or equivalent for non-Go languages) and note:

- **Framework and assertion library** — e.g. `stretchr/testify/require` for Go
- **Fixture pattern** — e.g. `t.TempDir()` for filesystem, in-test fakes rather than mocks
- **Helper convention** — e.g. a per-package `renderStep` / `testData` / `captureWriter` helper trio
- **Naming** — `TestXxxDoesY` vs `Test_Xxx_DoesY` vs other
- **File layout** — co-located next to the package under test (e.g. `foo_test.go` sits next to `foo.go`)

If the project has a `thoughts/notes/testing.md` or equivalent documentation, read it first.

### Step 2: Identify what to test

Re-read the current task in `plan.md` and extract every acceptance criterion. Each criterion should map 1:1 to at least one test assertion. If a criterion cannot be directly asserted in a unit test (e.g. it's a manual-verification criterion), note it and skip — do not invent a test to cover it.

### Step 3: Write the tests

Rules:

- **Match existing file locations.** If the package's tests live in `internal/foo/foo_test.go`, put new tests there — don't create a new `internal/foo/task1_test.go` unless the project already groups tests that way.
- **Use the existing helpers.** If the package has a `renderStep` helper, use it. If it doesn't, write tests directly rather than introducing a new helper.
- **Don't mock internal code.** Use in-test fakes and real dependencies where possible. Only mock at system boundaries.
- **Hand-maintain expected values.** Never derive expected values at runtime from the subject under test — that defeats the test's purpose.

### Step 4: Return a concise summary

When done, return a short report to the main agent:

- List of `*_test.go` files written or modified (absolute paths)
- One-line summary of what each new test asserts
- Any acceptance criterion that could not be covered by a unit test, with the reason

Do **not** return full test code — the main agent can read the files if it needs to.
