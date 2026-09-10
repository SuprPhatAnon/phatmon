.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt
ARGS ?=
TEST_ARGS ?=
CODEX ?= codex
CODEX_ARGS ?=
INSTALL_ARGS ?=
SESSION_NAME ?= consumable
SESSION_DIR ?= $(CURDIR)
WORK_ADDR ?= ws://127.0.0.1:4500
PERSONAL_ADDR ?= ws://127.0.0.1:4501
ifeq ($(SESSION_NAME),personal)
SESSION_HOME ?= $(HOME)/.codex
SERVER_ADDR ?= $(PERSONAL_ADDR)
else
SESSION_HOME ?= $(HOME)/.codex-homes/$(SESSION_NAME)
SERVER_ADDR ?= $(WORK_ADDR)
endif

# This Go project uses .venv as a local dependency/toolchain cache, not a Python venv.
VENV := $(CURDIR)/.venv
export GOPATH := $(VENV)/go
export GOMODCACHE := $(VENV)/pkg/mod
export GOCACHE := $(VENV)/cache/build
export GOBIN := $(VENV)/bin
export GOTMPDIR := $(VENV)/tmp

.PHONY: help init build install run server codex live live-one demo list-homes init-config test test-integration vet fmt fmt-check check clean

help:
	@printf '%s\n' \
	  'Phatmon development commands' \
	  '' \
	  '  make init              Prepare .venv and download missing Go dependencies' \
	  '  make build             Build bin/phatmon' \
	  '  make install           Install user services and per-home environment files' \
	  '  make run               Build and run (ARGS="--flag value" passes arguments)' \
	  '  make server            Start the shared Codex server (leave running)' \
	  '  make codex             Open Codex on the shared server in SESSION_DIR' \
	  '  make live              Connect Phatmon to both work and personal servers' \
	  '  make live-one          Connect only SESSION_NAME to SERVER_ADDR' \
	  '  make demo              Build and run with synthetic data' \
	  '  make list-homes        Print discovered/configured homes without starting Codex' \
	  '  make init-config       Create configuration if missing, then print its path' \
	  '  make test              Run all tests with the race detector' \
	  '  make test-integration  Check installed Codex using an isolated temporary home' \
	  '  make vet               Run Go static checks' \
	  '  make fmt               Format Go source' \
	  '  make fmt-check         Check formatting without changing files' \
	  '  make check             Check formatting, run vet, and run race tests' \
	  '  make clean             Remove build output; keep .venv for reuse' \
	  '' \
	  'Live defaults: WORK_ADDR=ws://127.0.0.1:4500, PERSONAL_ADDR=ws://127.0.0.1:4501.' \
	  'Server/Codex default to SESSION_NAME=consumable; use personal for ~/.codex.' \
	  'Override SESSION_HOME, SERVER_ADDR, SESSION_DIR, CODEX, or CODEX_ARGS as needed.' \
	  '' \
	  'Go 1.24+ is required. The race detector also needs a C compiler.' \
	  'Codex is required for live sessions and test-integration, not for demo or unit tests.'

# go mod download reuses cached modules and fetches only what is missing.
# Every build/test/run path passes through init, including after .venv is removed.
init:
	@command -v "$(GO)" >/dev/null 2>&1 || { printf '%s\n' 'Go is missing. Install Go 1.24+ or set GO=/path/to/go.' >&2; exit 1; }
	@mkdir -p "$(GOPATH)" "$(GOMODCACHE)" "$(GOCACHE)" "$(GOBIN)" "$(GOTMPDIR)"
	"$(GO)" mod download

build: init
	@mkdir -p bin
	"$(GO)" build -trimpath -o bin/phatmon ./cmd/phatmon

install: build
	"$(GO)" run ./cmd/phatmon-install --codex "$(CODEX)" --work-addr "$(WORK_ADDR)" --personal-addr "$(PERSONAL_ADDR)" $(INSTALL_ARGS)

run: build
	./bin/phatmon $(ARGS)

server:
	@test -d "$(SESSION_HOME)" || { printf 'Codex home does not exist: %s\n' "$(SESSION_HOME)" >&2; exit 1; }
	CODEX_HOME="$(SESSION_HOME)" "$(CODEX)" app-server --listen "$(SERVER_ADDR)"

codex:
	@test -d "$(SESSION_HOME)" || { printf 'Codex home does not exist: %s\n' "$(SESSION_HOME)" >&2; exit 1; }
	CODEX_HOME="$(SESSION_HOME)" "$(CODEX)" --remote "$(SERVER_ADDR)" --cd "$(SESSION_DIR)" $(CODEX_ARGS)

live: build
	./bin/phatmon --connect "consumable=$(WORK_ADDR)" --connect "personal=$(PERSONAL_ADDR)" $(ARGS)

live-one: build
	./bin/phatmon --connect "$(SESSION_NAME)=$(SERVER_ADDR)" $(ARGS)

demo: build
	./bin/phatmon --demo $(ARGS)

list-homes: build
	./bin/phatmon --list-homes $(ARGS)

init-config: build
	./bin/phatmon --init-config $(ARGS)

test: init
	"$(GO)" test -race $(TEST_ARGS) ./...

test-integration: init
	@command -v codex >/dev/null 2>&1 || { printf '%s\n' 'Codex CLI is required for the integration check.' >&2; exit 1; }
	PHATMON_TEST_CODEX=1 "$(GO)" test -v ./internal/codex -run '^TestInstalledCodex$$' -count=1

vet: init
	"$(GO)" vet ./...

fmt:
	"$(GOFMT)" -w cmd internal

fmt-check:
	@files=$$("$(GOFMT)" -l cmd internal) || exit 1; \
	if [ -n "$$files" ]; then \
	  printf 'Run make fmt to format:\n%s\n' "$$files" >&2; exit 1; \
	fi

check: fmt-check vet test

clean:
	rm -rf bin
