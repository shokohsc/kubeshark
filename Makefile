SHELL=/bin/bash

.PHONY: help
.DEFAULT_GOAL := build
.ONESHELL:

SUFFIX=$(GOOS)_$(GOARCH)
COMMIT_HASH=$(shell git rev-parse HEAD)
GIT_BRANCH=$(shell git branch --show-current | tr '[:upper:]' '[:lower:]')
GIT_VERSION=$(shell git branch --show-current | tr '[:upper:]' '[:lower:]')
BUILD_TIMESTAMP=$(shell date +%s)
export VER?=0.0.0

help: ## Print this help message.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build-debug:  ## Build for debugging.
	export CGO_ENABLED=1
	export GCLFAGS='-gcflags="all=-N -l"'
	${MAKE} build-base

build: ## Build.
	export CGO_ENABLED=0
	export LDFLAGS_EXT='-extldflags=-static -s -w'
	${MAKE} build-base

build-race: ## Build with -race flag.
	export CGO_ENABLED=1
	export GCLFAGS='-race'
	export LDFLAGS_EXT='-extldflags=-static -s -w'
	${MAKE} build-base

build-base: ## Build binary (select the platform via GOOS / GOARCH env variables).
	go build ${GCLFAGS} -ldflags="${LDFLAGS_EXT} \
					-X 'github.com/kubeshark/kubeshark/misc.GitCommitHash=$(COMMIT_HASH)' \
					-X 'github.com/kubeshark/kubeshark/misc.Branch=$(GIT_BRANCH)' \
					-X 'github.com/kubeshark/kubeshark/misc.BuildTimestamp=$(BUILD_TIMESTAMP)' \
					-X 'github.com/kubeshark/kubeshark/misc.Platform=$(SUFFIX)' \
					-X 'github.com/kubeshark/kubeshark/misc.Ver=$(VER)'" \
					-o bin/kubeshark_$(SUFFIX) kubeshark.go && \
	cd bin && shasum -a 256 kubeshark_${SUFFIX} > kubeshark_${SUFFIX}.sha256

build-brew: ## Build binary for brew/core CI
	go build ${GCLFAGS} -ldflags="${LDFLAGS_EXT} \
					-X 'github.com/kubeshark/kubeshark/misc.GitCommitHash=$(COMMIT_HASH)' \
					-X 'github.com/kubeshark/kubeshark/misc.Branch=$(GIT_BRANCH)' \
					-X 'github.com/kubeshark/kubeshark/misc.BuildTimestamp=$(BUILD_TIMESTAMP)' \
					-X 'github.com/kubeshark/kubeshark/misc.Platform=$(SUFFIX)' \
					-X 'github.com/kubeshark/kubeshark/misc.Ver=$(VER)'" \
					-o kubeshark kubeshark.go

build-windows-amd64:
	$(MAKE) build GOOS=windows GOARCH=amd64 && \
	mv ./bin/kubeshark_windows_amd64 ./bin/kubeshark.exe && \
	rm bin/kubeshark_windows_amd64.sha256 && \
	cd bin && shasum -a 256 kubeshark.exe > kubeshark.exe.sha256

build-all: ## Build for all supported platforms.
	export CGO_ENABLED=0
	echo "Compiling for every OS and Platform" && \
	mkdir -p bin && sed s/_VER_/$(VER)/g RELEASE.md.TEMPLATE >  bin/README.md && \
	$(MAKE) build GOOS=linux GOARCH=amd64 && \
	$(MAKE) build GOOS=linux GOARCH=arm64 && \
	$(MAKE) build GOOS=darwin GOARCH=amd64 && \
	$(MAKE) build GOOS=darwin GOARCH=arm64 && \
	$(MAKE) build-windows-amd64 && \
	echo "---------" && \
	find ./bin -ls

