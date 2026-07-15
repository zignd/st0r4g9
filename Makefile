# st0r4g9 — developer tasks.
#
# The `go`/`golangci-lint` invocations are wrapped in `env -u GOROOT` because
# this machine exports a stale GOROOT that otherwise breaks the toolchain.

GO          := env -u GOROOT go
GOLANGCILINT := env -u GOROOT golangci-lint

BIN_DIR := bin
BINARY  := $(BIN_DIR)/st0r4g9
PKG     := ./cmd/st0r4g9

# Runtime defaults (override on the command line, e.g. `make run ADDR=:8080`).
ADDR     ?= :9000
DB       ?= store.db
DATA_DIR ?= data

# Port derived from ADDR (":9000" -> "9000", "0.0.0.0:9000" -> "9000"), used by
# the ngrok tunnel. NGROK_ARGS passes extra flags, e.g.
# `make tunnel NGROK_ARGS="--url=my-name.ngrok.app"`.
PORT      := $(lastword $(subst :, ,$(ADDR)))
NGROK_ARGS ?=

# Containerized run (docker-tunnel). NGROK_CONFIG is bind-mounted so the tunnel
# uses the host's ngrok token; DOCKER_DATA_DIR is bind-mounted so data persists
# on the host across container restarts. Override either on the command line.
NGROK_CONFIG    ?= $(HOME)/Library/Application Support/ngrok/ngrok.yml
DOCKER_DATA_DIR ?= $(CURDIR)/data

.DEFAULT_GOAL := help

## help: show this help
.PHONY: help
help:
	@echo "st0r4g9 — available targets:"
	@grep -E '^## [a-z-]+:' $(MAKEFILE_LIST) | sed 's/^## /  /' | sort

## build: compile the server to bin/st0r4g9
.PHONY: build
build:
	$(GO) build -o $(BINARY) $(PKG)

## run: build and run the server (ADDR/DB/DATA_DIR overridable)
.PHONY: run
run:
	ST0R4G9_ADDR=$(ADDR) ST0R4G9_DB=$(DB) ST0R4G9_DATA_DIR=$(DATA_DIR) $(GO) run $(PKG)

## tunnel: run the server and expose it publicly via ngrok (Ctrl-C stops both)
.PHONY: tunnel
tunnel: build
	@command -v ngrok >/dev/null || { echo "ngrok not found — install it and run 'ngrok config add-authtoken <token>'"; exit 1; }
	@echo "Starting st0r4g9 on $(ADDR) and tunneling port $(PORT) via ngrok…"
	@ST0R4G9_ADDR=$(ADDR) ST0R4G9_DB=$(DB) ST0R4G9_DATA_DIR=$(DATA_DIR) $(BINARY) & \
		server_pid=$$!; \
		trap 'kill $$server_pid 2>/dev/null' EXIT INT TERM; \
		sleep 1; \
		ngrok http $(PORT) $(NGROK_ARGS)

## docker-tunnel: run server + ngrok in containers (data dir + ngrok auth bind-mounted from host; Ctrl-C stops)
.PHONY: docker-tunnel
docker-tunnel:
	@test -f "$(NGROK_CONFIG)" || { echo "ngrok config not found: $(NGROK_CONFIG)"; echo "set NGROK_CONFIG=/path/to/ngrok.yml (or run 'ngrok config add-authtoken <token>' on the host)"; exit 1; }
	@mkdir -p "$(DOCKER_DATA_DIR)"
	@echo "Data → $(DOCKER_DATA_DIR)  |  ngrok auth → $(NGROK_CONFIG)  |  inspector → http://localhost:4040"
	NGROK_CONFIG="$(NGROK_CONFIG)" DATA_DIR="$(DOCKER_DATA_DIR)" docker compose up --build

## docker-down: stop and remove the containers (host data is preserved)
.PHONY: docker-down
docker-down:
	docker compose down

## test: run all tests
.PHONY: test
test:
	$(GO) test ./...

## test-race: run all tests with the race detector
.PHONY: test-race
test-race:
	$(GO) test -race ./...

## cover: run tests and open an HTML coverage report
.PHONY: cover
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

## lint: run golangci-lint
.PHONY: lint
lint:
	$(GOLANGCILINT) run ./...

## vet: run go vet
.PHONY: vet
vet:
	$(GO) vet ./...

## fmt: format all Go files in place
.PHONY: fmt
fmt:
	$(GO) fmt ./...

## fmt-check: fail if any Go file is not gofmt-formatted
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(env -u GOROOT gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

## tidy: sync go.mod/go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy

## check: fmt-check + vet + lint + test (what CI should run)
.PHONY: check
check: fmt-check vet lint test

## clean: remove build artifacts and local runtime state
.PHONY: clean
clean:
	rm -rf $(BIN_DIR) coverage.out $(DB) $(DB)-shm $(DB)-wal $(DATA_DIR)
