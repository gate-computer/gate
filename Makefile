GO		?= go
GOFMT		?= gofmt
PROTOC		?= protoc

CGROUP_BACKEND	?= systemd

DESTDIR		:=
PREFIX		:= /usr/local
BINDIR		:= $(PREFIX)/bin
LIBDIR		:= $(PREFIX)/lib/gate

GEN_LIB_SOURCES := \
	runtime/include/errors.h

GEN_BIN_SOURCES := \
	internal/error/runtime/errors.go \
	internal/serverapi/serverapi.pb.go \
	server/detail/detail.pb.go \
	server/event/event.pb.go \
	server/event/type.gen.go \
	server/monitor/monitor.pb.go

-include config.mk

export GO111MODULE := on

lib: $(GEN_LIB_SOURCES)
	$(MAKE) -C runtime/container CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) -C runtime/executor
	$(MAKE) -C runtime/loader

bin: $(GEN_BIN_SOURCES)
	$(GO) build $(GOBUILDFLAGS) -o bin/gate-run ./cmd/gate-run
	$(GO) build $(GOBUILDFLAGS) -o bin/gate-runtimed ./cmd/gate-runtimed
	$(GO) build $(GOBUILDFLAGS) -o bin/gate-server ./cmd/gate-server

generate: $(GEN_LIB_SOURCES) $(GEN_BIN_SOURCES)

all: lib bin

check: lib bin
	$(MAKE) -C runtime/loader/test check
	$(GO) build $(GOBUILDFLAGS) -buildmode=plugin -o lib/gate/service/internal/test.so ./internal/test/service-plugin
	$(GO) vet ./...
	$(GO) test $(GOTESTFLAGS) ./...

benchmark: lib bin
	$(GO) test -run=^$$ -bench=.* -v ./... | tee bench-new.txt
	[ ! -e bench-old.txt ] || benchcmp bench-old.txt bench-new.txt

install-lib:
	install -m 755 -d $(DESTDIR)$(LIBDIR)/runtime
	$(MAKE) LIBDIR=$(DESTDIR)$(LIBDIR) -C runtime/container install CGROUP_BACKEND=$(CGROUP_BACKEND)
	$(MAKE) LIBDIR=$(DESTDIR)$(LIBDIR) -C runtime/executor install
	$(MAKE) LIBDIR=$(DESTDIR)$(LIBDIR) -C runtime/loader install

install-lib-capabilities: install-lib
	$(MAKE) LIBDIR=$(DESTDIR)$(LIBDIR) -C runtime/container capabilities

install-bin:
	install -m 755 -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/gate-runtimed bin/gate-run bin/gate-server $(DESTDIR)$(BINDIR)

install: install-lib install-bin
install-capabilities: install-lib-capabilities install-bin

internal/error/runtime/errors.go runtime/include/errors.h: internal/cmd/runtime-errors/generate.go $(wildcard runtime/*/*.c runtime/*/*.S)
	mkdir -p tmp
	$(GO) run internal/cmd/runtime-errors/generate.go $(wildcard runtime/*/*.c runtime/*/*.S) | $(GOFMT) > tmp/errors.go
	test -s tmp/errors.go
	mv tmp/errors.go internal/error/runtime/errors.go

bin/protoc-gen-gate: go.mod internal/cmd/protoc/generate.go
	$(GO) build -o $@ ./internal/cmd/protoc

%.pb.go: %.proto bin/protoc-gen-gate
	mkdir -p tmp
	PATH=$(shell pwd)/bin:$(PATH) $(PROTOC) --gate_out=tmp $*.proto
	mv tmp/github.com/tsavola/gate/$@ $@

server/event/event.pb.go: server/detail/detail.proto

server/event/type.gen.go: server/event/event.pb.go internal/cmd/event-types/generate.go
	[ ! -e $@ ] || (echo "package event" > tmp/empty.go && touch --reference=$@ tmp/empty.go)
	$(GO) run ./internal/cmd/event-types/generate.go | $(GOFMT) > tmp/$(notdir $@)
	mv tmp/$(notdir $@) $@
	$(GO) build ./server/event || (mv tmp/empty.go $@; false)

clean:
	rm -rf bin lib tmp
	$(MAKE) -C runtime/container clean
	$(MAKE) -C runtime/executor clean
	$(MAKE) -C runtime/loader clean
	$(MAKE) -C runtime/loader/test clean

.PHONY: lib bin generate all check benchmark install-lib install-lib-capabilities install-bin install install-capabilities clean
