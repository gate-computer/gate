# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

PWD		:= $(shell pwd)

GO		?= vgo
SETCAP		?= setcap

CGROUP_BACKEND	?= systemd

GOPACKAGES	:= $(shell find . -name '*.go' -printf '%h\n' | sort -u)
TESTS		:= $(dir $(wildcard tests/*/Makefile))

-include config.mk

export GATE_TEST_LIBDIR		:= $(PWD)/lib
export GATE_TEST_DIR		:= $(PWD)/tests

lib:
	$(MAKE) -C run/container CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader

bin:
	$(GO) build $(GOBUILDFLAGS) -o bin/containerd ./cmd/gate-containerd
	$(GO) build $(GOBUILDFLAGS) -o bin/runner ./cmd/gate-runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server ./cmd/gate-server
	$(GO) build $(GOBUILDFLAGS) -o bin/webio ./cmd/gate-webio

devlibs:
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C libcxx
	$(MAKE) -C capi

tests: devlibs
	$(MAKE) -C run/loader/tests
	$(MAKE) -C examples/gate-talk/payload
	$(GO) build $(GOBUILDFLAGS) -o bin/talk ./examples/gate-talk
	set -e; $(foreach dir,$(TESTS),$(MAKE) -C $(dir);)

all: lib bin devlibs tests

capabilities:
	chmod -R go-w lib
	chmod go-wx lib/gate-container
	$(SETCAP) cap_sys_admin,cap_dac_override,cap_setuid+ep lib/gate-container

check: lib bin tests
	$(MAKE) -C run/loader/tests check
	$(GO) vet $(GOPACKAGES)
	$(GO) test $(GOTESTFLAGS) $(GOPACKAGES)
	bin/runner tests/echo/prog.wasm
	bin/runner tests/cxx/prog.wasm
	bin/runner -c benchmark.repeat=2 tests/hello/prog.wasm
	bin/runner -c program.arg=-32 tests/hello/prog.wasm | grep "HELLO WORLD"
	bin/runner -c benchmark.repeat=100 tests/nop/prog.wasm
	bin/runner tests/peer/prog.wasm tests/peer/prog.wasm

check-toolchain:
	$(MAKE) -C examples/toolchain
	bin/runner examples/toolchain/example.wasm

benchmark: lib bin tests
	$(GO) test -run=^$$ -bench=.* -v $(GOPACKAGES)
	bin/runner -c benchmark.repeat=10000 -benchmark.timing tests/nop/prog.wasm

clean:
	rm -rf bin lib
	$(MAKE) -C run/container clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(MAKE) -C libcxx clean
	$(MAKE) -C capi clean
	$(MAKE) -C examples/gate-talk/payload clean
	$(MAKE) -C examples/toolchain clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: lib bin devlibs tests all capabilities check check-toolchain benchmark clean
