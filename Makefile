PWD		:= $(shell pwd)

GO		?= go

GOPACKAGEPREFIX	:= github.com/tsavola/gate

TESTS		:= $(dir $(wildcard tests/*/Makefile))

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
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) build $(GOBUILDFLAGS) -o bin/runner $(GOPACKAGEPREFIX)/cmd/runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server $(GOPACKAGEPREFIX)/cmd/server

all: build
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C run/loader/tests
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir);)

check: all
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) test -race -v $(GOPACKAGES)
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir) check;)

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: build all check clean
