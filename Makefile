BINARY := spektacular
VERSION := dev

HARBOR_AUTH := CLAUDE_CODE_OAUTH_TOKEN=$$(python3 -c "import json; print(json.load(open('$$HOME/.claude/.credentials.json'))['claudeAiOauth']['accessToken'])")
HARBOR_MODEL := claude-sonnet-5-5
# Installing the agent in a fresh container (apt packages, then the agent
# itself) takes two to four minutes against harbor's 360-second default, so a
# slow mirror fails a run before the agent starts. Double the allowance.
HARBOR_RUN_FLAGS := --agent-setup-timeout-multiplier 2

# How harbor runs each agent, for targets that take AGENT. Like Claude, Bob
# signs in with the login on this machine: tests/harbor/agents/bob_agent.py
# extends harbor's ACP agent to upload your Bob Shell login into the container
# before launching `bob acp`. Set BOBSHELL_API_KEY to use a key instead. Bob
# takes no -m: its ACP server cannot choose a model, and harbor fails the run
# when it cannot apply one.
HARBOR_ARGS_claude = -a claude-code -m $(HARBOR_MODEL)
HARBOR_ARGS_codex = -a codex -m $(HARBOR_MODEL)
HARBOR_ARGS_bob = -a bob_agent:BobAgent --ae BOB_LOG_LEVEL=debug $(if $(BOBSHELL_API_KEY),--ae BOBSHELL_API_KEY=$$BOBSHELL_API_KEY)
HARBOR_AUTH_claude = $(HARBOR_AUTH)
HARBOR_AUTH_codex = $(HARBOR_AUTH)
HARBOR_AUTH_bob = PYTHONPATH=$(CURDIR)/tests/harbor/agents

# Artifacts only the agent being run writes, such as its own log directory,
# appended to a rendered task's task.toml. They are declared only for that
# agent because `harbor trial regrade` refuses a trial in which a declared
# artifact failed to collect. Agents with no file of their own add nothing.
AGENT_ARTIFACTS = tests/harbor/agents/$(AGENT)-artifacts.toml

# Writes build.json into a rendered suite's tests directory: the run's identity
# for the metrics recorder (tests/harbor/shared/metrics.py), which the container
# cannot discover for itself. Call it as
# $(call write_build_info,<tests dir>,<suite>,<scenario, or empty for none>).
define write_build_info
printf '{"suite": "%s", "scenario": %s, "agent": "%s", "spektacular_version": "%s", "commit": "%s", "dirty": %s}\n' \
	'$(2)' '$(if $(3),"$(3)",null)' '$(AGENT)' "$$(git describe --tags --always)" "$$(git rev-parse HEAD)" \
	"$$(if [ -n "$$(git status --porcelain --untracked-files=no)" ]; then echo true; else echo false; fi)" \
	> $(1)/build.json
endef

.PHONY: build test lint clean install install-local cross harbor-install harbor-report harbor-test harbor-test-plan harbor-test-spec harbor-test-spec-claude harbor-test-spec-codex _harbor-test-spec harbor-test-implement harbor-test-repo harbor-test-repo-one-at-a-time harbor-test-repo-delegated _harbor-test-repo harbor-test-epic harbor-test-epic-claude harbor-test-epic-codex harbor-test-epic-bob _harbor-test-epic harbor-test-plan-claude harbor-test-plan-codex harbor-test-plan-bob _harbor-test-plan harbor-test-implement-epic harbor-test-implement-epic-claude harbor-test-implement-epic-codex harbor-test-implement-epic-bob harbor-test-implement-epic-bob-unlimited _harbor-test-implement-epic harbor-test-standing-rules harbor-test-standing-rules-claude harbor-test-standing-rules-codex harbor-test-standing-rules-bob _harbor-test-standing-rules harbor-test-goal-check harbor-test-goal-check-claude harbor-test-goal-check-codex harbor-test-goal-check-bob _harbor-test-goal-check harbor-test-background-child _harbor-test-background-child

