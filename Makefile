# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

PWD		:= $(shell pwd)

ifeq ($(GOPATH),)
GOPATH		:= $(HOME)/go
endif

GO		?= go
SETCAP		?= setcap

CGROUP_BACKEND	?= systemd

GOPACKAGEPREFIX	:= github.com/tsavola/gate

TESTS		:= $(dir $(wildcard tests/*/Makefile))

-include config.mk

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/cmd/gate-containerd \
	$(GOPACKAGEPREFIX)/cmd/gate-runner \
	$(GOPACKAGEPREFIX)/cmd/gate-server \
	$(GOPACKAGEPREFIX)/cmd/gate-webio \
	$(GOPACKAGEPREFIX)/examples/gate-talk \
	$(GOPACKAGEPREFIX)/internal/cred \
	$(GOPACKAGEPREFIX)/internal/memfd \
	$(GOPACKAGEPREFIX)/internal/server \
	$(GOPACKAGEPREFIX)/packet \
	$(GOPACKAGEPREFIX)/run \
	$(GOPACKAGEPREFIX)/server \
	$(GOPACKAGEPREFIX)/server/serverconfig \
	$(GOPACKAGEPREFIX)/service \
	$(GOPACKAGEPREFIX)/service/defaults \
	$(GOPACKAGEPREFIX)/service/echo \
	$(GOPACKAGEPREFIX)/service/origin \
	$(GOPACKAGEPREFIX)/service/peer \
	$(GOPACKAGEPREFIX)/webapi \
	$(GOPACKAGEPREFIX)/webserver

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

lib:
	$(MAKE) -C run/container CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader

get:
	test $(PWD) = $(GOPATH)/src/$(GOPACKAGEPREFIX) && $(GO) get -d $(GOPACKAGES)

bin: get
	$(GO) build $(GOBUILDFLAGS) -o bin/containerd $(GOPACKAGEPREFIX)/cmd/gate-containerd
	$(GO) build $(GOBUILDFLAGS) -o bin/runner $(GOPACKAGEPREFIX)/cmd/gate-runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server $(GOPACKAGEPREFIX)/cmd/gate-server
	$(GO) build $(GOBUILDFLAGS) -o bin/webio $(GOPACKAGEPREFIX)/cmd/gate-webio

devlibs:
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C capi

tests: devlibs
	$(MAKE) -C run/loader/tests
	$(MAKE) -C examples/gate-talk/payload
	$(GO) build $(GOBUILDFLAGS) -o bin/talk $(GOPACKAGEPREFIX)/examples/gate-talk
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir);)

all: lib bin devlibs tests

capabilities:
	chmod -R go-w lib
	chmod go-wx lib/container
	$(SETCAP) cap_dac_override,cap_setgid,cap_setuid+ep lib/container

# check and benchmark can't require lib due to the capabilities step

check: bin tests
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) test -race $(GOPACKAGES)
	$(run) tests/echo/prog.wasm
	$(run) -repeat=2 tests/hello/prog.wasm
	$(run) -repeat=100 tests/nop/prog.wasm
	$(run) tests/peer/prog.wasm tests/peer/prog.wasm

check-toolchain:
	$(MAKE) -C examples/toolchain
	$(run) examples/toolchain/example.wasm

benchmark: bin tests
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
	$(MAKE) -C capi clean
	$(MAKE) -C examples/gate-talk/payload clean
	$(MAKE) -C examples/toolchain clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: lib get bin devlibs tests all capabilities check check-toolchain benchmark clean
