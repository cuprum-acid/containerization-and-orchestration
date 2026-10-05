#   make build    - static binary at bin/api
#   make run      - build, then run it in the foreground
#   make smoke    - hit every endpoint of a running instance
#   make image    - docker image api:lab2
#   make vet      - go vet + gofmt check
#   make clean
#
#   make check-agents   - validate the Alloy config and the Vector VRL before rollout
#   make deploy-alloy   - roll out Alloy with helm/files/config.alloy
#   make deploy-vector  - roll out Vector with helm/files/api.vrl

BIN   ?= bin/api
ADDR  ?= :8080
URL   ?= http://localhost$(ADDR)
IMAGE ?= api:lab2

.PHONY: build run smoke image vet clean check-agents deploy-alloy deploy-vector

ALLOY_IMAGE  ?= grafana/alloy:v1.20.1
VECTOR_IMAGE ?= timberio/vector:0.58.0-distroless-libc

build:
	cd api && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../$(BIN) .
	@ls -lh $(BIN)

run: build
	ADDR=$(ADDR) ./$(BIN)

smoke:
	@echo "GET /health -> $$(curl -s $(URL)/health)"
	@echo "GET /fail   -> $$(curl -s -o /dev/null -w '%{http_code}' $(URL)/fail)"
	@echo "GET /slow   -> $$(curl -s $(URL)/slow)"
	@echo "GET /load   -> $$(curl -s '$(URL)/load?n=20')"
	@echo "GET /metrics:"; curl -s $(URL)/metrics | grep -E '^http_(requests_total|request_errors_total)'

image:
	docker build -t $(IMAGE) .

vet:
	cd api && go vet ./...
	@test -z "$$(gofmt -l api)" || { echo "gofmt needed:"; gofmt -l api; exit 1; }

clean:
	rm -rf bin

# The agents' configs are separate files passed with --set-file. These targets
# exist so the flag cannot be forgotten: without it Alloy would get an empty
# config and silently ship nothing.

check-agents:
	docker run --rm -v "$(CURDIR)/helm/files:/c" $(ALLOY_IMAGE) fmt /c/config.alloy | diff -q - helm/files/config.alloy
	@tmp=$$(mktemp -d); \
	helm template vector vector/vector --version 0.58.0 -n monitoring -f helm/vector.yaml \
	  --set-file customConfig.transforms.api_json.source=helm/files/api.vrl \
	  | python3 -c 'import sys,yaml; [sys.stdout.write(d["data"]["vector.yaml"]) for d in yaml.safe_load_all(sys.stdin) if d and d.get("kind")=="ConfigMap"]' \
	  > $$tmp/vector.yaml; \
	docker run --rm -v "$$tmp:/c" $(VECTOR_IMAGE) validate --no-environment /c/vector.yaml; \
	rc=$$?; rm -rf $$tmp; exit $$rc

deploy-alloy: check-agents
	helm upgrade --install alloy grafana/alloy --version 1.13.0 -n monitoring \
	  -f helm/alloy.yaml --set-file alloy.configMap.content=helm/files/config.alloy --wait

deploy-vector: check-agents
	helm upgrade --install vector vector/vector --version 0.58.0 -n monitoring \
	  -f helm/vector.yaml --set-file customConfig.transforms.api_json.source=helm/files/api.vrl --wait
