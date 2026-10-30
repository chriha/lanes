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

.PHONY: build install dev test lint
