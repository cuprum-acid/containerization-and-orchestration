#
#   make build    - static binary at bin/api
#   make run      - build, then run it in the foreground
#   make dev      - go run, no binary
#   make smoke    - hit every endpoint of a running instance
#   make vet      - go vet + gofmt check
#   make clean

BIN  ?= bin/api
ADDR ?= :8080
URL  ?= http://localhost$(ADDR)

.PHONY: all build run dev smoke vet fmt clean

all: build

# CGO_ENABLED=0 gives a static binary with no libc dependency: it runs on a bare
# rootfs inside a mount namespace, and it is what makes FROM scratch work later.
build:
	cd api && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../$(BIN) .
	@ls -lh $(BIN)

run: build
	ADDR=$(ADDR) ./$(BIN)

dev:
	cd api && ADDR=$(ADDR) go run .

smoke:
	@set -e; \
	echo "GET /health       -> $$(curl -sf $(URL)/health)"; \
	echo "GET /eat?mb=64    -> $$(curl -sf '$(URL)/eat?mb=64')"; \
	echo "GET /burn         -> $$(curl -sf $(URL)/burn)"; \
	echo "GET /stop         -> $$(curl -sf $(URL)/stop)"; \
	echo "GET /free         -> $$(curl -sf $(URL)/free)"; \
	echo "GET /openapi.yaml -> $$(curl -sf $(URL)/openapi.yaml | head -1)"; \
	echo "GET /docs         -> $$(curl -sf -o /dev/null -w '%{http_code}, %{size_download} bytes' $(URL)/docs)"; \
	echo "GET /docs/swagger-ui-bundle.js -> $$(curl -sf -o /dev/null -w '%{http_code}, %{size_download} bytes' $(URL)/docs/swagger-ui-bundle.js)"; \
	echo "GET /info         ->"; curl -sf $(URL)/info

vet:
	cd api && go vet ./...
	@test -z "$$(gofmt -l api)" || { echo "gofmt needed:"; gofmt -l api; exit 1; }

fmt:
	gofmt -w api

clean:
	rm -rf bin
