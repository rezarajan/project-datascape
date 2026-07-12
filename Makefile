GO ?= go
BINARY ?= bin/platformctl
VERSION ?= 0.1.0-dev
SOURCE_COMMIT ?=

.PHONY: test race build validate-examples generate-example docs docs-check terminology-check determinism-check conformance clean

test:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) test ./...

race:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) test -race ./...

build:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags="-s -w -buildid= -X main.version=$(VERSION) -X main.sourceCommit=$(SOURCE_COMMIT)" -o $(BINARY) ./cmd/platformctl

validate-examples:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) run ./cmd/platformctl validate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml

generate-example:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output dist/local

docs-check:
	test -s docs/index.md
	test -s docs/specification/reference.md
	test -s docs/migration/resource-model-v1alpha1.md

docs: build
	rm -rf dist/docs
	./$(BINARY) docs build --source docs --output dist/docs
	test -s dist/docs/index.html
	test -s dist/docs/reference/api.html

terminology-check:
	./scripts/terminology-check.sh

determinism-check:
	rm -rf /tmp/project-datascape-det-a /tmp/project-datascape-det-b
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output /tmp/project-datascape-det-a
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) run ./cmd/platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output /tmp/project-datascape-det-b
	diff -ru /tmp/project-datascape-det-a /tmp/project-datascape-det-b

conformance:
	GOCACHE=$${GOCACHE:-/tmp/project-datascape-go-cache} $(GO) run ./cmd/platformctl conformance >/tmp/project-datascape-conformance.json

clean:
	rm -rf bin dist coverage.out