build:
	go build -ldflags "-X github.com/hivecommons/spektacular/cmd.version=$(VERSION)" -o ./bin/$(BINARY) .

test:
	go test -shuffle=on ./...

lint:
	go vet ./...

clean:
	rm -f ./bin 

install-local: build
	sudo cp ./bin/$(BINARY) /usr/local/bin/$(BINARY)

dagger_build:
	dagger -v call --progress=plain -m dagger all \
		--output=./all_archive \
		--src=. \
		--notorize-cert=${QUILL_SIGN_P12} \
		--notorize-cert-password=QUILL_SIGN_PASSWORD \
		--notorize-key=${QUILL_NOTARY_KEY} \
		--notorize-id=${QUILL_NOTARY_KEY_ID} \
		--notorize-issuer=${QUILL_NOTARY_ISSUER}

#--github-token=GITHUB_TOKEN \

cross:
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o ./bin/$(BINARY)-darwin-arm64  .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o ./bin/$(BINARY)-darwin-amd64  .
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o ./bin/$(BINARY)-linux-amd64   .
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o ./bin/$(BINARY)-linux-arm64   .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ./bin/$(BINARY)-windows-amd64.exe .

# Installs the harbor CLI the harbor-test-* targets run, or upgrades it when
# already installed. Needs uv.
harbor-install:
	@command -v uv >/dev/null 2>&1 || (echo "uv is required to install harbor: https://docs.astral.sh/uv/getting-started/installation/" && exit 1)
	uv tool install --upgrade harbor

# Reports the metrics records collected harbor runs left in tests/harbor/jobs.
# It reads only each trial's verifier/metrics.json and result.json, so it
# needs no sudo for the root-owned files a run leaves. Pass options in ARGS:
#   make harbor-report                                # every run, by suite
#   make harbor-report ARGS='--run <trial>'           # one run's step breakdown
#   make harbor-report ARGS='tests/harbor/jobs/fix2-*'  # only some jobs
harbor-report:
	@python3 tests/harbor/shared/report.py $(ARGS)

harbor-test-spec: harbor-test-spec-claude

harbor-test-spec-claude:
	$(MAKE) _harbor-test-spec AGENT=claude HARBOR_AGENT=claude-code SPEK_NEW='/spek:new user-auth'

harbor-test-spec-codex:
	$(MAKE) _harbor-test-spec AGENT=codex HARBOR_AGENT=codex SPEK_NEW='$$$$spek-new user-auth'

