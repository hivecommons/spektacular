# Working context — implement 000051_working-context-and-plan-reading

(Previous spec/plan working context for this feature is in git history: commit 8255150.)

## Decisions / answers
- User chose plan 000051_working-context-and-plan-reading to implement.
- read_plan: structure valid, no drift found, all spec requirements covered, no `## Changelog` yet (first-phase mode).

## Learnings
- Repos: spektacular root = /home/nicj/code/github.com/jumppad-labs/spektacular; docs root = /home/nicj/code/github.com/jumppad-labs/spektacular-website (has unrelated uncommitted change in src/pages/knowledge-base.mdx — leave alone).
- Dogfooding hazard: after Phase 1.2 lands, `go run .` emits `.spektacular/working-context.md`; copy this file's content there by hand before leaving Phase 1.2 and leave the old file.

## Phase 1.1 analysis
- All refs confirmed: stepkit.go RenderTemplate:121 uses mustache.Render; WriteStepResult vars at :78; 45 byte-identical footers; 00-new.md:20 `{{command}}` bug.
- Only Go refs to footer text: templates/context_directive_test.go (delete) and templates/guided_add_conversation_test.go:314-357 (drop footer exclusion).
- Contract harness: step table from internal/steps/*/steps.go writeStep calls (spec 11, plan 19, implement 11, repo 9 templates; finished → ""). Skills installed via agent.Get("claude").Install(tmp, cfg, io.Discard) or installWorkflowSkills (unexported, agent pkg).
- Default cfg.Command = "spektacular" (config.NewDefault).

## Phase 1.1 implement
- Footer stripped from 45 templates by script (all byte-identical, preceded by "\n\n---\n\n"); partial `templates/partials/working-context-footer.md` has a leading standalone `{{! ... }}` comment line + footer paragraph.
- stepkit: RenderTemplate now uses mustache.RenderPartials with FSPartials{templates.FS}; mustache v1.4.0 propagates provider Get errors. Footer appended as TrimRight(instr,"\n")+"\n\n---\n\n"+footer when NextStep != "".

## Phase 1.1 tests
- cmd/instruction_contract_test.go harness: stepTemplateTable (50 rows), renderAllStepInstructions(t, command), renderedInstruction{workflow, stepName, templatePath, nextStep, body}. Agent lookup is `agent.Lookup(name)` (not agent.Get).
- `{{command}}` stepkit var has no test yet (no template uses it) — cover in Phase 2.1 when the plan-documents partial uses it.
- Phase 1.1 verify: all green (build, vet, gofmt, go test ./..., make lint = go vet only).
- Phase 1.1 complete: plan ticked, changelog section created. Looping phases without asking (user memory: drive workflows straight through).
- Helper: scratchpad tick.py <phase> <src> <dst> ticks a phase heading + its criteria.

## Phase 1.2 analysis
- Old-path refs confirmed as plan lists; plus test consts in cmd/instruction_contract_test.go:25 and internal/stepkit/stepkit_test.go:222 (footer oracles) need the new path. CHANGELOG.md mentions are historical — leave.
