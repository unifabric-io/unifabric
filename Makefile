ROOT_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
BIN_DIR ?= $(ROOT_DIR)/bin
CHART_DIR ?= $(ROOT_DIR)/chart
CRD_DIR ?= $(CHART_DIR)/crds

GOFLAGS ?=
GOCACHE ?= /tmp/unifabric-go-build
COVERAGE_FILE ?= $(ROOT_DIR)/.tmp/coverage.out
COVERAGE_HTML ?= $(ROOT_DIR)/.tmp/coverage.html
E2E_WAIT_TIMEOUT_MINUTES ?= 30
TOPOLOGY_DIR ?= e2e/topology

CONTROLLER_GEN ?= $(ROOT_DIR)/hack/controller-gen.sh
HELM_DOCS ?= helm-docs
PROTOC ?= protoc

IMAGE_REGISTRY ?= ghcr.io/unifabric-io
IMAGE_TAG ?= dev
IMAGE_PLATFORMS ?= linux/amd64,linux/arm64
CONTROLLER_IMAGE ?= $(IMAGE_REGISTRY)/unifabric-controller:$(IMAGE_TAG)
AGENT_IMAGE ?= $(IMAGE_REGISTRY)/unifabric-agent:$(IMAGE_TAG)
SWITCH_AGENT_IMAGE ?= $(IMAGE_REGISTRY)/unifabric-switch-agent:$(IMAGE_TAG)

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make build              - Build the unifabric controller, agent, and switch-agent binaries"
	@echo "  make image              - Build the unifabric controller, agent, and switch-agent images"
	@echo "  make image-push         - Build and push the unifabric controller, agent, and switch-agent images"
	@echo "  make test-unit          - Run unit tests with coverage"
	@echo "  make test-e2e           - Run E2E validation"
	@echo "  make test-coverage      - Generate HTML coverage report (.tmp/coverage.html)"
	@echo "  make check-license      - Check Go source license headers"
	@echo "  make crd                - Generate Kubernetes API deepcopy code and CRDs"
	@echo "  make helm-docs          - Generate Helm chart documentation"
	@echo "  make proto-switch-agent - Generate switch-agent gRPC stubs"
	@echo "  make clean              - Remove build artifacts"

.PHONY: all
all: build

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BIN_DIR)/controller ./cmd/controller
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BIN_DIR)/agent ./cmd/agent
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BIN_DIR)/switch-agent ./cmd/switch-agent

.PHONY: image
image:
	docker buildx build -t $(CONTROLLER_IMAGE) -f image/controller/Dockerfile .
	docker buildx build -t $(AGENT_IMAGE) -f image/agent/Dockerfile .
	docker buildx build -t $(SWITCH_AGENT_IMAGE) -f image/switch-agent/Dockerfile .

.PHONY: image-push
image-push:
	docker buildx build --platform $(IMAGE_PLATFORMS) --push -t $(CONTROLLER_IMAGE) -f image/controller/Dockerfile .
	docker buildx build --platform $(IMAGE_PLATFORMS) --push -t $(AGENT_IMAGE) -f image/agent/Dockerfile .
	docker buildx build --platform $(IMAGE_PLATFORMS) --push -t $(SWITCH_AGENT_IMAGE) -f image/switch-agent/Dockerfile .

.PHONY: test-unit
test-unit:
	mkdir -p $(dir $(COVERAGE_FILE))
	GOCACHE=$(GOCACHE) go test $(GOFLAGS) -count=1 -coverprofile=$(COVERAGE_FILE) ./cmd/... ./pkg/...

.PHONY: test-e2e
test-e2e:
	go run ./e2e \
		--timeout-minutes "$(E2E_WAIT_TIMEOUT_MINUTES)" \
		--topology-dir "$(TOPOLOGY_DIR)"

.PHONY: test-coverage
test-coverage: test-unit
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)

.PHONY: check-license
check-license:
	@$(ROOT_DIR)/hack/check-license.sh

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)

.PHONY: crd
crd:
	$(CONTROLLER_GEN) object:headerFile="$(ROOT_DIR)/hack/boilerplate.go.txt" paths="./pkg/api/v1beta1"
	$(CONTROLLER_GEN) crd paths="./pkg/api/v1beta1" output:dir="$(CRD_DIR)"

.PHONY: helm-docs
helm-docs:
	$(HELM_DOCS) -c chart -g chart

.PHONY: proto-switch-agent
proto-switch-agent:
	$(PROTOC) --proto_path=. --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. pkg/switchagent/switchreporter.proto
