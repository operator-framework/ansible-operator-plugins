SHELL = /bin/bash

# IMAGE_VERSION represents the ansible-operator, helm-operator, and scorecard subproject versions.
# This value must be updated to the release tag of the most recent release, a change that must
# occur in the release commit. IMAGE_VERSION will be removed once each subproject that uses this
# version is moved to a separate repo and release process.
export IMAGE_VERSION = v1.34.3
# Build-time variables to inject into binaries
export SIMPLE_VERSION := $(shell (test "$(shell git describe --tags)" = "$(shell git describe --tags --abbrev=0)" && echo $(shell git describe --tags)) || echo $(shell git describe --tags --abbrev=0)+git)
export GIT_VERSION := $(shell git describe --dirty --tags --always)
export GIT_COMMIT := $(shell git rev-parse HEAD)
export K8S_VERSION = 1.29.0

# Build settings
export TOOLS_DIR = tools/bin
export SCRIPTS_DIR = tools/scripts
REPO = $(shell go list -m)
BUILD_DIR = .
export GO_BUILD_ASMFLAGS = all=-trimpath=$(shell dirname $(PWD))
export GO_BUILD_GCFLAGS = all=-trimpath=$(shell dirname $(PWD))
export GO_BUILD_LDFLAGS =  \
    -X '$(REPO)/internal/version.Version=$(SIMPLE_VERSION)' \
    -X '$(REPO)/internal/version.GitVersion=$(GIT_VERSION)' \
    -X '$(REPO)/internal/version.GitCommit=$(GIT_COMMIT)' \
    -X '$(REPO)/internal/version.KubernetesVersion=v$(K8S_VERSION)' \
    -X '$(REPO)/internal/version.ImageVersion=$(IMAGE_VERSION)' \
 \

GO_BUILD_ARGS = \
  -gcflags "$(GO_BUILD_GCFLAGS)" -asmflags "$(GO_BUILD_ASMFLAGS)" -ldflags "$(GO_BUILD_LDFLAGS)"

export GO111MODULE = on
export CGO_ENABLED = 0
export PATH := $(PWD)/$(BUILD_DIR):$(PWD)/$(TOOLS_DIR):$(PATH)

export IMAGE_REPO ?= quay.io/operator-framework/ansible-operator
export IMAGE_TAG ?= dev

# bingo manages consistent tooling versions for things like setup-envtest, goreleaser, kind, etc.
include .bingo/Variables.mk

# This is to allow for building and testing on Apple Silicon.
# These values default to the host's GOOS and GOARCH, but should
# be overridden when running builds and tests on Apple Silicon unless
# you are only building the binary
BUILD_GOOS ?= $(shell go env GOOS)
BUILD_GOARCH ?= $(shell go env GOARCH)

##@ Development

.PHONY: generate
generate: build # Generate CLI docs and samples
	rm -rf testdata
	go run ./hack/generate/samples/generate_testdata.go
	go generate ./...

.PHONY: fix
fix: $(GOLANGCI_LINT) ## Fixup files in the repo.
	go mod tidy
	go fmt ./...
	$(GOLANGCI_LINT) run --fix --timeout=5m

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run the lint check
	$(GOLANGCI_LINT) run

.PHONY: clean
clean: ## Cleanup build artifacts and tool binaries.
	rm -rf $(BUILD_DIR) dist $(TOOLS_DIR)

##@ Build

.PHONY: install
install: ## Install ansible-operator
	GOOS=$(BUILD_GOOS) GOARCH=$(BUILD_GOARCH) go install $(GO_BUILD_ARGS) ./cmd/ansible-operator

.PHONY: build
build: ## Build ansible-operator
	@mkdir -p $(BUILD_DIR)
	GOOS=$(BUILD_GOOS) GOARCH=$(BUILD_GOARCH) go build $(GO_BUILD_ARGS) -o $(BUILD_DIR) ./cmd/ansible-operator

.PHONY: build/ansible-operator
build/ansible-operator:
	GOOS=$(BUILD_GOOS) GOARCH=$(BUILD_GOARCH) go build $(GO_BUILD_ARGS) -o $(BUILD_DIR)/$(@F) ./cmd/$(@F)

##@ Dev image build

