# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

PWD		:= $(shell pwd)

GO		?= go
GOFMT		?= gofmt
PROTOC		?= protoc
SETCAP		?= setcap

CGROUP_BACKEND	?= systemd

GEN_SOURCES := \
	server/detail/detail.pb.go \
	server/event/event.pb.go \
	server/event/type.gen.go \
	server/monitor/monitor.pb.go

TESTS		:= $(dir $(wildcard tests/*/Makefile))

-include config.mk

export GO111MODULE		:= on
export GATE_TEST_LIBDIR		:= $(PWD)/lib
export GATE_TEST_DIR		:= $(PWD)/tests

lib:
	$(MAKE) -C run/container CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader

generate: $(GEN_SOURCES)

bin: generate
	$(GO) build $(GOBUILDFLAGS) -o bin/containerd ./cmd/gate-containerd
	$(GO) build $(GOBUILDFLAGS) -o bin/runner ./cmd/gate-runner
	$(GO) build $(GOBUILDFLAGS) -o bin/server ./cmd/gate-server
	$(GO) build $(GOBUILDFLAGS) -o bin/webio ./cmd/gate-webio

devlibs:
	$(MAKE) -C crt
	$(MAKE) -C rustrt
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
	$(GO) vet ./...
	$(GO) test $(GOTESTFLAGS) ./...
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
	$(GO) test -run=^$$ -bench=.* -v ./...
	bin/runner -c benchmark.repeat=10000 -benchmark.timing tests/nop/prog.wasm

bin/protoc-gen-gate: go.mod $(wildcard internal/protoc-gen/*.go)
	$(GO) build -o bin/protoc-gen-gate ./internal/protoc-gen

%.pb.go: %.proto bin/protoc-gen-gate
	mkdir -p tmp
	PATH=$(PWD)/bin:$(PATH) $(PROTOC) --gate_out=tmp $*.proto
	mv tmp/github.com/tsavola/gate/$@ $@

server/event/event.pb.go: server/detail/detail.proto

server/event/type.gen.go: server/event/event.pb.go $(wildcard internal/event-type-gen/*.go)
	[ ! -e $@ ] || (echo "package event" > tmp/empty.go && touch --reference=$@ tmp/empty.go)
	$(GO) run ./internal/event-type-gen/main.go | $(GOFMT) > tmp/$(notdir $@)
	mv tmp/$(notdir $@) $@
	$(GO) build ./server/event || (mv tmp/empty.go $@; false)

clean:
	rm -rf bin lib tmp
	$(MAKE) -C run/container clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C run/loader/tests clean
	$(MAKE) -C crt clean
	$(MAKE) -C rustrt clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(MAKE) -C libcxx clean
	$(MAKE) -C capi clean
	$(MAKE) -C examples/gate-talk/payload clean
	$(MAKE) -C examples/toolchain clean
	$(foreach dir,$(TESTS),$(MAKE) -C $(dir) clean;)

.PHONY: lib generate bin devlibs tests all capabilities check check-toolchain benchmark clean
