NAME    := lanes
DEV_BIN ?= $(HOME)/.local/bin/$(NAME)-dev
VERSION ?= dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(NAME) .

install:
	go install -ldflags "-X main.version=$(VERSION)" .

# Build and symlink the dev binary onto PATH (override with DEV_BIN=...).
dev: build
	mkdir -p $(dir $(DEV_BIN))
	ln -sf $(CURDIR)/bin/$(NAME) $(DEV_BIN)
	@echo "linked $(DEV_BIN) -> $(CURDIR)/bin/$(NAME)"

test:
	go vet ./...
	go test ./...

lint:
	golangci-lint run

# Time to first list, the startup goal (docs/design.md § Startup); CI's bench job compares it against the base.
bench:
	go test -run '^$$' -bench TimeToFirstList -count=6 ./internal/ui

# Tag the next release from the Conventional Commits since the last tag (see README § Development).
version:
	@scripts/version.sh

.PHONY: build install dev test lint bench version