# Convenience wrapper for building all remotely hosted images.
.PHONY: image-build
IMAGE_TARGET_LIST = ansible-operator
image-build: build $(foreach i,$(IMAGE_TARGET_LIST),image/$(i)) ## Build all images.

# Build an image.
BUILD_IMAGE_REPO = quay.io/operator-framework
# When running in a terminal, this will be false. If true (ex. CI), print plain progress.
ifneq ($(shell test -t 0; echo $$?),0)
DOCKER_PROGRESS = --progress plain
endif
image/%: export DOCKER_CLI_EXPERIMENTAL = enabled
image/%:
	docker buildx build $(DOCKER_PROGRESS) -t $(BUILD_IMAGE_REPO)/$*:$(IMAGE_TAG) -f ./images/$*/Dockerfile --load . --no-cache
##@ Release

## TODO: Add release targets here

##@ Test

.PHONY: test-all
test-all: test-static test-e2e ## Run all tests

.PHONY: test-static
test-static: test-sanity test-unit ## Run all non-cluster-based tests

.PHONY: test-sanity
test-sanity: generate fix ## Test repo formatting, linting, etc.
	git diff --exit-code # fast-fail if generate or fix produced changes
	./hack/check-license.sh
	./hack/check-error-log-msg-format.sh
	go vet ./...
	make lint
	git diff --exit-code # diff again to ensure other checks don't change repo

.PHONY: test-docs
test-docs: ## Test doc links
	go run ./release/changelog/gen-changelog.go -validate-only
	git submodule update --init --recursive website/
	./hack/check-links.sh

.PHONY: test-unit
ENVTEST_VERSION = $(shell go list -m k8s.io/client-go | cut -d" " -f2 | sed 's/^v0\.\([[:digit:]]\{1,\}\)\.[[:digit:]]\{1,\}$$/1.\1.x/')
TEST_PKGS = $(shell go list ./... | grep -v -E 'github.com/operator-framework/ansible-operator-plugins/test/')
test-unit: $(SETUP_ENVTEST) ## Run unit tests
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_VERSION) -p path)" go test -coverprofile=coverage.out -covermode=count -short $(TEST_PKGS)

e2e_tests := test-e2e-ansible test-e2e-ansible-molecule
e2e_targets := test-e2e $(e2e_tests)
.PHONY: $(e2e_targets)

.PHONY: test-e2e-setup
export KIND_CLUSTER := osdk-test

test-e2e-setup:: build dev-install cluster-create

.PHONY: cluster-create
cluster-create:: $(KIND)
	[[ "`$(KIND) get clusters`" =~ "$(KIND_CLUSTER)" ]] || $(KIND) create cluster --image="kindest/node:v$(K8S_VERSION)" --name $(KIND_CLUSTER)
	$(KIND) export kubeconfig --name $(KIND_CLUSTER)

.PHONY: dev-install
dev-install::
	$(SCRIPTS_DIR)/fetch kubectl $(K8S_VERSION) # Install kubectl AFTER envtest because envtest includes its own kubectl binary

.PHONY: test-e2e-teardown
test-e2e-teardown: $(KIND)
	$(KIND) delete cluster --name $(KIND_CLUSTER)
	rm -f $(KUBECONFIG)

# Double colon rules allow repeated rule declarations.
# Repeated rules are executed in the order they appear.
$(e2e_targets):: test-e2e-setup
test-e2e:: $(e2e_tests) ## Run e2e tests

test-e2e-ansible:: image/ansible-operator ## Run Ansible e2e tests
	go test ./test/e2e/ansible -v -ginkgo.v
test-e2e-ansible-molecule:: install dev-install image/ansible-operator ## Run molecule-based Ansible e2e tests
	go run ./hack/generate/samples/molecule/generate.go
	./hack/tests/e2e-ansible-molecule.sh

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

export ENABLE_RELEASE_PIPELINE ?= false
export GORELEASER_ARGS         ?= --snapshot --clean --timeout=120m
release: $(GORELEASER) ## Runs goreleaser. By default, this will run only as a snapshot and will not publish any artifacts unless it is run with different arguments. To override the arguments, run with "GORELEASER_ARGS=...". When run as a github action from a tag, this target will publish a full release.
	$(GORELEASER) $(GORELEASER_ARGS)

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help screen.
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
