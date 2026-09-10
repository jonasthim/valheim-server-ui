VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS := CGO_ENABLED=0
DEVDATA := $(CURDIR)/devdata

.PHONY: all deps gen web build build-go build-go-windows check test lint vet fmt dev dev-backend dev-web e2e clean release fake-server deploy-sync deploy-sync-check

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

# Windows build (cross-compiled; the embedded frontend must be built first).
build-go-windows:
	GOOS=windows GOARCH=amd64 $(GOFLAGS) go build -trimpath -ldflags '$(LDFLAGS)' -o bin/valheim-ui.exe ./cmd/valheim-ui

# Portable fake game server used by the launcher/supervisor tests and by
# Windows development (Linux dev uses testdata/fake-server.sh).
fake-server:
	$(GOFLAGS) go build -o bin/fake-server$(if $(filter windows,$(GOOS)),.exe,) ./tools/fake-server

fmt:
	gofmt -w cmd internal web/*.go
	cd web && npm run format 2>/dev/null || true

vet:
	go vet ./...
	GOOS=windows go vet ./...

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

release: build build-go-windows
	mkdir -p dist
	tar -czf dist/valheim-ui_linux_amd64.tar.gz -C bin valheim-ui
	rm -f dist/valheim-ui_windows_amd64.zip && (cd bin && zip -q ../dist/valheim-ui_windows_amd64.zip valheim-ui.exe)
	cp -r deploy dist/
	@echo "artifacts in dist/"

# Regenerate deploy/install.sh from deploy/install.sh.in, inlining unitctl,
# sudoers.d/valheim-ui, valheim-ui.service, valheim@.service and
# config.example.yaml so the installer stays a single self-contained file.
# Run this after editing any of those files and commit the result.
deploy-sync:
	./deploy/build-installer.sh

# CI check: fails if deploy/install.sh was not regenerated after an edit to
# deploy/install.sh.in or one of the files it embeds.
deploy-sync-check: deploy-sync
	git diff --exit-code -- deploy/install.sh || (echo "deploy/install.sh is out of date; run 'make deploy-sync' and commit it" && exit 1)

clean:
	rm -rf bin dist web/dist devdata
	mkdir -p web/dist && touch web/dist/.gitkeep
