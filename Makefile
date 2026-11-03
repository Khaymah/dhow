GO       ?= go
export GOTOOLCHAIN := $(shell awk '/^toolchain/ {print $$2}' go.mod)
PKG      := ./...
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
CHECKOUT_IMAGE ?=
LDFLAGS  := -s -w \
	-X github.com/Khaymah/dhow/internal/buildinfo.version=$(VERSION) \
	-X github.com/Khaymah/dhow/internal/buildinfo.commit=$(COMMIT) \
	-X github.com/Khaymah/dhow/internal/buildinfo.checkoutImage=$(CHECKOUT_IMAGE)
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.8.0
ACTIONLINT_VERSION    := v1.7.12
OAPI_CODEGEN_VERSION  := v2.8.1
SQLC_VERSION          := v1.31.1
PROTOBUF_VERSION      := v1.36.12
CONNECT_VERSION       := v1.21.0
BUF_VERSION           := v1.73.0
BUF                   := $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
TOOLS_DIR             := bin/tools
PROTO_DIR             := packages/proto
PROTO_AGAINST_BRANCH  ?= main
PROTO_AGAINST         ?= $(shell git rev-parse --git-common-dir 2>/dev/null)\#branch=$(PROTO_AGAINST_BRANCH),subdir=$(PROTO_DIR)
WEB_DIST              := apps/server/internal/App/WebDist
IMAGE                 ?= dhow:dev
GIT_IMAGE             ?= dhow-git:dev
E2E_BASE              ?= debian:12
RUNNER_E2E_BASE       ?= debian:13
DISTRO                ?=
IMAGE_DOCKERFILE      ?= deploy/docker/Dockerfile
DOCKER_BUILD_FLAGS    ?=
COMPOSE_DIR           := deploy/compose
COMPOSE               := docker compose --project-directory $(COMPOSE_DIR) -f $(COMPOSE_DIR)/compose.yaml

.PHONY: build
build: ## Build bin/dhow-server
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/dhow-server ./apps/server/cmd/dhow-server

.PHONY: build-runner
build-runner: ## Build bin/dhow-runner (Linux only)
	CGO_ENABLED=0 GOOS=linux $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/dhow-runner ./apps/runner/cmd/dhow-runner

.PHONY: build-cli
build-cli: ## Build bin/dhow, the dhow CLI (dhow lint), for this machine
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/dhow ./apps/cli/cmd/dhow

.PHONY: web
web: ## Build the web app and copy it into the server embed directory
	pnpm -C apps/web install --frozen-lockfile
	pnpm -C apps/web build
	find $(WEB_DIST) -mindepth 1 ! -name .keep -delete
	cp -R apps/web/dist/. $(WEB_DIST)/

.PHONY: release
release: ## Build bin/dhow-server with the web app embedded
	$(MAKE) web
	$(MAKE) build

.PHONY: dist
dist: ## Build reproducible release archives and SHA256SUMS in dist/ (run make web first; CHECKOUT_IMAGE=ghcr.io/khaymah/dhow-git@sha256:...)
	CHECKOUT_IMAGE='$(CHECKOUT_IMAGE)' deploy/release/dist.sh $(VERSION) $(COMMIT)

.PHONY: git-image
git-image: ## Build the dhow-git checkout image locally (GIT_IMAGE=dhow-git:dev)
	docker build --tag $(GIT_IMAGE) deploy/dhow-git

.PHONY: git-image-pins
git-image-pins: ## Print the apk versions that git and openssh-client resolve to on the pinned base, to paste into deploy/dhow-git/Dockerfile
	base=$$(sed -n 's/^ARG BASE=//p' deploy/dhow-git/Dockerfile); \
	docker run --rm "$$base" sh -c 'apk info -v 2>/dev/null | sort > /tmp/base && apk add -q --no-cache git openssh-client >/dev/null && apk info -v 2>/dev/null | sort | comm -13 /tmp/base - | sed -E "s/^(.+)-([^-]+-r[0-9]+)$$/\1=\2/"' 

