# The web client, run in a Node container as the invoking user with the npm cache under .cache/.
# Defined only once web/package.json exists, so the gate skips check-web before then.
ifneq ($(wildcard $(ROOT)/web/package.json),)

NPM_CACHE := $(ROOT)/.cache/npm

# The whole checkout is mounted, so the client reads VERSION without extra mounts.
DOCKER_NODE = docker run --rm $(DOCKER_NODE_TTY) \
	-u $(UID_GID) \
	-v $(ROOT):/src \
	-v $(NPM_CACHE):/npmcache \
	-e npm_config_cache=/npmcache \
	-e npm_config_update_notifier=false \
	-e HOME=/tmp \
	-w /src/web $(NODE_IMAGE)

# One dev server port per checkout, so worktrees running side by side do not fight over 5173.
WEB_DEV_PORT ?= $(shell echo $$((5200 + $$(printf %s '$(ROOT)' | cksum | cut -d' ' -f1) % 700)))

$(NPM_CACHE):
	@mkdir -p $@

.PHONY: npm
npm: | $(NPM_CACHE) ## Run an npm command in the web container: make npm ARGS="install"
	$(DOCKER_NODE) npm $(ARGS)

.PHONY: web-install
web-install: | $(NPM_CACHE) ## Install the web client's dependencies from the lock file
	$(DOCKER_NODE) npm ci

.PHONY: check-web
check-web: | $(NPM_CACHE) ## Lint, type-check and unit-test the web client
	@mkdir -p $(REPORTS)
	$(DOCKER_NODE) sh -c 'npm ci --no-audit --no-fund && npm run lint && npm run typecheck && npm test -- --coverage'

.PHONY: test-web
test-web: | $(NPM_CACHE) ## Run the web client's unit tests
	$(DOCKER_NODE) sh -c '[ -d node_modules ] || npm ci --no-audit --no-fund; npm test -- $(ARGS)'

.PHONY: web-build
web-build: | $(NPM_CACHE) ## Production build of the web client into web/dist
	$(DOCKER_NODE) sh -c 'npm ci --no-audit --no-fund && npm run build'

# The api of this checkout's stack, reached from the dev server's container on the stack's network.
VITE_API_PROXY ?= http://api:8080

.PHONY: web-dev
web-dev: | $(NPM_CACHE) ## Vite dev server on a port derived from this checkout's path, against the running stack's api
	@echo "Bearing web on http://localhost:$(WEB_DEV_PORT)"
	docker run --rm --init $(if $(DOCKER_NODE_TTY),-it) \
		-u $(UID_GID) \
		--network $(STACK_NET) \
		-v $(ROOT):/src \
		-v $(NPM_CACHE):/npmcache \
		-e npm_config_cache=/npmcache \
		-e HOME=/tmp \
		-e VITE_API_PROXY=$(VITE_API_PROXY) \
		--name bearing-web-dev-$(WEB_DEV_PORT) \
		-p 127.0.0.1:$(WEB_DEV_PORT):5173 \
		-w /src/web $(NODE_IMAGE) npm run dev

.PHONY: format
format: | $(NPM_CACHE) ## Format the web client and the browser suite with Biome
	docker run --rm -u $(UID_GID) -v $(ROOT):/src -v $(NPM_CACHE):/npmcache -e npm_config_cache=/npmcache -e HOME=/tmp -w /src $(NODE_IMAGE) npx --yes @biomejs/biome@2 format --write web/src

endif
