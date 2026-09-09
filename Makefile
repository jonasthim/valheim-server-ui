VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS := CGO_ENABLED=0
DEVDATA := $(CURDIR)/devdata

.PHONY: all deps gen web build check test lint vet fmt dev dev-backend dev-web e2e clean release fake-server

all: build

deps:
	go mod download
	cd web && npm ci

gen:
	cd web && npm run gen:api

web:
	cd web && npm run build

build: web
	$(GOFLAGS) go build -trimpath -ldflags '$(LDFLAGS)' -o bin/valheim-ui ./cmd/valheim-ui

build-go:
	$(GOFLAGS) go build -trimpath -ldflags '$(LDFLAGS)' -o bin/valheim-ui ./cmd/valheim-ui

fmt:
	gofmt -w cmd internal web/*.go
	cd web && npm run format 2>/dev/null || true

vet:
	go vet ./...

lint: vet
	golangci-lint run ./...
	cd web && npm run lint && npm run typecheck

test:
	go test -race -count=1 ./...

check: lint test

# Local development: backend on :8080 with direct supervisor + fake game server,
# frontend dev server on :5173 proxying /api to the backend.
dev-backend: build-go
	mkdir -p $(DEVDATA)
	VALHEIM_UI_CONFIG=$(CURDIR)/dev.config.yaml VALHEIM_UI_DATA_DIR=$(DEVDATA) \
	VALHEIM_UI_FAKE_SERVER_PATH=$(CURDIR)/testdata/fake-server.sh \
	./bin/valheim-ui serve

dev-web:
	cd web && npm run dev

dev:
	@echo "run 'make dev-backend' and 'make dev-web' in two terminals"

e2e: build
	cd web && npx playwright test --config e2e/playwright.config.ts

release: build
	mkdir -p dist
	tar -czf dist/valheim-ui_linux_amd64.tar.gz -C bin valheim-ui
	cp -r deploy dist/
	@echo "artifacts in dist/"

clean:
	rm -rf bin dist web/dist devdata
	mkdir -p web/dist && printf '<!doctype html><title>valheim-server-ui</title><p>Run make web.' > web/dist/index.html
