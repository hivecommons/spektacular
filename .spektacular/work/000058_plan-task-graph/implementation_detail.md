**A plan-reading module boundary (new pattern).** Code that needs to understand a plan's work stops pattern-matching plan.md on its own. It asks the task reader instead. The reader works line by line with anchored heading patterns, the same style as the current milestone scanner. The project has no markdown library and this feature does not add one. Parsing is confined to the `## Milestones & Tasks` section, with the legacy `## Milestones & Phases` section recognised as well. Anything outside that section, including `- [ ]` lines in the Changelog or Testing Approach, never counts as a task. `Parse` is tolerant and records what it saw, while `Validate` is strict. That split lets the milestone committer and whole-plan implement keep working on old or partly structured plans, while writes of task-format plans are held to the full rule set.

**Validation at the store boundary.** The shared store-file command builder gains an optional validator. This is the first time that builder enforces content rules rather than just metadata. The plan store is the only one that opts in, and it validates only `plan.md`, so `context.md`, `research.md` and `test-plan.md` writes are unchanged. Because validation runs before the store write, a refused write has no side effects, and callers see a normal structured error.

**Pluggable id providers (following the store pattern).** Task ids follow the same "provider name in config → implementation" idea as stores. They are held in a small in-package registry so another provider can be added later without touching the plan format or the export. Following the existing identifier package, providers are plain functions behind a name, not a plugin system.

**First non-JSON output mode.** `plan export --format pretty` is the first command that prints human text on success. It is written as a pure renderer from the export document to text, so the JSON and pretty paths share every lookup and differ only in the final write. Errors never switch format: they always go through the existing JSON failure path.

**Single-task implement as data, not a new workflow.** The implement FSM gains one edge (`update_changelog → finished`) and one piece of persisted data (`task`). The scoping lives in two places:

- Step callbacks pass the selected task to templates.
- Templates use conditional mustache sections to swap "the first unchecked task" for "the selected task".

The last-task decision moves from template prose into Go, so it is unit-testable. The template still renders both exits, but it tells the agent which one to take. A developer reading the implement steps will see one workflow with one optional input, not two parallel step lists.

**Legacy compatibility is explicit.** Wherever templates or Go code name the work unit, they name "task" first and fall back to the legacy "Phase N.M" form only where an old plan must still run: implement templates, the milestone committer and the unchecked-work count in `implement status`. The fallback is described in one shared partial rather than repeated in every template.

**Vocabulary change across the prose surface.** Renaming phase → task touches many templates, skills, the plan step name, scaffolds, the glossary and the harbor oracles. It is done as a mechanical rename followed by the semantic additions (ids, dependencies, executor), so the rename's diff stays reviewable and phrase-assertion tests fail in one place at a time.
