# Each area brings its own targets in mk/<area>.mk; run-tests.sh calls a gate target only when it exists.
.DEFAULT_GOAL := help

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
VERSION := $(shell cat $(ROOT)/VERSION)
# Compose passes both to the image builds, so GET /api/v1/meta names the build a checkout made.
export BEARING_BUILD_VERSION := $(shell $(ROOT)/scripts/build-stamp.sh version)
export BEARING_BUILD_COMMIT := $(shell $(ROOT)/scripts/build-stamp.sh commit)
UID_GID := $(shell id -u):$(shell id -g)
REPORTS := $(ROOT)/reports
NODE_IMAGE ?= node:26-alpine
# A terminal for the Node containers only when there is one, so the gate runs the same in CI.
DOCKER_NODE_TTY := $(shell [ -t 0 ] && echo -t)

include $(wildcard $(ROOT)/mk/*.mk)

.PHONY: test-ports
test-ports: ## Check the port rule in scripts/stack-ports.sh
	$(ROOT)/tests/test-ports.sh

.PHONY: help
help:
	@grep -hE '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | sort
