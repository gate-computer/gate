PWD		:= $(shell pwd)

GO		?= go
export GOPATH	:= $(PWD)

GOPACKAGEPREFIX	:= github.com/tsavola/gate

-include config.mk

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/cmd/runner \
	$(GOPACKAGEPREFIX)/cmd/server \
	$(GOPACKAGEPREFIX)/internal/memfd \
	$(GOPACKAGEPREFIX)/run

export GATE_TEST_EXECUTOR	:= $(PWD)/bin/executor
export GATE_TEST_LOADER		:= $(PWD)/bin/loader
export GATE_TEST_WASM		:= $(PWD)/tests/hello/prog.wasm

build:
	$(GO) get github.com/gorilla/websocket
	$(GO) get github.com/tsavola/wag
	$(GO) get golang.org/x/crypto/acme/autocert
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/cmd/runner
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/cmd/server

all: build
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C run/loader/tests
	set -e; for DIR in tests/*/; do $(MAKE) -C $$DIR; done

check: all
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) get github.com/bnagy/gapstone
	$(GO) test -race -v $(GOPACKAGES)

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	for DIR in tests/*/; do $(MAKE) -C $$DIR clean; done

.PHONY: build all check clean
