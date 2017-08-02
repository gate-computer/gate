PWD		:= $(shell pwd)

GO		?= go
SETCAP		?= setcap

CGROUP_BACKEND	?= systemd

GOPACKAGEPREFIX	:= github.com/tsavola/gate

TESTS		:= $(dir $(wildcard tests/*/Makefile))

-include config.mk

GOPACKAGES := \
	$(GOPACKAGEPREFIX) \
	$(GOPACKAGEPREFIX)/cmd/containerd \
	$(GOPACKAGEPREFIX)/cmd/runner \
	$(GOPACKAGEPREFIX)/cmd/server \
	$(GOPACKAGEPREFIX)/cmd/talk \
	$(GOPACKAGEPREFIX)/cmd/webio \
	$(GOPACKAGEPREFIX)/internal/memfd \
	$(GOPACKAGEPREFIX)/run \
	$(GOPACKAGEPREFIX)/server \
	$(GOPACKAGEPREFIX)/service \
	$(GOPACKAGEPREFIX)/service/defaults \
	$(GOPACKAGEPREFIX)/service/echo \
	$(GOPACKAGEPREFIX)/service/origin \
	$(GOPACKAGEPREFIX)/service/peer

export GATE_TEST_COMMONGROUP	= $(word 2,$(shell groups))
export GATE_TEST_CONTAINERUSER	= sys
export GATE_TEST_EXECUTORUSER	= daemon
export GATE_TEST_LIBDIR		= $(PWD)/lib
export GATE_TEST_DIR		= $(PWD)/tests

run = bin/runner \
	-common-gid=$(shell getent group $(GATE_TEST_COMMONGROUP) | cut -d: -f3) \
	-container-uid=$(shell id -u $(GATE_TEST_CONTAINERUSER)) \
	-container-gid=$(shell id -g $(GATE_TEST_CONTAINERUSER)) \
	-executor-uid=$(shell id -u $(GATE_TEST_EXECUTORUSER)) \
	-executor-gid=$(shell id -g $(GATE_TEST_EXECUTORUSER))

build:
	$(MAKE) -C run/container CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) build $(GOBUILDFLAGS) -o bin/containerd $(GOPACKAGEPREFIX)/cmd/containerd
	$(GO) build $(GOBUILDFLAGS) -o bin/runner $(GOPACKAGEPREFIX)/cmd/runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server $(GOPACKAGEPREFIX)/cmd/server
	$(GO) build $(GOBUILDFLAGS) -o bin/webio $(GOPACKAGEPREFIX)/cmd/webio

all: build
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C run/loader/tests
	$(MAKE) -C cmd/talk/payload
	$(GO) build $(GOBUILDFLAGS) -o bin/talk $(GOPACKAGEPREFIX)/cmd/talk
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir);)

capabilities:
	chmod -R go-w lib
	chmod go-wx lib/container
	$(SETCAP) cap_dac_override,cap_setgid,cap_setuid+ep lib/container

check:
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) test -race $(GOPACKAGES)
	$(run) tests/echo/prog.wasm
	$(run) -repeat=2 tests/hello/prog.wasm
	$(run) -repeat=100 tests/nop/prog.wasm
	$(run) tests/peer/prog.wasm tests/peer/prog.wasm

benchmark:
	$(GO) test -run=^$$ -bench=.* -v $(GOPACKAGES)
	$(run) -repeat=10000 -dump-time tests/nop/prog.wasm

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/container clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(MAKE) -C cmd/talk/payload clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: build all capabilities check benchmark clean