clean: ## Clean all build artifacts.
	go clean
	rm -rf ./bin/*

test: ## Run cli tests.
	@go test ./... -coverpkg=./... -race -coverprofile=coverage.out -covermode=atomic


hub-coverage: ## Enforce 80% hub test coverage.
	go test ./hub/... -coverpkg=./hub/... -covermode=atomic -coverprofile=hub_coverage.out -race
	@total=$$(go tool cover -func=hub_coverage.out | tail -1 | awk '{print $$3}' | tr -d '%%'); \
	echo "hub coverage: $$total%%"; \
	awk -v t="$$total" 'BEGIN { exit !(t >= 80) }'

test-integration: ## Run integration tests (requires Kubernetes cluster).
	@echo "Running integration tests..."
	@LOG_FILE=$$(mktemp /tmp/integration-test.XXXXXX.log); \
	go test -tags=integration -timeout $${INTEGRATION_TIMEOUT:-5m} -v ./integration/... 2>&1 | tee $$LOG_FILE; \
	status=$$?; \
	echo ""; \
	echo "========================================"; \
	echo "         INTEGRATION TEST SUMMARY"; \
	echo "========================================"; \
	grep -E "^(--- PASS|--- FAIL|--- SKIP)" $$LOG_FILE || true; \
	echo "----------------------------------------"; \
	pass=$$(grep -c "^--- PASS" $$LOG_FILE 2>/dev/null || true); \
	fail=$$(grep -c "^--- FAIL" $$LOG_FILE 2>/dev/null || true); \
	skip=$$(grep -c "^--- SKIP" $$LOG_FILE 2>/dev/null || true); \
	echo "PASSED:  $${pass:-0}"; \
	echo "FAILED:  $${fail:-0}"; \
	echo "SKIPPED: $${skip:-0}"; \
	echo "========================================"; \
	rm -f $$LOG_FILE; \
	exit $$status

test-integration-mcp: ## Run only MCP integration tests.
	@echo "Running MCP integration tests..."
	@LOG_FILE=$$(mktemp /tmp/integration-test.XXXXXX.log); \
	go test -tags=integration -timeout $${INTEGRATION_TIMEOUT:-5m} -v ./integration/ -run "MCP" 2>&1 | tee $$LOG_FILE; \
	status=$$?; \
	echo ""; \
	echo "========================================"; \
	echo "         INTEGRATION TEST SUMMARY"; \
	echo "========================================"; \
	grep -E "^(--- PASS|--- FAIL|--- SKIP)" $$LOG_FILE || true; \
	echo "----------------------------------------"; \
	pass=$$(grep -c "^--- PASS" $$LOG_FILE 2>/dev/null || true); \
	fail=$$(grep -c "^--- FAIL" $$LOG_FILE 2>/dev/null || true); \
	skip=$$(grep -c "^--- SKIP" $$LOG_FILE 2>/dev/null || true); \
	echo "PASSED:  $${pass:-0}"; \
	echo "FAILED:  $${fail:-0}"; \
	echo "SKIPPED: $${skip:-0}"; \
	echo "========================================"; \
	rm -f $$LOG_FILE; \
	exit $$status

test-integration-short: ## Run quick integration tests (skips long-running tests).
	@echo "Running quick integration tests..."
	@LOG_FILE=$$(mktemp /tmp/integration-test.XXXXXX.log); \
	go test -tags=integration -timeout $${INTEGRATION_TIMEOUT:-2m} -short -v ./integration/... 2>&1 | tee $$LOG_FILE; \
	status=$$?; \
	echo ""; \
	echo "========================================"; \
	echo "         INTEGRATION TEST SUMMARY"; \
	echo "========================================"; \
	grep -E "^(--- PASS|--- FAIL|--- SKIP)" $$LOG_FILE || true; \
	echo "----------------------------------------"; \
	pass=$$(grep -c "^--- PASS" $$LOG_FILE 2>/dev/null || true); \
	fail=$$(grep -c "^--- FAIL" $$LOG_FILE 2>/dev/null || true); \
	skip=$$(grep -c "^--- SKIP" $$LOG_FILE 2>/dev/null || true); \
	echo "PASSED:  $${pass:-0}"; \
	echo "FAILED:  $${fail:-0}"; \
	echo "SKIPPED: $${skip:-0}"; \
	echo "========================================"; \
	rm -f $$LOG_FILE; \
	exit $$status

lint: ## Lint the source code.
	golangci-lint run

kubectl-view-all-resources: ## This command outputs all Kubernetes resources using YAML format and pipes it to VS Code
	./kubectl.sh view-all-resources

kubectl-view-kubeshark-resources: ## This command outputs all Kubernetes resources in "kubeshark" namespace using YAML format and pipes it to VS Code
	./kubectl.sh view-kubeshark-resources

logs-sniffer:
	export LOGS_POD_PREFIX=kubeshark-worker-
	export LOGS_CONTAINER='-c sniffer'
	export LOGS_FOLLOW=
	${MAKE} logs

logs-sniffer-follow:
	export LOGS_POD_PREFIX=kubeshark-worker-
	export LOGS_CONTAINER='-c sniffer'
	export LOGS_FOLLOW=--follow
	${MAKE} logs

logs-tracer:
	export LOGS_POD_PREFIX=kubeshark-worker-
	export LOGS_CONTAINER='-c tracer'
	export LOGS_FOLLOW=
	${MAKE} logs

logs-tracer-follow:
	export LOGS_POD_PREFIX=kubeshark-worker-
	export LOGS_CONTAINER='-c tracer'
	export LOGS_FOLLOW=--follow
	${MAKE} logs

logs-worker: logs-sniffer

logs-worker-follow: logs-sniffer-follow

logs-hub:
	export LOGS_POD_PREFIX=kubeshark-hub
	export LOGS_FOLLOW=
	${MAKE} logs

logs-hub-follow:
	export LOGS_POD_PREFIX=kubeshark-hub
	export LOGS_FOLLOW=--follow
	${MAKE} logs

logs-front:
	export LOGS_POD_PREFIX=kubeshark-front
	export LOGS_FOLLOW=
	${MAKE} logs

logs-front-follow:
	export LOGS_POD_PREFIX=kubeshark-front
	export LOGS_FOLLOW=--follow
	${MAKE} logs

logs:
	kubectl logs $$(kubectl get pods | awk '$$1 ~ /^$(LOGS_POD_PREFIX)/' | awk 'END {print $$1}') $(LOGS_CONTAINER) $(LOGS_FOLLOW)

ssh-node:
	kubectl ssh node $$(kubectl get nodes | awk 'END {print $$1}')

exec-worker:
	export EXEC_POD_PREFIX=kubeshark-worker-
	${MAKE} exec

exec-hub:
	export EXEC_POD_PREFIX=kubeshark-hub
	${MAKE} exec

exec-front:
	export EXEC_POD_PREFIX=kubeshark-front
	${MAKE} exec

exec:
	kubectl exec --stdin --tty $$(kubectl get pods | awk '$$1 ~ /^$(EXEC_POD_PREFIX)/' | awk 'END {print $$1}') -- /bin/sh

proxy:
	kubeshark proxy

port-forward:
	kubectl port-forward $$(kubectl get pods | awk '$$1 ~ /^$(POD_PREFIX)/' | awk 'END {print $$1}') $(SRC_PORT):$(DST_PORT)

release: ## Print release workflow instructions.
	@echo "Release workflow — each step is idempotent and can be rerun on its own:"
	@echo ""
	@echo "  1. make release-siblings VERSION=x.y.z"
	@echo "     Tag worker, hub, front with vx.y.z. Also run standalone when"
	@echo "     rebuilding docker images without cutting a full release."
	@echo ""
	@echo "  2. make release-pr-kubeshark VERSION=x.y.z"
	@echo "     Build and open the kubeshark release PR."
	@echo ""
	@echo "  Shortcut: make release-pr VERSION=x.y.z runs 1 → 2."
	@echo ""
	@echo "  After both PRs merge, create the release tag:"
	@echo "  make release-tag VERSION=x.y.z"

# Internal: validate VERSION before any release-* target runs.
_release-check-version:
	@if [ -z "$(VERSION)" ]; then echo "ERROR: VERSION is required. Usage: make <target> VERSION=x.y.z"; exit 1; fi
	@echo "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+' || { echo "ERROR: VERSION must be semver (e.g. 53.2.4)"; exit 1; }

release-siblings: _release-check-version ## Tag worker, hub, front with v$(VERSION). Idempotent; standalone for docker-image-only updates.
	@for repo in worker hub front; do \
		echo "==> $$repo: ensuring v$(VERSION) tag"; \
		(cd ../$$repo && git checkout master && git pull) || exit 1; \
		if (cd ../$$repo && git ls-remote --tags origin "refs/tags/v$(VERSION)" | grep -q .); then \
			echo "    v$(VERSION) already on origin — skipping"; \
		else \
			(cd ../$$repo && git tag -d v$(VERSION) 2>/dev/null; git tag v$(VERSION) && git push origin "refs/tags/v$(VERSION)") || exit 1; \
		fi; \
	done

release-pr-kubeshark: _release-check-version ## Build and open the kubeshark release PR.
	@cd ../kubeshark && git checkout master && git pull
	@$(MAKE) build VER=$(VERSION)
	@if [ "$(shell uname)" = "Darwin" ]; then \
		codesign --sign - --force --preserve-metadata=entitlements,requirements,flags,runtime ./bin/kubeshark__; \
	fi
	@if git show-ref --verify --quiet refs/heads/release/v$(VERSION); then \
		git branch -D release/v$(VERSION); \
	fi
	@git checkout -b release/v$(VERSION)
	@git add -A .
	@if ! git diff --cached --quiet; then \
		git commit -m ":bookmark: Release v$(VERSION)"; \
	else \
		echo "nothing to commit"; \
	fi
	@git push --force-with-lease -u origin release/v$(VERSION)
	@if gh pr view release/v$(VERSION) --json number >/dev/null 2>&1; then \
		echo "PR already exists for release/v$(VERSION)"; \
	else \
		gh pr create --title ":bookmark: Release v$(VERSION)" \
			--body "Automated release PR for v$(VERSION)." \
			--base master \
			--reviewer corest; \
	fi

release-pr: release-siblings release-pr-kubeshark ## Run release-siblings and release-pr-kubeshark in sequence.
	@echo ""
	@echo "Release PRs created (or already present):"
	@echo "  - kubeshark: Review and merge the release PR."
	@echo "Tag will be created automatically, or run: make release-tag VERSION=$(VERSION)"

release-tag: _release-check-version ## Step 2: Tag master after release PR is merged. Idempotent; re-run to retrigger the release build.
	@echo "Verifying release PR was merged..."
	@if ! gh pr list --state merged --head release/v$(VERSION) --json number --jq '.[0].number' | grep -q .; then \
		echo "Error: No merged PR found for release/v$(VERSION). Merge the PR first."; \
		exit 1; \
	fi
	@git checkout master && git pull
	@if git ls-remote --tags origin "refs/tags/v$(VERSION)" | grep -q .; then \
		echo "Tag v$(VERSION) already exists on origin — deleting to retrigger release..."; \
		git push origin :refs/tags/v$(VERSION); \
	fi
	@git tag -d v$(VERSION) 2>/dev/null; git tag v$(VERSION) && git push origin "refs/tags/v$(VERSION)"
	@echo ""
	@echo "Tagged v$(VERSION) on master. GitHub Actions will build the release."

release-dry-run:
	@cd ../worker && git checkout master && git pull 
	# @cd ../tracer && git checkout master && git pull 
	@cd ../hub && git checkout master && git pull
	@cd ../front && git checkout master && git pull 
	# @if [ "$(shell uname)" = "Darwin" ]; then \
	# 	codesign --sign - --force --preserve-metadata=entitlements,requirements,flags,runtime ./bin/kubeshark__; \
	# fi
	@cd ../kubeshark.github.io/
	@cd ../kubeshark

branch:
	@cd ../worker && git checkout master && git pull && git checkout -b $(name); git push --set-upstream origin $(name)
	@cd ../hub && git checkout master && git pull && git checkout -b $(name); git push --set-upstream origin $(name)
	@cd ../front && git checkout master && git pull && git checkout -b $(name); git push --set-upstream origin $(name)

switch-to-branch:
	@cd ../worker && git checkout $(name)
	@cd ../hub && git checkout $(name)
	@cd ../front && git checkout $(name)