.PHONY: installer-e2e
installer-e2e: build ## Run the installer end to end in a systemd container (E2E_BASE=debian:12)
	deploy/install/e2e/run.sh $(E2E_BASE) bin/dhow-server

.PHONY: runner-install-e2e
runner-install-e2e: build ## Install, check and uninstall the runner in a systemd container with the fake driver (RUNNER_E2E_DATABASE_URL=postgres://..., RUNNER_E2E_BASE=debian:13)
	@test -n "$(RUNNER_E2E_DATABASE_URL)" || { echo "runner-install-e2e: set RUNNER_E2E_DATABASE_URL to an empty PostgreSQL 18 database" >&2; exit 2; }
	CGO_ENABLED=0 GOOS=linux $(GO) build -trimpath -tags dhowtest -ldflags '$(LDFLAGS)' -o bin/dhow-runner-fake ./apps/runner/cmd/dhow-runner
	deploy/install/e2e/runner-install.sh $(RUNNER_E2E_BASE) bin/dhow-server bin/dhow-runner-fake '$(RUNNER_E2E_DATABASE_URL)'

.PHONY: vm-test
vm-test: build build-runner ## Install, check and uninstall the runner in a cloud image under QEMU/KVM (DISTRO=debian-13, fedora or rocky-10)
	@test -n "$(DISTRO)" || { echo "vm-test: set DISTRO=debian-13, fedora or rocky-10" >&2; exit 2; }
	deploy/test-vm/run.sh $(DISTRO) bin/dhow-server bin/dhow-runner

.PHONY: image
image: ## Build the container image (IMAGE=dhow:dev)
	docker buildx build --load $(DOCKER_BUILD_FLAGS) -f $(IMAGE_DOCKERFILE) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE) .

.PHONY: compose-up
compose-up: ## Start the Compose stack with the local image (run deploy/compose/init.sh first)
	@test -f $(COMPOSE_DIR)/.env || { echo "$(COMPOSE_DIR)/.env is missing: run $(COMPOSE_DIR)/init.sh first" >&2; exit 1; }
	DHOW_IMAGE=$(IMAGE) $(COMPOSE) up --detach --wait

.PHONY: compose-down
compose-down: ## Stop the Compose stack and keep its data (COMPOSE_DOWN_FLAGS=--volumes deletes it)
	DHOW_IMAGE=$(IMAGE) $(COMPOSE) down $(COMPOSE_DOWN_FLAGS)

.PHONY: smoke
smoke: image ## Build the image, start a fresh Compose stack, check it, and remove it
	deploy/smoke.sh $(IMAGE)

.PHONY: proxy-check
proxy-check: image ## Build the image and test the documented nginx, Caddy and Traefik configs in Docker (local only)
	deploy/proxy-check/run.sh $(IMAGE)

BENCH_DIR  ?= bin/bench
BENCH_ARGS ?=

.PHONY: bench
bench: image ## Build the image and measure the CPU, memory and disk Dhow needs under load (docs/en/requirements.md)
	$(MAKE) bench-run

