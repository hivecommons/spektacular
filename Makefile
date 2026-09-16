BINARY := spektacular
VERSION := 0.15.1

HARBOR_AUTH := CLAUDE_CODE_OAUTH_TOKEN=$$(python3 -c "import json; print(json.load(open('$$HOME/.claude/.credentials.json'))['claudeAiOauth']['accessToken'])")
HARBOR_MODEL := claude-sonnet-4-6

.PHONY: build test lint clean install install-local cross harbor-test plan-harbor-test harbor-test-spec harbor-test-spec-claude harbor-test-spec-codex _harbor-test-spec harbor-test-implement harbor-test-repo harbor-test-repo-one-at-a-time harbor-test-repo-delegated _harbor-test-repo

build:
	go build -ldflags "-X github.com/jumppad-labs/spektacular/cmd.version=$(VERSION)" -o ./bin/$(BINARY) .

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
	cp tests/harbor/spec-workflow/environment/Dockerfile tests/harbor/.build/spec-workflow-$(AGENT)/environment/Dockerfile
	cp tests/harbor/spec-workflow/solution/solve.sh tests/harbor/.build/spec-workflow-$(AGENT)/solution/solve.sh
	cp tests/harbor/spec-workflow/tests/test.sh tests/harbor/.build/spec-workflow-$(AGENT)/tests/test.sh
	cp tests/harbor/spec-workflow/tests/test_spec_workflow.py tests/harbor/.build/spec-workflow-$(AGENT)/tests/test_spec_workflow.py
	sed -e 's|{{agent}}|$(AGENT)|g' -e 's|{{spek_new}}|$(SPEK_NEW)|g' \
		tests/harbor/spec-workflow/instruction.md \
		> tests/harbor/.build/spec-workflow-$(AGENT)/instruction.md
	$(HARBOR_AUTH) harbor run -p tests/harbor/.build/spec-workflow-$(AGENT) -a $(HARBOR_AGENT) -m $(HARBOR_MODEL) -o tests/harbor/jobs
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
	cp tests/harbor/repo-workflow/environment/Dockerfile tests/harbor/.build/repo-workflow-$(SCENARIO)/environment/Dockerfile
	cp tests/harbor/repo-workflow/solution/solve.sh tests/harbor/.build/repo-workflow-$(SCENARIO)/solution/solve.sh
	cp tests/harbor/repo-workflow/tests/test.sh tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/test.sh
	cp tests/harbor/repo-workflow/tests/test_repo_workflow.py tests/harbor/.build/repo-workflow-$(SCENARIO)/tests/test_repo_workflow.py
	SCENARIO_BODY=$$(cat tests/harbor/repo-workflow/scenario-$(SCENARIO).md); \
	awk -v scenario="$$SCENARIO_BODY" -v agent="$(AGENT)" -v skill="$(SKILL)" \
		'{ gsub(/\{\{agent\}\}/, agent); gsub(/\{\{skill_invocation\}\}/, skill); \
		   if ($$0 == "{{scenario}}") print scenario; else print }' \
		tests/harbor/repo-workflow/instruction.md \
		> tests/harbor/.build/repo-workflow-$(SCENARIO)/instruction.md
	SPEK_SCENARIO=$(SCENARIO) $(HARBOR_AUTH) harbor run -p tests/harbor/.build/repo-workflow-$(SCENARIO) -a $(HARBOR_AGENT) -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/repo-workflow-$(SCENARIO)__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-plan:
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/plan-workflow/environment/spektacular .
	$(HARBOR_AUTH) harbor run -p tests/harbor/plan-workflow -a claude-code -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/plan-workflow__*/verifier/test-stdout.txt 2>/dev/null | head -1)

harbor-test-implement:
	GOOS=linux GOARCH=amd64 go build -o tests/harbor/implement-workflow/environment/spektacular .
	$(HARBOR_AUTH) harbor run -p tests/harbor/implement-workflow -a claude-code -m $(HARBOR_MODEL) -o tests/harbor/jobs
	@echo ""
	@echo "=== Test Results ==="
	@cat $$(ls -td tests/harbor/jobs/*/implement-workflow__*/verifier/test-stdout.txt 2>/dev/null | head -1)
