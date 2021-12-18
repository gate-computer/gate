GO		?= go
GOFMT		?= gofmt
PROTOC		?= protoc
PERFLOCK	?= perflock

DESTDIR		:=
PREFIX		:= /usr/local
BINDIR		= $(PREFIX)/bin
LIBEXECDIR	= $(PREFIX)/lib/gate
libexecprefix	= $(shell echo /$(LIBEXECDIR)/ | tr -s /)

GEN_LIB_SOURCES := \
	runtime/include/errors.h \
	runtime/loader/aarch64/runtime.S \
	runtime/loader/x86_64/runtime.S

GEN_BIN_SOURCES := \
	internal/error/runtime/errors.go \
	internal/manifest/manifest.pb.go \
	internal/webserverapi/webserverapi.pb.go \
	server/api/server.pb.go \
	server/detail/detail.pb.go \
	server/event/event.pb.go \
	server/event/type.gen.go \
	service/grpc/api/service.pb.go \
	service/grpc/api/service_grpc.pb.go

TAGS		:= gateexecdir

BUILDFLAGS	= -tags="$(TAGS)" -ldflags="$(BUILDLDFLAGS)"
BUILDLDFLAGS	= -X gate.computer/gate/runtime/container.ExecDir=$(LIBEXECDIR)

TEST		:= .
TESTFLAGS	= -tags="$(TAGS)" -ldflags="$(TESTLDFLAGS)" -run="$(TEST)" -count=1 -race
TESTLDFLAGS	= -X gate.computer/wag/internal.Panic=1
ifneq ($(TEST),)
TESTFLAGS	+= -v
endif

BENCH		:= .
BENCHFLAGS	= -tags="$(TAGS)" -bench="$(BENCH)"

-include config.mk
include runtime/include/runtime.mk

.PHONY: lib
lib: $(GEN_LIB_SOURCES)
	$(MAKE) -C runtime/executor
	$(MAKE) -C runtime/loader

.PHONY: bin
bin: $(GEN_BIN_SOURCES)
	$(GO) build $(BUILDFLAGS) -o bin/gate ./cmd/gate
	$(GO) build $(BUILDFLAGS) -o bin/gate-daemon ./cmd/gate-daemon
	$(GO) build $(BUILDFLAGS) -o bin/gate-runtime ./cmd/gate-runtime
	$(GO) build $(BUILDFLAGS) -o bin/gate-server ./cmd/gate-server

.PHONY: gen
gen: $(GEN_LIB_SOURCES) $(GEN_BIN_SOURCES)

.PHONY: all
all: lib bin
	$(MAKE) -C runtime/loader/test
	$(GO) build $(BUILDFLAGS) -o tmp/bin/test-grpc-service ./internal/test/grpc-service

.PHONY: check
check: all
	$(GO) build -o /dev/null ./cmd/gate-librarian
	$(GO) build -o /dev/null ./cmd/gate-resource
	$(MAKE) -C runtime/loader/test check
	GOARCH=amd64 $(GO) build -o /dev/null ./...
	GOARCH=arm64 $(GO) build -o /dev/null ./...
	GOOS=darwin $(GO) build -o /dev/null ./cmd/gate
	GOOS=windows $(GO) build -o /dev/null ./cmd/gate
	$(GO) vet ./...
	$(GO) test $(TESTFLAGS) ./...

.PHONY: benchmark
benchmark: lib bin
	@ $(PERFLOCK) true
	$(PERFLOCK) $(GO) test -run=- $(BENCHFLAGS) ./... | tee bench-new.txt
	[ ! -e bench-old.txt ] || benchstat bench-old.txt bench-new.txt

.PHONY: install-lib
install-lib:
	$(MAKE) DESTDIR=$(DESTDIR) LIBEXECDIR=$(LIBEXECDIR) -C runtime/executor install
	$(MAKE) DESTDIR=$(DESTDIR) LIBEXECDIR=$(LIBEXECDIR) -C runtime/loader install

.PHONY: install-bin
install-bin:
	install -m 755 -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/gate bin/gate-daemon bin/gate-runtime bin/gate-server $(DESTDIR)$(BINDIR)

.PHONY: install
install:
	[ ! -e lib/gate/gate-runtime-loader.$(GATE_COMPAT_VERSION) ] || $(MAKE) install-lib
	[ ! -e bin/gate ] || $(MAKE) install-bin

.PHONY: install-bash
install-bash:
	install -m 755 -d $(DESTDIR)/etc/bash_completion.d
	install -m 644 etc/bash_completion.d/gate.bash $(DESTDIR)/etc/bash_completion.d/gate

.PHONY: install-systemd
install-systemd: install-systemd-user

.PHONY: install-systemd-user
install-systemd-user:
	install -m 755 -d $(PREFIX)/share/dbus-1/services $(PREFIX)/share/systemd/user
	sed "s,/usr/local/bin/,$(BINDIR)/,g" etc/systemd/user/gate.service > $(PREFIX)/share/systemd/user/gate.service
	sed "s,/usr/local/bin/,$(BINDIR)/,g" etc/dbus/services/computer.gate.Daemon.service > $(PREFIX)/share/dbus-1/services/computer.gate.Daemon.service

internal/error/runtime/errors.go runtime/include/errors.h: internal/cmd/runtime-errors/generate.go $(wildcard runtime/*/*.c runtime/*/*/*.S internal/container/child/*.go internal/cmd/runtime-assembly/*.go)
	mkdir -p tmp
	$(GO) run internal/cmd/runtime-errors/generate.go -- tmp/errors.go $(wildcard runtime/*/*.c runtime/*/*/*.h runtime/*/*.S runtime/*/*/*.S internal/container/child/*.go internal/cmd/runtime-assembly/*.go)
	$(GOFMT) -w tmp/errors.go
	test -s tmp/errors.go
	mv tmp/errors.go internal/error/runtime/errors.go

runtime/loader/aarch64/runtime.S runtime/loader/x86_64/runtime.S: internal/cmd/runtime-assembly/generate.go
	$(GO) run internal/cmd/runtime-assembly/generate.go

%.pb.go: %.proto go.mod
	$(GO) build -o tmp/bin/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
	PATH=$(shell pwd)/tmp/bin:$(PATH) $(PROTOC) --go_out=. --go_opt=paths=source_relative $*.proto

%_grpc.pb.go: %.proto go.mod
	$(GO) build -o tmp/bin/protoc-gen-go-grpc google.golang.org/grpc/cmd/protoc-gen-go-grpc
	PATH=$(shell pwd)/tmp/bin:$(PATH) $(PROTOC) --go-grpc_out=. --go-grpc_opt=paths=source_relative $*.proto

server/event/event.pb.go: server/detail/detail.proto

server/event/type.gen.go: server/event/event.pb.go internal/cmd/event-types/generate.go
	mkdir -p tmp
	[ ! -e $@ ] || (echo "package event" > tmp/empty.go && touch --reference=$@ tmp/empty.go)
	$(GO) run ./internal/cmd/event-types/generate.go -- tmp/$(notdir $@)
	$(GOFMT) -w tmp/$(notdir $@)
	mv tmp/$(notdir $@) $@
	$(GO) build ./server/event || (mv tmp/empty.go $@; false)

.PHONY: clean
clean:
	rm -rf bin lib tmp
	$(MAKE) -C runtime/executor clean
	$(MAKE) -C runtime/loader clean
	$(MAKE) -C runtime/loader/test clean