.PHONY: bench-run
bench-run: ## Measure an image that is already built (IMAGE=dhow:dev, BENCH_ARGS="-configs 2g:2g" for one run)
	mkdir -p $(BENCH_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -tags dhowtest -o $(BENCH_DIR)/dhow-runner ./apps/runner/cmd/dhow-runner
	$(GO) run ./deploy/bench -image $(IMAGE) -runner $(BENCH_DIR)/dhow-runner -out $(BENCH_DIR)/results.json -work $(BENCH_DIR)/work $(BENCH_ARGS)

.PHONY: tools
tools: ## Build the pinned protoc plugins into bin/tools
	GOBIN=$(abspath $(TOOLS_DIR)) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOBUF_VERSION)
	GOBIN=$(abspath $(TOOLS_DIR)) $(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@$(CONNECT_VERSION)

.PHONY: gen
gen: tools ## Regenerate code from the OpenAPI spec (one config per module), SQL queries and the runner protocol
	set -e; for cfg in apps/server/internal/Modules/*/oapi-codegen.yaml; do \
		$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
			-config $$cfg packages/openapi/openapi.yaml; \
	done
	cd apps/server && $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate
	$(BUF) generate $(PROTO_DIR) --template $(PROTO_DIR)/buf.gen.yaml

.PHONY: proto-lint
proto-lint: ## Lint the runner protocol and check it for breaking changes against main
	$(BUF) lint $(PROTO_DIR)
	@git rev-parse --verify --quiet refs/heads/$(PROTO_AGAINST_BRANCH) >/dev/null || \
		{ echo "proto-lint: branch $(PROTO_AGAINST_BRANCH) is missing: git fetch origin $(PROTO_AGAINST_BRANCH):$(PROTO_AGAINST_BRANCH)" >&2; exit 1; }
	@if git cat-file -e $(PROTO_AGAINST_BRANCH):$(PROTO_DIR)/buf.yaml 2>/dev/null; then \
		echo "$(BUF) breaking $(PROTO_DIR) --against '$(PROTO_AGAINST)'"; \
		$(BUF) breaking $(PROTO_DIR) --against '$(PROTO_AGAINST)'; \
	else \
		echo "proto-lint: $(PROTO_AGAINST_BRANCH) has no $(PROTO_DIR) yet, so there is nothing to break"; \
	fi

DEV_ENV         ?= .env.dev
DEV_DB_ENGINE   ?= $(shell command -v docker >/dev/null 2>&1 && echo docker || { command -v podman >/dev/null 2>&1 && echo podman; })
DEV_DB_PORT     ?= 5432
DEV_DB_NAME     := dhow-dev-postgres
DEV_DB_VOLUME   := dhow-dev-pgdata
DEV_DB_PASSWORD := dhow-dev-only
DEV_DB_URL      := postgres://dhow:$(DEV_DB_PASSWORD)@127.0.0.1:$(DEV_DB_PORT)

.PHONY: dev-setup
dev-setup: ## Install what development needs, ask for your settings and generate secrets (SETUP_ARGS="--help" for options)
	bash deploy/dev/setup.sh $(SETUP_ARGS)

DEV_KEYRING     := $(abspath .dhow-dev-keyring)

.PHONY: dev-env
dev-env: ## Write .env.dev once: DHOW_DEV, a fresh secret key, a development keyring and the local database URLs
	@umask 077; if [ ! -f $(DEV_KEYRING) ]; then printf 'kek 1 %s\n' "$$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')" > $(DEV_KEYRING); echo "wrote $(DEV_KEYRING)"; fi
	@if [ -f $(DEV_ENV) ]; then grep -q '^DHOW_KEYRING_FILE=' $(DEV_ENV) || { echo "DHOW_KEYRING_FILE=$(DEV_KEYRING)" >> $(DEV_ENV); echo "added DHOW_KEYRING_FILE to $(DEV_ENV)"; }; \
		echo "$(DEV_ENV) already exists; delete it to make a new one"; exit 0; fi; \
	umask 077; { \
		echo "DHOW_DEV=true"; \
		echo "DHOW_PUBLIC_URL=http://localhost:8080"; \
		echo "DHOW_DATABASE_URL=$(DEV_DB_URL)/dhow?sslmode=disable"; \
		echo "DHOW_TEST_DATABASE_URL=$(DEV_DB_URL)/dhow_test?sslmode=disable"; \
		echo "DHOW_TEST_PG_DUMP=\"$(DEV_DB_ENGINE) run --rm --network host docker.io/library/postgres:18 pg_dump\""; \
		echo "DHOW_SECRET_KEY=$$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"; \
		echo "DHOW_KEYRING_FILE=$(DEV_KEYRING)"; \
	} > $(DEV_ENV); echo "wrote $(DEV_ENV)"

.PHONY: dev-db
dev-db: ## Start PostgreSQL 18 with the dhow and dhow_test databases in Docker or Podman (DEV_DB_ENGINE=podman)
	@test -n "$(DEV_DB_ENGINE)" || { echo "dev-db: found neither docker nor podman; install PostgreSQL 18 natively (docs/en/development.md)" >&2; exit 1; }
	@if $(DEV_DB_ENGINE) container inspect $(DEV_DB_NAME) >/dev/null 2>&1; then $(DEV_DB_ENGINE) start $(DEV_DB_NAME) >/dev/null; \
	else $(DEV_DB_ENGINE) run -d --name $(DEV_DB_NAME) -e POSTGRES_USER=dhow -e POSTGRES_PASSWORD=$(DEV_DB_PASSWORD) -e POSTGRES_DB=dhow \
		-p 127.0.0.1:$(DEV_DB_PORT):5432 -v $(DEV_DB_VOLUME):/var/lib/postgresql docker.io/library/postgres:18 >/dev/null; fi
	@for i in $$(seq 1 60); do $(DEV_DB_ENGINE) exec $(DEV_DB_NAME) pg_isready -h 127.0.0.1 -U dhow -d dhow >/dev/null 2>&1 && break; sleep 1; done
	@$(DEV_DB_ENGINE) exec $(DEV_DB_NAME) psql -h 127.0.0.1 -U dhow -d dhow -tAc "SELECT 1 FROM pg_database WHERE datname = 'dhow_test'" | grep -q 1 \
		|| $(DEV_DB_ENGINE) exec $(DEV_DB_NAME) psql -h 127.0.0.1 -U dhow -d dhow -qc "CREATE DATABASE dhow_test"
	@echo "PostgreSQL 18 is up on 127.0.0.1:$(DEV_DB_PORT) with the databases dhow and dhow_test ($(DEV_DB_ENGINE))"

.PHONY: dev-db-reset
dev-db-reset: ## Delete the development PostgreSQL container and its data
	$(DEV_DB_ENGINE) rm -f $(DEV_DB_NAME)
	$(DEV_DB_ENGINE) volume rm -f $(DEV_DB_VOLUME)

.PHONY: dev
dev: dev-env ## Run the server from source with .env.dev on http://localhost:8080 (run make web first for the UI)
	set -a; . $(abspath $(DEV_ENV)); set +a; $(GO) run ./apps/server/cmd/dhow-server

.PHONY: dev-vite
dev-vite: dev-env ## Run the server for the Vite dev server: then pnpm -C apps/web dev and open http://localhost:5173
	set -a; . $(abspath $(DEV_ENV)); set +a; DHOW_PUBLIC_URL=http://localhost:5173 $(GO) run ./apps/server/cmd/dhow-server

.PHONY: dev-check
dev-check: dev-env ## Run make check with the test database from .env.dev, so no database test is skipped
	set -a; . $(abspath $(DEV_ENV)); set +a; $(MAKE) check

.PHONY: help
help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

.PHONY: check
check: lint lint-runner lint-actions proto-lint test test-runner vuln hooks-test docs-check ## Everything CI checks

.PHONY: fmt
fmt: ## Format Go code
	$(GO) run mvdan.cc/gofumpt@v0.11.0 -w .

.PHONY: lint
lint: ## Run golangci-lint
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

.PHONY: lint-runner
lint-runner: ## Run golangci-lint on the runner with the fake driver build tag
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --build-tags dhowtest ./apps/runner/...

.PHONY: lint-actions
lint-actions: ## Lint GitHub Actions workflows
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

.PHONY: docs-check
docs-check: ## Check that the Arabic docs and their English mirror in docs/en have the same files, headings, tables, code and links
	$(GO) test -count=1 ./deploy/docs-check/

.PHONY: hooks-test
hooks-test: ## Test the Claude Code project hooks
	python3 -B -m unittest discover -s .claude/hooks/tests -p "test_*.py"

.PHONY: test
test: ## Run Go tests with the race detector
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKG)

FUZZTIME     ?= 10s
SERVER_FUZZ_DIR := apps/server/internal
FUZZ_TARGETS := \
	$(addprefix $(SERVER_FUZZ_DIR)/,Core/Auth:FuzzHashSessionToken Core/Auth:FuzzAPITokenHash Core/Auth:FuzzAuthorizationHeader \
	Core/Auth:FuzzTotp Core/Auth:FuzzRecoveryCode Core/Auth:FuzzEmailAndName Core/Auth:FuzzPasswordLength \
	Core/Auth:FuzzWebAuthnResponses Core/I18n:FuzzParseAcceptLanguage Core/Middleware:FuzzClientIP Core/RateLimit:FuzzNetwork \
	Core/Oidc:FuzzEmailVerifiedClaim Core/Oidc:FuzzAcceptedEmail Modules/Sources/Test/Edge:FuzzWebhook \
	Modules/Pipelines/Test/Edge:FuzzDefinitionProof Core/Blob:FuzzParseDigest Modules/Artifacts/Test/Edge:FuzzDownloadName) \
	internal/gitref:FuzzRef internal/gitsource:FuzzCloneUrl internal/jobspec:FuzzAllowlist internal/jobspec:FuzzPrepare \
	internal/pipelinespec:FuzzParse internal/pipelinespec:FuzzCEL internal/pipelinespec:FuzzMatrix \
	internal/cachekey:FuzzRender internal/pathglob:FuzzMatchPattern

define fuzz_listed
	@found=$$(grep -rhoE '^func Fuzz[A-Za-z0-9]+' $(1) --include='*_test.go' | sed 's/^func //' | sort); \
	listed=$$(printf '%s\n' $(2) | sed 's/.*://' | sort); \
	test "$$found" = "$$listed" || { echo "fuzz: $(3) does not match the Fuzz functions in $(1)" >&2; exit 1; }
endef

define fuzz_run
	set -e; for t in $(1); do \
		echo "fuzz $${t##*:} for $(FUZZTIME)"; \
		$(GO) test -run '^$$' -fuzz "^$${t##*:}$$" -fuzztime $(FUZZTIME) ./$${t%%:*}; \
	done
endef

.PHONY: fuzz
fuzz: ## Fuzz the server and shared inputs, FUZZTIME per target (default 10s; the M2.5 bar is 60s, the nightly run 30m)
	$(call fuzz_listed,$(SERVER_FUZZ_DIR) internal,$(FUZZ_TARGETS),FUZZ_TARGETS)
	$(call fuzz_run,$(FUZZ_TARGETS))

.PHONY: fuzz-list
fuzz-list: ## Print every fuzz target, server and runner, as a JSON list for the nightly workflow
	$(call fuzz_listed,$(SERVER_FUZZ_DIR) internal,$(FUZZ_TARGETS),FUZZ_TARGETS)
	$(call fuzz_listed,apps/runner,$(RUNNER_FUZZ),RUNNER_FUZZ)
	@printf '%s\n' $(FUZZ_TARGETS) $(RUNNER_FUZZ) | sed 's/.*/"&"/' | paste -sd, - | sed 's/.*/[&]/'

FUZZ_TARGET ?=
export FUZZ_TARGET
fuzz_known = $(and $(filter 1,$(words $(FUZZ_TARGET))),$(if $(findstring %,$(FUZZ_TARGET)),,$(filter $(FUZZ_TARGET),$(FUZZ_TARGETS) $(RUNNER_FUZZ))))

.PHONY: fuzz-one
fuzz-one: ## Fuzz one target of make fuzz-list for FUZZTIME (FUZZ_TARGET=internal/gitref:FuzzRef)
	$(if $(fuzz_known),,$(error fuzz-one: FUZZ_TARGET must be one of make fuzz-list))
	@printf '%s\n' $(FUZZ_TARGETS) $(RUNNER_FUZZ) | grep -qxF -- "$$FUZZ_TARGET" || { echo "fuzz-one: FUZZ_TARGET must be one of make fuzz-list" >&2; exit 1; }
	$(GO) test -run '^$$' -fuzz "^$${FUZZ_TARGET##*:}$$" -fuzztime $(FUZZTIME) -timeout 0 "./$${FUZZ_TARGET%%:*}"

.PHONY: timing
timing: ## Run the statistical timing tests against DHOW_TEST_DATABASE_URL (slow, separate CI job)
	$(GO) test -tags timing -count=1 -run '^TestTiming' -timeout 30m -v ./apps/server/internal/App/Timing/...

.PHONY: test-runner
test-runner: ## Run the runner tests with the fake sandbox driver (dhowtest build tag)
	$(GO) test -race -count=1 -tags dhowtest ./apps/runner/...

ISOLATION_DIR     ?= $(abspath bin/isolation)
ISOLATION_BUSYBOX ?= /bin/busybox
ISOLATION_RUN     ?=
ISOLATION_GIT_ARCHIVE ?=
ISOLATION_SSHD    ?=

.PHONY: runner-isolation
runner-isolation: ## Run the escape and limits suites on rootless Podman (Podman 5.4+, cgroup v2 delegation, subuids, busybox-static)
	mkdir -p $(ISOLATION_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(ISOLATION_DIR)/dhow-runner ./apps/runner/cmd/dhow-runner
	CGO_ENABLED=0 $(GO) build -trimpath -o $(ISOLATION_DIR)/escape-probe ./apps/runner/Test/Isolation/Probe
	$(GO) test -c -tags isolation -o $(ISOLATION_DIR)/isolation.test ./apps/runner/Test/Isolation
	chmod 0755 $(ISOLATION_DIR) $(ISOLATION_DIR)/dhow-runner $(ISOLATION_DIR)/escape-probe $(ISOLATION_DIR)/isolation.test
	$(ISOLATION_RUN) env DHOW_ISOLATION_BIN=$(ISOLATION_DIR) DHOW_ISOLATION_BUSYBOX=$(ISOLATION_BUSYBOX) \
		DHOW_ISOLATION_GIT_ARCHIVE=$(ISOLATION_GIT_ARCHIVE) DHOW_ISOLATION_SSHD=$(ISOLATION_SSHD) \
		$(ISOLATION_DIR)/isolation.test -test.v -test.count=1 -test.timeout 30m

CHAOS_DIR     ?= $(abspath bin/chaos)
CHAOS_BUSYBOX ?= /bin/busybox
CHAOS_RUN     ?=

.PHONY: runner-chaos
runner-chaos: ## Run the chaos scenarios: the real server and runner on Podman, Postgres and a TLS registry (see docs/en/runner/chaos.md)
	mkdir -p $(CHAOS_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(CHAOS_DIR)/dhow-server ./apps/server/cmd/dhow-server
	CGO_ENABLED=0 $(GO) build -trimpath -o $(CHAOS_DIR)/dhow-runner ./apps/runner/cmd/dhow-runner
	$(GO) test -c -tags chaos -o $(CHAOS_DIR)/chaos.test ./apps/runner/Test/Chaos
	chmod 0755 $(CHAOS_DIR) $(CHAOS_DIR)/dhow-server $(CHAOS_DIR)/dhow-runner $(CHAOS_DIR)/chaos.test
	$(CHAOS_RUN) env DHOW_CHAOS_BIN=$(CHAOS_DIR) DHOW_CHAOS_BUSYBOX=$(CHAOS_BUSYBOX) \
		$(CHAOS_DIR)/chaos.test -test.v -test.count=1 -test.timeout 60m

RUNNER_FUZZ := $(addprefix apps/runner/internal/,Masking:FuzzMasker Masking:FuzzMaskerKeepsCleanText Logs:FuzzChunker \
	Logs:FuzzSpillRecords Journal:FuzzJournalOpen Executor:FuzzVerifyGrant Sandbox/Wire:FuzzDecode Executor/Plugin:FuzzManifest \
	Executor/Plugin:FuzzOutputs Sandbox/Archive:FuzzUnpack Egress:FuzzProxyRequest)

.PHONY: fuzz-runner
fuzz-runner: ## Fuzz every runner target for FUZZTIME each (default 10s; the M3.5 bar is 60s, the nightly run 30m)
	$(call fuzz_listed,apps/runner,$(RUNNER_FUZZ),RUNNER_FUZZ)
	$(call fuzz_run,$(RUNNER_FUZZ))

.PHONY: vuln
vuln: ## Scan Go dependencies for known vulnerabilities
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) $(PKG)

.PHONY: tidy
tidy: ## Tidy go.mod
	$(GO) mod tidy
