# Kaddie build, test and lint targets.
#
# Every target runs under bash: Git Bash on Windows, bash in CI. Tools are
# installed into ./bin at pinned versions so local runs match CI.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

# --- Versions ---------------------------------------------------------------

CONTROLLER_TOOLS_VERSION ?= v0.23.0
GOLANGCI_LINT_VERSION    ?= v2.15.0
SETUP_ENVTEST_VERSION    ?= v0.25.3
# Minimum supported Kubernetes (base §13). envtest always runs against it.
ENVTEST_K8S_VERSION      ?= 1.28.x

# --- Paths and helpers ------------------------------------------------------

ROOT     := $(abspath .)
LOCALBIN := $(ROOT)/bin
EXE      := $(if $(filter Windows_NT,$(OS)),.exe,)

CONTROLLER_GEN := $(LOCALBIN)/controller-gen$(EXE)
GOLANGCI_LINT  := $(LOCALBIN)/golangci-lint$(EXE)
SETUP_ENVTEST  := $(LOCALBIN)/setup-envtest$(EXE)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/morozj01/kaddie/pkg/version.Version=$(VERSION) \
           -X github.com/morozj01/kaddie/pkg/version.Commit=$(COMMIT)

comma := ,
# FOCUS is a comma-separated list of spec test IDs or test names, for example
# FOCUS=T6,T43,T53 or FOCUS=oidc-spike. It becomes a case-insensitive -run
# regex matching any underscore-separated part of a test function's name, so
# TestT9_GT6_GT40_StatusWrites runs for FOCUS=T9 and for FOCUS=GT40.
FOCUS ?=
FOCUS_RE := $(subst -,_,$(subst $(comma),|,$(FOCUS)))
RUN_FLAG := $(if $(FOCUS),-run '(?i)^Test(.*_)?($(FOCUS_RE))(_|$$)',)
# PKG narrows `make test` to some packages, e.g. PKG=./pkg/proposal/...
PKG ?= ./...

# kind cluster used by e2e tests.
K8S          ?= 1.28
KIND_CLUSTER ?= kaddie-e2e

##@ General

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n"} \
	  /^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-16s %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n%s\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: build
build: ## Build every component binary into ./dist.
	@mkdir -p dist
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/ ./cmd/...

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Regenerate deepcopy code and the CRDs in charts/kaddie/crds.
	$(CONTROLLER_GEN) object paths=./pkg/apis/v1alpha1
	$(CONTROLLER_GEN) crd:crdVersions=v1 paths=./pkg/apis/v1alpha1 output:crd:artifacts:config=charts/kaddie/crds
	go run ./hack/crdpost charts/kaddie/crds

.PHONY: fmt
fmt: $(GOLANGCI_LINT) ## Format Go code.
	$(GOLANGCI_LINT) fmt ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint (and ESLint for web/ once it exists).
	$(GOLANGCI_LINT) run ./...
	@if [ -f web/package.json ]; then (cd web && npm run lint); fi

.PHONY: tidy
tidy: ## Run go mod tidy.
	go mod tidy

##@ Tests

.PHONY: test
test: ## Run unit tests (go test). PKG=... narrows the packages.
	go test -count=1 $(PKG)

.PHONY: test-envtest
test-envtest: $(SETUP_ENVTEST) envtest-clean ## Run CRD/CEL tests against a real 1.28 kube-apiserver. FOCUS=crd narrows them.
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN)/k8s -p path)" \
	  go test -count=1 -tags envtest ./test/envtest/... $(RUN_FLAG) -v

.PHONY: vendor-openapi
vendor-openapi: $(SETUP_ENVTEST) envtest-clean ## Re-record pkg/proposal/schema/openapi from a real 1.28 kube-apiserver.
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN)/k8s -p path)" \
	  go run ./hack/vendor-openapi

.PHONY: envtest-clean
envtest-clean: ## Stop etcd and kube-apiserver left running by interrupted envtest runs.
	bash test/infra/envtest-clean.sh

.PHONY: chart-test
chart-test: ## helm lint plus render tests, with GitOps on and off.
	helm lint charts/kaddie
	helm lint charts/kaddie --set gitops.enabled=true
	go test -count=1 ./test/chart/...

.PHONY: e2e
e2e: ## Run kind e2e tests (cluster from kind-up). FOCUS=T6,T43 narrows them.
	KIND_CLUSTER=$(KIND_CLUSTER) go test -count=1 -tags e2e -timeout 30m ./test/e2e/... $(RUN_FLAG) -v

.PHONY: traceability
traceability: ## Check test/TRACEABILITY.md against the tests that claim spec IDs.
	go run ./hack/traceability

##@ Clusters

.PHONY: kind-up
kind-up: ## Create the e2e kind cluster (K8S=1.28 or K8S=latest), with OIDC and audit logging.
	bash test/infra/kind-up.sh "$(K8S)" "$(KIND_CLUSTER)"

.PHONY: kind-down
kind-down: ## Delete the e2e kind cluster and the mock OIDC issuer.
	bash test/infra/kind-down.sh "$(KIND_CLUSTER)"

##@ Tools

.PHONY: tools
tools: $(CONTROLLER_GEN) $(GOLANGCI_LINT) $(SETUP_ENVTEST) ## Install pinned tools into ./bin.

$(CONTROLLER_GEN):
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

$(GOLANGCI_LINT):
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(SETUP_ENVTEST):
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)
