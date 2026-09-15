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

## Phase 1.2 implement
- Renamed path in workingcontext, footer partial, spec 00-new/00b-interview/08-verification, plan 13-assemble, resume.md, 4 skills, historical-artifacts.md (2 lines). spec/steps.go comments + error text.
- Regenerated .claude/.bob/AGENTS.md via init claude + init bob: diff is rename-only.
- DOGFOOD: working context now lives in .spektacular/working-context.md (copied from old .spektacular/context.md, which is left for the user to delete). Write only to working-context.md from here on.
- skill_verify-implementation.md:15 bare `context.md` is Phase 2.3 work.
- Phase 1.2 tests done. cmd/instruction_contract_test.go now has installClaudeInto + workflowInstructionCorpus helpers; helper skills fetched via fetchSkillInstructions/skillProject (served raw; checked: none contain placeholders).
- Phase 1.2 verify: all green.
- Phase 1.3 analysis: docs unknown-criteria.mdx:54 is the only line; docs repo has unrelated M src/pages/knowledge-base.mdx.
- Phase 1.3 implement: line 54 changed; byte-identical to rendered footer paragraph. No automated test per plan.
- Phase 1.3 verify: docs build ok, astro check 0 errors, Rule 1 grep 0 matches, only unknown-criteria.mdx changed by us.
- Phase 2.1 analysis: no import cycle agent→stepkit (stepkit deps: store, output, workflow, templates). agent_test has withSourceFS helper for fixture FS. read_plan partial include at column 0.
- Phase 2.1 implement: partial templates/partials/implement-plan-documents.md (uses {{command}} + literal <plan_name>); included at column 0 in spek-implement SKILL.md (new `## The plan documents` under `# How to start`) and 01-read_plan.md Step 1. skills.go uses RenderPartials with stepkit.FSPartials{sourceFS}. Other renderers (commands.go wrappers, managed sections) don't render skills — unchanged. Regenerated .claude/.bob.
- Phase 2.1 tests done: normalizeIndent helper in cmd/instruction_contract_test.go; TestImplementStartListsPlanDocuments.
- Phase 2.1 verify: all green.
- Phase 2.2 analysis: resume.md shared template stays (rename only). cmd/resume_test.go implement row uses currentStep 'execute'. skill_resume_test.go reads raw skill templates.
- Phase 2.2 implement: templates/steps/resume_implement.md (partial indented 3 spaces under item 1 → renders indented); resumeInstruction picks it for kind implement, both renderers pass command. Skill resume step 2 is now a 4-item ordered sub-list referring to **The plan documents**. Regenerated .claude/.bob.
- Phase 2.2 tests done (resume_test.go, skill_resume_test.go). Cross-surface equality covered transitively via same rendered partial.
- Phase 2.2 verify: all green; live 'implement new' on this run emitted the new implement resume report, state untouched at verify.
- Phase 2.3 analysis: bare context.md occurrences listed via grep; scaffold/plan.md keeps its bare mentions (constraint: scaffold unchanged; it's a document scaffold, not an emitted instruction — check if any step renders it).
- Phase 2.3 implement: added 'Step 1: Load the current phase' block (both plan file read cmds) to 03-implement/04-test/05-verify; qualified bare context.md across implement 01/02, plan 03-18, spek-plan SKILL (16,33,45), skill_verify-implementation. scaffold/*.md untouched (not emitted instructions). Regenerated .claude/.bob.
- Phase 2.3 tests: TestPhaseStepsReadPhaseDetail, TestContextMdAlwaysQualified (+ unqualifiedContextMd checker test), agentFacingCorpus helper. Guard flags nothing.
- Phase 2.3 verify: go build/vet/gofmt/test all green. Harbor NOT run: docker daemon down (and it needs creds, ~25min each). Harbor AC left unchecked; carry to test plan + tell user at the end.
- All 6 phases done; advancing to test_plan.
- test_plan written (metrics 1, 2 manual + outstanding harbor run section).
- Feature changelogs written: project + --repo spektacular + --repo docs.
- reconcile_spec: all 12 requirements + 12 ACs ticked.
- Workflow finished. Nothing committed.
