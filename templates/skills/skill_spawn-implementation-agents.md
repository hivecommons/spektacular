# Spawn Implementation Agents

Guide for efficient agent orchestration during implementation tasks.

## Instructions

**Work belongs in its attributed repo.** When the plan attributes a task (or part of one) to a registered member repo, carry that work out inside that repo's code — the `repo list` command reports where it lives as `root` — not in the project's own directory. Give each implementation agent the exact source directory its files live in, and keep one agent's changes inside one repo where possible so verification and changelog derivation stay per-repo.

When breaking implementation into agent tasks, consider the complexity tier:

### Low Complexity (~10k tokens)
- **Strategy**: Single agent, sequential execution
- **Use when**: Simple file changes, adding a field, updating a config
- **No orchestration needed**

### Medium Complexity (~25k tokens)
- **Strategy**: 2-3 parallel agents for independent changes
- **Use when**: Multiple files need changes but changes are independent
- **Pattern**: Launch agents for each independent file group, then integrate

### High Complexity (~50k+ tokens)
- **Strategy**: Parallel analysis, sequential integration
- **Use when**: Cross-cutting changes, new subsystems, complex refactors
- **Pattern**:
  1. Launch parallel research agents to understand each affected area
  2. Integrate findings into a change plan
  3. Execute changes sequentially (or in independent parallel groups)
  4. Run verification after each group

## Agent Task Template

For each agent, specify:
- **Goal**: What the agent should accomplish
- **Files**: Which files to read/modify
- **Constraints**: What NOT to change
- **Success criteria**: How to verify the work is correct

## Token Management

- Keep each agent's context under 50k tokens
- Prefer focused agents over broad ones
- Use file:line references from planning to minimize search time
- Verify each agent's output before moving to dependent work
