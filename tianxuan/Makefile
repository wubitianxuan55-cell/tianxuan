VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# CodeGraph release pinned for the bundled MCP server / e2e test. Bump together
# with any change to the integration in internal/codegraph.
CODEGRAPH_VERSION := v0.9.7

.PHONY: build vet fmt test hooks cross clean e2e-codegraph webui clean-webui lint-cache

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/tianxuan ./cmd/tianxuan
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/tianxuan-plugin-example ./cmd/tianxuan-plugin-example

# webui builds the serve-frontend and copies assets into internal/serve/webui.
# Requires pnpm (install via `npm i -g pnpm` or `corepack enable`).
webui:
	cd desktop/frontend && pnpm install && pnpm run build
	rm -rf internal/serve/webui/assets/*
	cp desktop/frontend/dist/index.html internal/serve/webui/
	cp -r desktop/frontend/dist/assets internal/serve/webui/assets/
	@echo "serve webui updated — run 'go build' to embed the new assets"

# clean-webui removes stale build artifacts from the serve webui directory.
clean-webui:
	rm -rf internal/serve/webui/assets/*
	@echo "serve webui assets cleaned"

vet:
	go vet ./...

fmt:
	gofmt -w .

test:
	go test ./...

hooks:
	@git config core.hooksPath .githooks
	@echo "installed: core.hooksPath -> .githooks (pre-push runs go vet)"

cross:
	@mkdir -p dist
	@for p in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "build $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o dist/tianxuan-$$os-$$arch$$ext ./cmd/tianxuan; \
	done

clean:
	rm -rf bin dist

# Fetch the matching CodeGraph bundle into bin/codegraph/ (the distribution
# layout: launcher at bin/codegraph/bin/codegraph beside bin/tianxuan) and run the
# gated MCP end-to-end test against it. Requires `gh`. Windows: install via the
# upstream install.ps1 and run the test with TIANXUAN_CODEGRAPH_BIN set.
e2e-codegraph:
	@os=$$(uname -s | tr 'A-Z' 'a-z'); arch=$$(uname -m); \
	case $$arch in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=x64;; *) echo "unsupported arch $$arch"; exit 1;; esac; \
	asset=codegraph-$$os-$$arch.tar.gz; dest=bin/codegraph; \
	echo "fetching $$asset ($(CODEGRAPH_VERSION)) -> $$dest"; \
	rm -rf $$dest && mkdir -p $$dest; \
	gh release download $(CODEGRAPH_VERSION) -R colbymchenry/codegraph -p $$asset -O /tmp/$$asset; \
	tar -xzf /tmp/$$asset -C $$dest --strip-components=1; \
	TIANXUAN_CODEGRAPH_E2E=1 TIANXUAN_CODEGRAPH_BIN=$$PWD/$$dest/bin/codegraph \
		go test ./internal/codegraph/ -run E2E -v -count=1

# lint-cache runs the cacheguard static checker to detect patterns that would
# break the DeepSeek prefix cache (L1 identity / L2 runtime / L3 tools).
lint-cache:
	go run ./cmd/cacheguard
