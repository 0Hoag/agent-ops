SHELL := /bin/sh

.PHONY: dev test lint build ci

dev:
	go run .

test:
	go test ./...

lint:
	golangci-lint run

build:
	go build ./...

ci: test lint build
