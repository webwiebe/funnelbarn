SHELL := /bin/sh
.DEFAULT_GOAL := help

SPEC_DIR := specs/001-analytics-funnel
LOCAL_DIRS := .cache var
FIND_PRUNE := \( -path './.git' -o -path './.cache' -o -path './.claude' -o -path '*/node_modules' -o -path '*/dist' -o -path '*/build' -o -path '*/.venv' \) -prune -o
export XDG_CACHE_HOME := $(CURDIR)/.cache
export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/go-mod

.PHONY: help setup build test lint dev docker-build clean

help:
	@printf '%s\n' \
		'Trailpost targets:' \
		'  setup        install all dependencies' \
		'  build        build Go binary and frontend' \
		'  test         run all tests' \
		'  lint         run linters' \
		'  dev          start docker compose stack' \
		'  docker-build build Docker images locally' \
		'  clean        remove build artifacts'

setup:
	@set -eu; \
	for dir in $(LOCAL_DIRS) $(GOCACHE) $(GOMODCACHE); do mkdir -p "$$dir"; done; \
	for mod in $$(find . $(FIND_PRUNE) -name go.mod -print 2>/dev/null); do \
		dir=$$(dirname "$$mod"); \
		echo "[setup] go $$dir"; \
		(cd "$$dir" && go mod download); \
	done; \
	for pkg in $$(find . $(FIND_PRUNE) -name package.json -print 2>/dev/null); do \
		dir=$$(dirname "$$pkg"); \
		echo "[setup] node $$dir"; \
		if [ -f "$$dir/package-lock.json" ]; then \
			(cd "$$dir" && npm ci); \
		else \
			(cd "$$dir" && npm install); \
		fi; \
	done

build:
	@set -eu; \
	for dir in $(GOCACHE) $(GOMODCACHE); do mkdir -p "$$dir"; done; \
	for mod in $$(find . $(FIND_PRUNE) -name go.mod -print 2>/dev/null); do \
		dir=$$(dirname "$$mod"); \
		if find "$$dir" -name '*.go' -print -quit | grep -q .; then \
			echo "[build] go $$dir"; \
			(cd "$$dir" && go build ./...); \
		fi; \
	done; \
	for pkg in $$(find . $(FIND_PRUNE) -name package.json -print 2>/dev/null); do \
		dir=$$(dirname "$$pkg"); \
		echo "[build] node $$dir"; \
		(cd "$$dir" && npm run build --if-present); \
	done

test:
	@set -eu; \
	for dir in $(GOCACHE) $(GOMODCACHE); do mkdir -p "$$dir"; done; \
	for mod in $$(find . $(FIND_PRUNE) -name go.mod -print 2>/dev/null); do \
		dir=$$(dirname "$$mod"); \
		if find "$$dir" -name '*.go' -print -quit | grep -q .; then \
			echo "[test] go $$dir"; \
			(cd "$$dir" && go test ./...); \
		fi; \
	done; \
	for pkg in $$(find . $(FIND_PRUNE) -name package.json -print 2>/dev/null); do \
		dir=$$(dirname "$$pkg"); \
		echo "[test] node $$dir"; \
		(cd "$$dir" && npm run test --if-present); \
	done

lint:
	@set -eu; \
	for dir in $(GOCACHE) $(GOMODCACHE); do mkdir -p "$$dir"; done; \
	for mod in $$(find . $(FIND_PRUNE) -name go.mod -print 2>/dev/null); do \
		dir=$$(dirname "$$mod"); \
		if find "$$dir" -name '*.go' -print -quit | grep -q .; then \
			echo "[lint] go $$dir"; \
			(cd "$$dir" && go vet ./...); \
			formatted=$$(cd "$$dir" && find . $(FIND_PRUNE) -name '*.go' -print0 | xargs -0 gofmt -l); \
			if [ -n "$$formatted" ]; then \
				printf '%s\n' "$$formatted"; \
				exit 1; \
			fi; \
		fi; \
	done; \
	for pkg in $$(find . $(FIND_PRUNE) -name package.json -print 2>/dev/null); do \
		dir=$$(dirname "$$pkg"); \
		echo "[lint] node $$dir"; \
		(cd "$$dir" && npm run lint --if-present); \
	done

dev:
	@set -eu; \
	if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 && docker info >/dev/null 2>&1; then \
		docker compose up --build; \
	else \
		echo "docker compose is not available"; \
	fi

docker-build:
	@set -eu; \
	if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then echo "[docker-build] docker is not available"; exit 0; fi; \
	found=0; \
	if [ -f deploy/docker/service.Dockerfile ]; then \
		found=1; \
		docker build -f deploy/docker/service.Dockerfile -t trailpost/service:local .; \
	fi; \
	if [ -f deploy/docker/web.Dockerfile ]; then \
		found=1; \
		docker build -f deploy/docker/web.Dockerfile -t trailpost/web:local .; \
	fi; \
	if [ "$$found" -eq 0 ]; then echo "[docker-build] no Dockerfiles found yet"; fi

clean:
	@rm -rf dist/ .cache/go-build
	@echo "cleaned"
