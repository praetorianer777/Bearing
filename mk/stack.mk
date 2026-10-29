# The compose stack. The project name and every published port derive from the checkout's path, so worktrees
# running their gates side by side never share a database or collide on a port.

STACK_HASH := $(shell printf '%s' '$(ROOT)' | cksum | cut -d' ' -f1)
STACK_PROJECT := bearing-$(STACK_HASH)
STACK_NET := $(STACK_PROJECT)_default

# scripts/stack-ports.sh holds the rule and tests/test-ports.sh checks it.
STACK_PORT_BASE ?= $(shell $(ROOT)/scripts/stack-ports.sh '$(ROOT)')
stack_port = $(shell echo $$(($(STACK_PORT_BASE) + $(1))))

WEB_PORT              := $(call stack_port,0)
API_PORT              := $(call stack_port,1)
KEYCLOAK_PORT         := $(call stack_port,2)
MAILPIT_PORT          := $(call stack_port,3)
POSTGRES_PORT         := $(call stack_port,4)
POSTGRES_REPLICA_PORT := $(call stack_port,5)
VALKEY_PORT           := $(call stack_port,6)

# Credentials for the throwaway development stack; override them in the environment for anything else.
POSTGRES_PASSWORD ?= bearing
APP_DB_PASSWORD   ?= bearing_app
ADMIN_DB_PASSWORD ?= bearing_admin
VALKEY_PASSWORD   ?= bearing_valkey

KEYCLOAK_REALM := dynamo
# Dynamo holds the suite's canonical realm; Bearing's own client and group are left aside until Dynamo carries them.
DYNAMO_REALM_URL ?= https://raw.githubusercontent.com/praetorianer777/Dynamo/main/deploy/keycloak/realm.json

.PHONY: realm-check
realm-check: ## Fail if deploy/keycloak/realm.json differs from Dynamo's canonical realm
	@docker run --rm -u $(UID_GID) -v $(ROOT)/deploy/keycloak:/realm:ro $(NODE_IMAGE) \
		node /realm/realm-check.mjs '$(DYNAMO_REALM_URL)' /realm/realm.json

export COMPOSE_FILE := $(ROOT)/deploy/docker-compose.yml
export COMPOSE_PROJECT_NAME := $(STACK_PROJECT)
export WEB_PORT API_PORT KEYCLOAK_PORT MAILPIT_PORT POSTGRES_PORT POSTGRES_REPLICA_PORT VALKEY_PORT
export POSTGRES_PASSWORD APP_DB_PASSWORD ADMIN_DB_PASSWORD VALKEY_PASSWORD

# What the stack publishes, for the test suites and for people: a shell can source it, and so can a Playwright config.
STACK_ENV_FILE := $(ROOT)/.cache/stack.env

.PHONY: stack-env
stack-env:
	@mkdir -p $(dir $(STACK_ENV_FILE))
	@printf '%s\n' \
		'COMPOSE_PROJECT_NAME=$(STACK_PROJECT)' \
		'STACK_NETWORK=$(STACK_NET)' \
		'WEB_PORT=$(WEB_PORT)' \
		'API_PORT=$(API_PORT)' \
		'KEYCLOAK_PORT=$(KEYCLOAK_PORT)' \
		'MAILPIT_PORT=$(MAILPIT_PORT)' \
		'POSTGRES_PORT=$(POSTGRES_PORT)' \
		'POSTGRES_REPLICA_PORT=$(POSTGRES_REPLICA_PORT)' \
		'VALKEY_PORT=$(VALKEY_PORT)' \
		'BEARING_WEB_URL=http://localhost:$(WEB_PORT)' \
		'BEARING_KEYCLOAK_URL=http://localhost:$(KEYCLOAK_PORT)' \
		'BEARING_OIDC_ISSUER=http://localhost:$(KEYCLOAK_PORT)/realms/$(KEYCLOAK_REALM)' \
		'BEARING_MAILPIT_URL=http://localhost:$(MAILPIT_PORT)' \
		'BEARING_BUILD_VERSION=$(BEARING_BUILD_VERSION)' \
		'BEARING_BUILD_COMMIT=$(BEARING_BUILD_COMMIT)' \
		> $(STACK_ENV_FILE)

.PHONY: up
up: stack-env ## Build and start the whole stack in the background, and say where it is
	docker compose up -d --build --wait
	@echo "Bearing    http://localhost:$(WEB_PORT) (admin@bearing.test / bearing admin password)"
	@echo "API        http://localhost:$(API_PORT)/api/v1/meta (also under /api/ on the web port)"
	@echo "Keycloak   http://localhost:$(KEYCLOAK_PORT) (admin / admin; realm $(KEYCLOAK_REALM); people alice, bob, carol, dave with password '<name> password')"
	@echo "Mailpit    http://localhost:$(MAILPIT_PORT)"
	@echo "Postgres   127.0.0.1:$(POSTGRES_PORT) (replica $(POSTGRES_REPLICA_PORT)), user bearing, password $(POSTGRES_PASSWORD)"
	@echo "Valkey     127.0.0.1:$(VALKEY_PORT)"
	@echo "Ports and project name are in $(STACK_ENV_FILE)."

.PHONY: down
down: ## Stop the stack, keeping its data
	docker compose down

.PHONY: clean
clean: ## Stop the stack and delete its volumes
	docker compose down -v --remove-orphans
	@rm -f $(STACK_ENV_FILE)

.PHONY: logs
logs: ## Follow the stack's logs: make logs S=keycloak for one service
	docker compose logs -f $(S)

.PHONY: ports
ports: ## Print this checkout's compose project and ports
	@echo "project $(STACK_PROJECT), ports $(STACK_PORT_BASE) to $(shell echo $$(($(STACK_PORT_BASE) + 19)))"

.PHONY: seed
seed: ## Run the seed again against the running stack
	docker compose run --rm seed

.PHONY: stack-up
stack-up: stack-env ## The gate's stack: build and start it, and wait until every service is healthy
	docker compose up -d --build --wait --quiet-pull

.PHONY: stack-down
stack-down: ## Remove the gate's stack and its volumes
	docker compose down -v --remove-orphans
	@rm -f $(STACK_ENV_FILE)
