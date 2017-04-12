PWD		:= $(shell pwd)

GO		?= go

GOPACKAGEPREFIX	:= github.com/tsavola/gate

TESTS		:= $(dir $(wildcard tests/*/Makefile))

-include config.mk

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/cmd/runner \
	$(GOPACKAGEPREFIX)/cmd/server \
	$(GOPACKAGEPREFIX)/cmd/webio \
	$(GOPACKAGEPREFIX)/internal/memfd \
	$(GOPACKAGEPREFIX)/run \
	$(GOPACKAGEPREFIX)/server \
	$(GOPACKAGEPREFIX)/service \
	$(GOPACKAGEPREFIX)/service/defaults \
	$(GOPACKAGEPREFIX)/service/echo \
	$(GOPACKAGEPREFIX)/service/origin \
	$(GOPACKAGEPREFIX)/service/peer

export GATE_TEST_EXECUTOR	:= $(PWD)/bin/executor
export GATE_TEST_LOADER		:= $(PWD)/bin/loader
export GATE_TEST_DIR		:= $(PWD)/tests

build:
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) build $(GOBUILDFLAGS) -o bin/runner $(GOPACKAGEPREFIX)/cmd/runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server $(GOPACKAGEPREFIX)/cmd/server
	$(GO) build $(GOBUILDFLAGS) -o bin/webio $(GOPACKAGEPREFIX)/cmd/webio

all: build
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C run/loader/tests
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir);)

check: all
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) test -race -v $(GOPACKAGES)
	bin/runner tests/echo/prog.wasm
	bin/runner tests/hello/prog.wasm
	bin/runner tests/peer/prog.wasm tests/peer/prog.wasm

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: build all check clean
