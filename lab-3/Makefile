#   make images VERSION=1.0.0  - build api and worker straight into minikube
#   make vet                   - go vet + gofmt for both services
#
# REGISTRY is part of the image name and has to match the trusted-registry
# policy of part 1. The node runs containerd, so the build is BuildKit's
# buildctl, and build args use its syntax: opt=build-arg:NAME=value.

REGISTRY ?= shop.local
VERSION  ?= 1.0.0

.PHONY: images vet

images:
	minikube image build -t $(REGISTRY)/api:$(VERSION) --build-opt=opt=build-arg:VERSION=$(VERSION) api
	minikube image build -t $(REGISTRY)/worker:$(VERSION) --build-opt=opt=build-arg:VERSION=$(VERSION) worker
	@minikube image ls | grep -E '$(REGISTRY)/(api|worker)'

vet:
	@for s in api worker; do (cd $$s && go vet ./... && test -z "$$(gofmt -l .)") || exit 1; done
	@echo "vet ok"
