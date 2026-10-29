SHELL := /bin/sh
APP_NAME := agent-ops-api
VERSION := $(shell cat .build-version 2>/dev/null || echo "0.1.0")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS := -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'
BINARY_DIR := bin
BINARY_NAME := $(APP_NAME)
DOCKER_IMAGE := $(APP_NAME)
GOCMD := go
GOTEST := $(GOCMD) test
SWAG_VERSION := v1.16.6
.PHONY: build run dev install fmt vet lint test test-coverage ci swagger docker-build
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(BINARY_NAME) .
run: build
	./$(BINARY_DIR)/$(BINARY_NAME)
dev:
	air
install:
	go mod download
	go install github.com/air-verse/air@latest
	go install github.com/swaggo/swag/cmd/swag@latest
fmt:
	go fmt ./...
vet:
	go vet ./...
lint:
	golangci-lint run
test:
	go test -v -cover ./...
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
ci:
	@echo "→ fmt"
	@test -z "$$($(GOCMD) fmt ./...)" || { echo "gofmt changes detected"; exit 1; }
	@echo "→ go.mod tidy"
	@$(GOCMD) mod tidy -diff
	@echo "→ vet"
	@$(GOCMD) vet ./...
	@echo "→ lint"
	@$(MAKE) --no-print-directory lint
	@echo "→ test (race)"
	@$(GOTEST) -race -count=1 -timeout=180s ./...
	@echo "→ build"
	@CGO_ENABLED=0 $(GOCMD) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(BINARY_NAME) .
	@echo "✓ ci pipeline passed locally"
swagger:
	go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init --parseDependency --parseInternal
docker-build:
	docker build --platform linux/amd64 --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME="$(BUILD_TIME)" --build-arg GIT_COMMIT=$(GIT_COMMIT) -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .
