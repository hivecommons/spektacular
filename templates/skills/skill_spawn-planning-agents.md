# Spawn Planning Agents

Orchestrate parallel research agents to investigate the codebase for planning purposes.

## Instructions

Use your agent orchestration capability to run multiple research tasks in parallel.

**Multi-repo projects**: when the project registers more than one repo, fan the research out per repo — the `repo list` command reports each registered repo's metadata and the `root` its code lives at — and give every research agent below an explicit code directory (or one agent per repo per question, for questions that span repos). Each repo's description, role, and tags tell you which repos a question is even relevant to; don't send every agent into every repo indiscriminately.

Launch the following agents concurrently:

### Agent 1: File Discovery
Find all files related to the feature being planned. Organize results by category:
- **Implementation files**: Source code that will be modified
- **Test files**: Existing tests for the affected code
- **Config files**: Configuration that may need changes
- **Documentation**: Existing docs about the relevant systems

### Agent 2: Prior Research
Search for existing research and plans related to this feature:
- Search the configured knowledge sources with the `knowledge search` command for related notes, gotchas, or prior learnings — results are ranked one per matching document, tagged by the tier and store name they came from, and carry the entry's own `tags`. A document need not contain every query word: it is returned if it carries evidence for any of them, so an empty result is not proof that nothing on the subject exists — try a differently-worded query before concluding the knowledge base is silent
- List prior plans with `plan file list` and read related plans with `plan file read <name> plan`
- List prior specs with `spec file list` and read related specs with `spec file read <name>`
- Reach stored artifacts only through the CLI, never by listing or reading store directories with your own file tools; store locations are configured and must not be built by hand
- Look for relevant issues, tickets, or TODOs in the codebase

### Agent 3: Similar Implementations
Find code examples that are similar to what needs to be built:
- Search for analogous patterns in the codebase
- Find existing implementations that can be modelled after
- Note file:line references for all findings

### Agent 4: Architecture Analysis
Understand how the relevant components fit together:
- Identify the dependency graph for affected modules
- Map integration points between components
- Note any shared state or cross-cutting concerns

## Output

Each agent should return structured findings. Combine all results and use them to inform your planning decisions.