# Renders tests/harbor/spec-workflow into tests/harbor/.build/spec-workflow-$(AGENT)
# with agent-specific placeholders substituted in instruction.md, then runs harbor.
# Callers must set AGENT, HARBOR_AGENT, SPEK_NEW.
_harbor-test-spec:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@mkdir -p tests/harbor/.build/spec-workflow-$(AGENT)/environment \
		tests/harbor/.build/spec-workflow-$(AGENT)/solution \
		tests/harbor/.build/spec-workflow-$(AGENT)/tests
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/spec-workflow-$(AGENT)/environment/spektacular .
	cp tests/harbor/spec-workflow/task.toml tests/harbor/.build/spec-workflow-$(AGENT)/task.toml
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/spec-workflow-$(AGENT)/task.toml; fi
	cp tests/harbor/spec-workflow/environment/Dockerfile tests/harbor/.build/spec-workflow-$(AGENT)/environment/Dockerfile
	cp tests/harbor/spec-workflow/solution/solve.sh tests/harbor/.build/spec-workflow-$(AGENT)/solution/solve.sh
	cp tests/harbor/spec-workflow/tests/test.sh tests/harbor/.build/spec-workflow-$(AGENT)/tests/test.sh
	cp tests/harbor/spec-workflow/tests/test_spec_workflow.py tests/harbor/.build/spec-workflow-$(AGENT)/tests/test_spec_workflow.py
	cp tests/harbor/shared/*.py tests/harbor/.build/spec-workflow-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/spec-workflow-$(AGENT)/tests,spec-workflow,)
	sed -e 's|{{agent}}|$(AGENT)|g' -e 's|{{spek_new}}|$(SPEK_NEW)|g' \
		tests/harbor/spec-workflow/instruction.md \
		> tests/harbor/.build/spec-workflow-$(AGENT)/instruction.md
	$(HARBOR_AUTH) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/spec-workflow-$(AGENT) -a $(HARBOR_AGENT) -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/spec-workflow-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-repo: harbor-test-repo-one-at-a-time

harbor-test-repo-one-at-a-time:
	$(MAKE) _harbor-test-repo SCENARIO=one-at-a-time AGENT=claude HARBOR_AGENT=claude-code SKILL='/spek:manage-repos'

harbor-test-repo-delegated:
	$(MAKE) _harbor-test-repo SCENARIO=delegated AGENT=claude HARBOR_AGENT=claude-code SKILL='/spek:manage-repos'

# Renders tests/harbor/repo-workflow into tests/harbor/.build/repo-workflow-$(SCENARIO),
# substituting the agent, the skill invocation and the scenario block into
# instruction.md, then runs harbor. The two scenarios share every other file:
# one answers each question in turn, the other hands the whole set over.
# Callers must set SCENARIO, AGENT, HARBOR_AGENT, SKILL.
_harbor-test-repo:
	@test -n "$(SCENARIO)" || (echo "SCENARIO is required" && exit 1)
	@mkdir -p tests/harbor/.build/repo-workflow-$(SCENARIO)/environment \
		tests/harbor/.build/repo-workflow-$(SCENARIO)/solution \
		tests/harbor/.build/repo-workflow-$(SCENARIO)/tests
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/repo-workflow-$(SCENARIO)/environment/spektacular .
	cp tests/harbor/repo-workflow/task.toml tests/harbor/.build/repo-workflow-$(SCENARIO)/task.toml
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/repo-workflow-$(SCENARIO)/task.toml; fi
	cp tests/harbor/repo-workflow/environment/Dockerfile tests/harbor/.build/repo-workflow-$(SCENARIO)/environment/Dockerfile
	cp tests/harbor/repo-workflow/solution/solve.sh tests/harbor/.build/repo-workflow-$(SCENARIO)/solution/solve.sh
	cp tests/harbor/repo-workflow/tests/test.sh tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/test.sh
	cp tests/harbor/repo-workflow/tests/test_repo_workflow.py tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/test_repo_workflow.py
	echo $(SCENARIO) > tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/scenario
	cp tests/harbor/shared/*.py tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/
	$(call write_build_info,tests/harbor/.build/repo-workflow-$(SCENARIO)/tests,repo-workflow,$(SCENARIO))
	SCENARIO_BODY=$$(cat tests/harbor/repo-workflow/scenario-$(SCENARIO).md); \
	awk -v scenario="$$SCENARIO_BODY" -v agent="$(AGENT)" -v skill="$(SKILL)" \
		'{ gsub(/\{\{agent\}\}/, agent); gsub(/\{\{skill_invocation\}\}/, skill); \
		   if ($$0 == "{{scenario}}") print scenario; else print }' \
		tests/harbor/repo-workflow/instruction.md \
		> tests/harbor/.build/repo-workflow-$(SCENARIO)/instruction.md
	SPEK_SCENARIO=$(SCENARIO) $(HARBOR_AUTH) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/repo-workflow-$(SCENARIO) -a $(HARBOR_AGENT) -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/repo-workflow-$(SCENARIO)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-plan: harbor-test-plan-claude

harbor-test-plan-claude:
	$(MAKE) _harbor-test-plan AGENT=claude SPEK_PLAN='/spek:plan 20260101000000-user-auth'

harbor-test-plan-codex:
	$(MAKE) _harbor-test-plan AGENT=codex SPEK_PLAN='$$$$spek-plan 20260101000000-user-auth'

harbor-test-plan-bob:
	$(MAKE) _harbor-test-plan AGENT=bob SPEK_PLAN='/spek-plan 20260101000000-user-auth'

# Renders tests/harbor/plan-workflow into tests/harbor/.build/plan-workflow-$(AGENT)
# with the agent and its skill invocation substituted into instruction.md, then
# runs harbor with that agent's arguments. The verifier reads a Claude Code
# transcript, or the trajectory harbor records for an ACP agent such as Bob.
# Callers must set AGENT and SPEK_PLAN.
_harbor-test-plan:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@rm -rf tests/harbor/.build/plan-workflow-$(AGENT)
	@mkdir -p tests/harbor/.build/plan-workflow-$(AGENT)
	cp -r tests/harbor/plan-workflow/environment tests/harbor/plan-workflow/solution tests/harbor/plan-workflow/tests \
		tests/harbor/plan-workflow/task.toml tests/harbor/.build/plan-workflow-$(AGENT)/
	cp tests/harbor/shared/*.py tests/harbor/.build/plan-workflow-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/plan-workflow-$(AGENT)/tests,plan-workflow,)
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/plan-workflow-$(AGENT)/task.toml; fi
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/plan-workflow-$(AGENT)/environment/spektacular .
	sed -e 's|{{agent}}|$(AGENT)|g' -e 's|{{spek_plan}}|$(SPEK_PLAN)|g' \
		tests/harbor/plan-workflow/instruction.md \
		> tests/harbor/.build/plan-workflow-$(AGENT)/instruction.md
	$(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/plan-workflow-$(AGENT) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/plan-workflow-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

# Renders tests/harbor/implement-workflow into
# tests/harbor/.build/implement-workflow-claude, with the shared helpers and
# build info in its tests, then runs harbor with Claude. Its verifier reads a
# Claude transcript, so Claude is the only agent it runs.
harbor-test-implement: AGENT = claude
harbor-test-implement:
	@rm -rf tests/harbor/.build/implement-workflow-$(AGENT)
	@mkdir -p tests/harbor/.build/implement-workflow-$(AGENT)
	cp -r tests/harbor/implement-workflow/environment tests/harbor/implement-workflow/tests \
		tests/harbor/implement-workflow/task.toml tests/harbor/implement-workflow/instruction.md \
		tests/harbor/.build/implement-workflow-$(AGENT)/
	cp tests/harbor/shared/*.py tests/harbor/.build/implement-workflow-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/implement-workflow-$(AGENT)/tests,implement-workflow,)
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/implement-workflow-$(AGENT)/environment/spektacular .
	$(HARBOR_AUTH) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/implement-workflow-$(AGENT) -a claude-code -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/implement-workflow-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-epic: harbor-test-epic-claude

harbor-test-epic-claude:
	$(MAKE) _harbor-test-epic AGENT=claude SPEK_PLAN_EPIC='/spek-plan-epic'

harbor-test-epic-codex:
	$(MAKE) _harbor-test-epic AGENT=codex SPEK_PLAN_EPIC='$$$$spek-plan-epic'

harbor-test-epic-bob:
	$(MAKE) _harbor-test-epic AGENT=bob SPEK_PLAN_EPIC='/spek-plan-epic'

# Renders tests/harbor/epic-workflow into tests/harbor/.build/epic-workflow-$(AGENT)
# with the agent and its skill invocation substituted into instruction.md, then
# runs harbor with that agent's arguments. The verifier reads only CLI state and
# the session log, so it is the same for every agent. Its timeline of the run is
# at the top of the results. Callers must set AGENT and SPEK_PLAN_EPIC.
_harbor-test-epic:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@rm -rf tests/harbor/.build/epic-workflow-$(AGENT)
	@mkdir -p tests/harbor/.build/epic-workflow-$(AGENT)
	cp -r tests/harbor/epic-workflow/environment tests/harbor/epic-workflow/tests \
		tests/harbor/epic-workflow/task.toml tests/harbor/.build/epic-workflow-$(AGENT)/
	cp tests/harbor/shared/*.py tests/harbor/.build/epic-workflow-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/epic-workflow-$(AGENT)/tests,epic-workflow,)
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/epic-workflow-$(AGENT)/task.toml; fi
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/epic-workflow-$(AGENT)/environment/spektacular .
	sed -e 's|{{agent}}|$(AGENT)|g' -e 's|{{spek_plan_epic}}|$(SPEK_PLAN_EPIC)|g' \
		tests/harbor/epic-workflow/instruction.md \
		> tests/harbor/.build/epic-workflow-$(AGENT)/instruction.md
	$(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/epic-workflow-$(AGENT) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/epic-workflow-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-standing-rules: harbor-test-standing-rules-claude

harbor-test-standing-rules-claude:
	$(MAKE) _harbor-test-standing-rules AGENT=claude

harbor-test-standing-rules-codex:
	$(MAKE) _harbor-test-standing-rules AGENT=codex

harbor-test-standing-rules-bob:
	$(MAKE) _harbor-test-standing-rules AGENT=bob

# Renders tests/harbor/standing-rules into
# tests/harbor/.build/standing-rules-$(AGENT), with the agent substituted into
# the seed, which runs `spektacular init $(AGENT)` while the image is built so
# the standing rules in AGENTS.md are the agent's project instructions from its
# first turn. The verifier reads only CLI state, the session log and the
# agent's answer file, so it is the same for every agent. Only the verifier is
# copied into tests: its self-check runs locally, never in the image.
# Callers must set AGENT.
_harbor-test-standing-rules:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@rm -rf tests/harbor/.build/standing-rules-$(AGENT)
	@mkdir -p tests/harbor/.build/standing-rules-$(AGENT)/tests
	cp -r tests/harbor/standing-rules/environment tests/harbor/standing-rules/task.toml \
		tests/harbor/.build/standing-rules-$(AGENT)/
	cp tests/harbor/standing-rules/tests/test.sh tests/harbor/standing-rules/tests/test_standing_rules.py \
		tests/harbor/.build/standing-rules-$(AGENT)/tests/
	cp tests/harbor/shared/*.py tests/harbor/.build/standing-rules-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/standing-rules-$(AGENT)/tests,standing-rules,)
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/standing-rules-$(AGENT)/task.toml; fi
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/.build/standing-rules-$(AGENT)/environment/spektacular .
	sed -e 's|{{agent}}|$(AGENT)|g' tests/harbor/standing-rules/environment/seed/seed.sh \
		> tests/harbor/.build/standing-rules-$(AGENT)/environment/seed/seed.sh
	sed -e 's|{{agent}}|$(AGENT)|g' tests/harbor/standing-rules/instruction.md \
		> tests/harbor/.build/standing-rules-$(AGENT)/instruction.md
	$(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/standing-rules-$(AGENT) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/standing-rules-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

# The background-child probe: one background child that sleeps for fifteen
# minutes, to learn whether a headless Claude run lets it outlive ten minutes
# and whether its completion notice starts a new turn. Renders
# tests/harbor/background-child into
# tests/harbor/.build/background-child-$(AGENT). Its verifier reports the
# outcome either way. Claude only: other agents' logs cannot show it.
harbor-test-background-child:
	$(MAKE) _harbor-test-background-child AGENT=claude

# Callers must set AGENT.
_harbor-test-background-child:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@rm -rf tests/harbor/.build/background-child-$(AGENT)
	@mkdir -p tests/harbor/.build/background-child-$(AGENT)/tests
	cp -r tests/harbor/background-child/environment tests/harbor/background-child/task.toml \
		tests/harbor/background-child/instruction.md tests/harbor/.build/background-child-$(AGENT)/
	cp tests/harbor/background-child/tests/test.sh tests/harbor/background-child/tests/test_background_child.py \
		tests/harbor/.build/background-child-$(AGENT)/tests/
	cp tests/harbor/shared/*.py tests/harbor/.build/background-child-$(AGENT)/tests/
	$(call write_build_info,tests/harbor/.build/background-child-$(AGENT)/tests,background-child,)
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> tests/harbor/.build/background-child-$(AGENT)/task.toml; fi
	$(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/background-child-$(AGENT) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/background-child-$(AGENT)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-implement-epic: harbor-test-implement-epic-claude

harbor-test-implement-epic-claude:
	$(MAKE) _harbor-test-implement-epic AGENT=claude SPEK_IMPLEMENT_EPIC='/spek-implement-epic'

harbor-test-implement-epic-codex:
	$(MAKE) _harbor-test-implement-epic AGENT=codex SPEK_IMPLEMENT_EPIC='$$$$spek-implement-epic'

# Bob with its default turn limit of 100 per task, sub-agents included: the
# setup that cuts epic children off mid-step.
harbor-test-implement-epic-bob:
	$(MAKE) _harbor-test-implement-epic AGENT=bob SPEK_IMPLEMENT_EPIC='/spek-implement-epic'

# Bob with no turn limit (session.maxTurns 0), to compare against the default.
harbor-test-implement-epic-bob-unlimited:
	BOB_MAX_TURNS=0 $(MAKE) _harbor-test-implement-epic AGENT=bob VARIANT=-unlimited SPEK_IMPLEMENT_EPIC='/spek-implement-epic'

# Renders tests/harbor/implement-epic-workflow into
# tests/harbor/.build/implement-epic-workflow-$(AGENT)$(VARIANT), with the shared
# session-log helpers copied into its tests and the agent and its skill
# invocation substituted into instruction.md, then runs harbor with that
# agent's arguments. The verifier reads only CLI state, git, the app's behaviour
# and the session log, so it is the same for every agent; its timeline of the
# run is at the top of the results. Callers must set AGENT and
# SPEK_IMPLEMENT_EPIC; VARIANT names a second build of the same agent. Harbor
# cuts a trial's name to 32 characters, so the results are found by that prefix.
IMPLEMENT_EPIC_BUILD = tests/harbor/.build/implement-epic-workflow-$(AGENT)$(VARIANT)
_harbor-test-implement-epic:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@rm -rf $(IMPLEMENT_EPIC_BUILD)
	@mkdir -p $(IMPLEMENT_EPIC_BUILD)
	cp -r tests/harbor/implement-epic-workflow/environment tests/harbor/implement-epic-workflow/solution \
		tests/harbor/implement-epic-workflow/tests tests/harbor/implement-epic-workflow/task.toml $(IMPLEMENT_EPIC_BUILD)/
	cp tests/harbor/shared/*.py $(IMPLEMENT_EPIC_BUILD)/tests/
	$(call write_build_info,$(IMPLEMENT_EPIC_BUILD)/tests,implement-epic-workflow,$(patsubst -%,%,$(VARIANT)))
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> $(IMPLEMENT_EPIC_BUILD)/task.toml; fi
	GOOS=linux GOARCH=amd64 go build -o $(IMPLEMENT_EPIC_BUILD)/environment/spektacular .
	sed -e 's|{{agent}}|$(AGENT)|g' -e 's|{{spek_implement_epic}}|$(SPEK_IMPLEMENT_EPIC)|g' \
		tests/harbor/implement-epic-workflow/instruction.md \
		> $(IMPLEMENT_EPIC_BUILD)/instruction.md
	$(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p $(IMPLEMENT_EPIC_BUILD) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/$$(echo implement-epic-workflow-$(AGENT)$(VARIANT) | cut -c1-32)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

# The goal-check suite: an implement run resumed at check_goal for one task whose
# README criterion is missing while its tests pass (SCENARIO=unmet, the
# default for the per-agent targets) or whose criteria all hold (SCENARIO=met). Renders
# tests/harbor/goal-check-workflow into
# tests/harbor/.build/goal-check-workflow-$(AGENT)-$(SCENARIO), with the
# scenario baked into the seed and the verifier, the shared session-log and
# transcript helpers copied into its tests, and the agent, its skill
# invocation and the scenario's text substituted into instruction.md. The
# verifier's self-check (tests/test_verifier_selfcheck.py) stays out of the
# image; run it locally with uvx --with pytest pytest.
harbor-test-goal-check: harbor-test-goal-check-claude

harbor-test-goal-check-claude:
	$(MAKE) _harbor-test-goal-check AGENT=claude SCENARIO=$(or $(SCENARIO),unmet) SPEK_IMPLEMENT='/spek-implement 20260301000000-todo-count'

harbor-test-goal-check-codex:
	$(MAKE) _harbor-test-goal-check AGENT=codex SCENARIO=$(or $(SCENARIO),unmet) SPEK_IMPLEMENT='$$$$spek-implement 20260301000000-todo-count'

harbor-test-goal-check-bob:
	$(MAKE) _harbor-test-goal-check AGENT=bob SCENARIO=$(or $(SCENARIO),unmet) SPEK_IMPLEMENT='/spek-implement 20260301000000-todo-count'

GOAL_CHECK_BUILD = tests/harbor/.build/goal-check-workflow-$(AGENT)-$(SCENARIO)
_harbor-test-goal-check:
	@test -n "$(AGENT)" || (echo "AGENT is required" && exit 1)
	@test -n "$(HARBOR_ARGS_$(AGENT))" || (echo "no harbor arguments for AGENT=$(AGENT)" && exit 1)
	@test -f tests/harbor/goal-check-workflow/scenario-$(SCENARIO).md || (echo "SCENARIO must be unmet or met" && exit 1)
	@rm -rf $(GOAL_CHECK_BUILD)
	@mkdir -p $(GOAL_CHECK_BUILD)/tests
	cp -r tests/harbor/goal-check-workflow/environment tests/harbor/goal-check-workflow/solution \
		tests/harbor/goal-check-workflow/task.toml $(GOAL_CHECK_BUILD)/
	cp tests/harbor/goal-check-workflow/tests/test.sh tests/harbor/goal-check-workflow/tests/test_goal_check.py $(GOAL_CHECK_BUILD)/tests/
	cp tests/harbor/shared/*.py $(GOAL_CHECK_BUILD)/tests/
	$(call write_build_info,$(GOAL_CHECK_BUILD)/tests,goal-check-workflow,$(SCENARIO))
	echo $(SCENARIO) > $(GOAL_CHECK_BUILD)/environment/seed/scenario
	echo $(AGENT) > $(GOAL_CHECK_BUILD)/environment/seed/agent
	echo $(SCENARIO) > $(GOAL_CHECK_BUILD)/tests/scenario
	@if [ -f $(AGENT_ARTIFACTS) ]; then cat $(AGENT_ARTIFACTS) >> $(GOAL_CHECK_BUILD)/task.toml; fi
	GOOS=linux GOARCH=amd64 go build -o $(GOAL_CHECK_BUILD)/environment/spektacular .
	SCENARIO_BODY=$$(cat tests/harbor/goal-check-workflow/scenario-$(SCENARIO).md); \
	awk -v scenario="$$SCENARIO_BODY" -v agent="$(AGENT)" -v skill="$(SPEK_IMPLEMENT)" \
		'{ gsub(/\{\{agent\}\}/, agent); gsub(/\{\{spek_implement\}\}/, skill); \
		   if ($$0 == "{{scenario}}") print scenario; else print }' \
		tests/harbor/goal-check-workflow/instruction.md \
		> $(GOAL_CHECK_BUILD)/instruction.md
	SPEK_SCENARIO=$(SCENARIO) $(HARBOR_AUTH_$(AGENT)) harbor run $(HARBOR_RUN_FLAGS) -p $(GOAL_CHECK_BUILD) $(HARBOR_ARGS_$(AGENT)) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/$$(echo goal-check-workflow-$(AGENT)-$(SCENARIO) | cut -c1-32)__*/verifier/test-stdout.txt 2>/dev/null | head -1)
