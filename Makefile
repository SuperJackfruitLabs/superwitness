GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Restrict Go patterns so web/node_modules .go files are never picked up.
PKGS = ./cmd/... ./internal/... ./test/...
# Every file under test/ is //go:build integration, so untagged unit tests use their own pattern.
UNIT_PKGS = ./cmd/... ./internal/...

.PHONY: build dist test test-integration web web-check lint licenses e2e

build:
	$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/superwitness ./cmd/superwitness

# Release tarballs for linux/amd64 and linux/arm64 plus SHA256SUMS, under dist/.
# Usage: make dist VERSION=v0.0.4
DIST_VER = $(patsubst v%,%,$(VERSION))
SHA256 = $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")

dist:
	rm -rf dist
	mkdir -p dist
	set -e; for arch in amd64 arm64; do \
	  name=superwitness_$(DIST_VER)_linux_$$arch; \
	  mkdir -p dist/$$name/deploy; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/$$name/superwitness ./cmd/superwitness; \
	  cp LICENSE NOTICE dist/$$name/; \
	  cp -R licenses dist/$$name/; \
	  cp deploy/superwitness.service deploy/env.example dist/$$name/deploy/; \
	  COPYFILE_DISABLE=1 tar -C dist -czf dist/$$name.tar.gz $$name; \
	  rm -rf dist/$$name; \
	done
	cd dist && $(SHA256) superwitness_$(DIST_VER)_linux_*.tar.gz > SHA256SUMS

test:
	$(GO) test $(UNIT_PKGS)

test-integration:
	$(GO) test -tags integration -count=1 ./internal/verdicts/... ./internal/runs/... ./internal/session/... ./internal/app/... ./test/integration/...

web:
	cd web && npm ci && npm test && npm run build

web-check: web
	test -z "$$(git status --porcelain -- internal/web/dist)"

lint:
	$(GO) vet -tags integration $(PKGS)

# github.com/segmentio/asm (pulled in by the MCP SDK) is MIT-0 (MIT No Attribution), hand-reviewed:
# permissive, but go-licenses does not classify it. The allowlist is unchanged.
licenses:
	GOFLAGS=-tags=integration $(GO) run github.com/google/go-licenses/v2@v2.0.1 check $(PKGS) \
	  --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC \
	  --ignore github.com/segmentio/asm

# The end-to-end test: the built binary in fake mode, a stand-in hub, a throwaway Postgres in
# Docker, and Playwright driving Chromium. Needs `npx playwright install chromium` once.
E2E_PG = superwitness-e2e-postgres
E2E_DSN = postgres://sw:sw@127.0.0.1:55432/superwitness?sslmode=disable

e2e: build
	docker rm -f $(E2E_PG) >/dev/null 2>&1 || true
	docker run -d --rm --name $(E2E_PG) -e POSTGRES_USER=sw -e POSTGRES_PASSWORD=sw -e POSTGRES_DB=superwitness \
	  -p 127.0.0.1:55432:5432 postgres:17-alpine >/dev/null
	i=0; until docker exec $(E2E_PG) pg_isready -U sw -d superwitness -h 127.0.0.1 >/dev/null 2>&1; do \
	  i=$$((i+1)); if [ $$i -ge 60 ]; then echo 'e2e: Postgres not ready after 60 s' >&2; docker rm -f $(E2E_PG) >/dev/null; exit 1; fi; sleep 1; \
	done
	cd web && E2E_DATABASE_URL='$(E2E_DSN)' npx playwright test; status=$$?; docker rm -f $(E2E_PG) >/dev/null; exit $$status
