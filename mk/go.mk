# The Go backend. Every target runs the toolchain in a container as the invoking user, with module and build
# caches under .cache/ so they survive between runs.

# Bookworm rather than alpine: the race detector needs cgo, and alpine ships no C compiler.
GO_IMAGE ?= golang:1.26-bookworm
GO_CACHE := $(ROOT)/.cache/go
GO_PKG   := github.com/praetorianer777/bearing/backend
GO_LDFLAGS := -X $(GO_PKG)/internal/version.Version=$(BEARING_BUILD_VERSION) -X $(GO_PKG)/internal/version.Commit=$(BEARING_BUILD_COMMIT)

go_run = docker run --rm \
	-u $(UID_GID) \
	-v $(ROOT):/work \
	-v $(GO_CACHE):/cache \
	-e HOME=/tmp \
	-e GOCACHE=/cache/build \
	-e GOMODCACHE=/cache/mod \
	-e GOFLAGS=-buildvcs=false \
	-e GOTOOLCHAIN=local \
	$(1) \
	-w /work/backend $(GO_IMAGE)
DOCKER_GO = $(call go_run,)

# The packages test-go runs; PKG=./internal/db/... runs one while working on it.
PKG ?= ./...

$(GO_CACHE):
	@mkdir -p $@

.PHONY: fmt
fmt: | $(GO_CACHE) ## Format the Go sources in place
	$(DOCKER_GO) gofmt -w .

.PHONY: fmt-check
fmt-check: | $(GO_CACHE) ## Fail when a Go file is not gofmt-clean
	@$(DOCKER_GO) sh -c 'out=$$(gofmt -l .); [ -z "$$out" ] || { echo "These files need formatting; run make fmt:"; echo "$$out"; exit 1; }'

.PHONY: vet
vet: | $(GO_CACHE) ## Run go vet over the backend, the integration suite included
	$(DOCKER_GO) go vet ./...
	$(DOCKER_GO) go vet -tags integration ./...

.PHONY: test-go
test-go: | $(GO_CACHE) ## Run the Go unit tests with the race detector, and hold the whole run to its coverage floor
	$(ROOT)/scripts/go-test.sh $(REPORTS)/go-unit.json $(DOCKER_GO) go test -race -json -coverprofile=../reports/go-unit.cover $(TESTFLAGS) $(PKG)
	@if [ "$(PKG)" = "./..." ] && [ -z "$(TESTFLAGS)" ]; then $(ROOT)/scripts/coverage.sh go go-unit $(REPORTS)/go-unit.cover; fi

.PHONY: build-go
build-go: | $(GO_CACHE) ## Build every backend binary into backend/bin, stamped with the version and commit
	$(DOCKER_GO) go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/ ./cmd/...

.PHONY: tidy
tidy: | $(GO_CACHE) ## Run go mod tidy
	$(DOCKER_GO) go mod tidy

.PHONY: openapi
openapi: | $(GO_CACHE) ## Regenerate api/openapi.json from the route table, and the web client's types from it
	$(DOCKER_GO) go run ./cmd/openapi ../api/openapi.json
	@if [ -f $(ROOT)/web/package.json ]; then $(MAKE) --no-print-directory web-schema; fi

.PHONY: openapi-check
openapi-check: | $(GO_CACHE) ## Fail when api/openapi.json differs from what the code generates
	@$(DOCKER_GO) sh -c 'go run ./cmd/openapi /tmp/openapi.json && diff -u ../api/openapi.json /tmp/openapi.json' \
		|| { echo "api/openapi.json is out of date. Run make openapi and commit the result."; exit 1; }

.PHONY: check-go
check-go: fmt-check vet test-go openapi-check ## The backend gate: formatting, vet, race-checked tests, OpenAPI drift

# The Go toolchain on the stack's network with what the integration suite reads. Reads go to the replica as they do
# in the running api, so the suite proves read-your-writes rather than stepping around it.
STACK_PG_URL = postgres://$(1):$(2)@$(3):5432/bearing?sslmode=disable
DOCKER_GO_STACK = $(call go_run,--network $(STACK_NET) \
	-e BEARING_DB_PRIMARY_URL='$(call STACK_PG_URL,bearing_app,$(APP_DB_PASSWORD),postgres-primary)' \
	-e BEARING_DB_REPLICA_URLS='$(call STACK_PG_URL,bearing_app,$(APP_DB_PASSWORD),postgres-replica)' \
	-e BEARING_DB_ADMIN_URL='$(call STACK_PG_URL,bearing_admin,$(ADMIN_DB_PASSWORD),postgres-primary)' \
	-e BEARING_VALKEY_URL='redis://:$(VALKEY_PASSWORD)@valkey:6379/0' \
	-e BEARING_TEST_SUPERUSER_URL='$(call STACK_PG_URL,bearing,$(POSTGRES_PASSWORD),postgres-primary)' \
	-e BEARING_TEST_REPLICA_SUPERUSER_URL='$(call STACK_PG_URL,bearing,$(POSTGRES_PASSWORD),postgres-replica)' \
	-e BEARING_TEST_API_URL=http://api:8080 \
	$(1))

INTEGRATION_TIMEOUT ?= 10m

.PHONY: test-integration
test-integration: | $(GO_CACHE) ## Run the integration suite against this checkout's running stack
	@docker compose ps --status running --services 2>/dev/null | grep -qx api \
		|| { echo "The stack for this checkout is not running. Start it with make up or make stack-up, then run this again."; exit 1; }
	$(ROOT)/scripts/go-test.sh $(REPORTS)/integration.json $(call DOCKER_GO_STACK,) go test -race -json -tags integration -count=1 -p 1 \
		-timeout $(INTEGRATION_TIMEOUT) -coverpkg=./... -coverprofile=../reports/go-integration.cover $(TESTFLAGS) ./test/...
	@[ -n "$(TESTFLAGS)" ] || $(ROOT)/scripts/coverage.sh go go-integration $(REPORTS)/go-integration.cover

.PHONY: shell
shell: | $(GO_CACHE) ## Interactive shell in the Go toolchain container, on the stack's network
	$(call DOCKER_GO_STACK,-it) sh
