PWD		:= $(shell pwd)

GO		?= go
export GOPATH	:= $(PWD)

GOPACKAGEPREFIX	:= github.com/tsavola/gate

-include config.make

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/httpserver \
	$(GOPACKAGEPREFIX)/internal/memfd \
	$(GOPACKAGEPREFIX)/run

export GATE_TEST_EXECUTOR	:= $(PWD)/bin/executor
export GATE_TEST_LOADER		:= $(PWD)/bin/loader
export GATE_TEST_WASM		:= $(PWD)/tests/test/prog.wasm

build:
	$(GO) get github.com/gorilla/websocket
	$(GO) get github.com/tsavola/wag
	$(GO) get golang.org/x/crypto/acme/autocert
	$(GO) get golang.org/x/net/http2
	$(GO) get golang.org/x/net/http2/hpack
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/httpserver

all: build
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C malloc
	$(MAKE) -C tests/test
	$(MAKE) -C tests/test2
	$(MAKE) -C tests/test3
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/client
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/server

check: all
	$(GO) vet $(GOPACKAGES)
	$(GO) get github.com/bnagy/gapstone
	$(GO) test -race -v $(GOPACKAGES)

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C malloc clean
	$(MAKE) -C tests/test clean
	$(MAKE) -C tests/test2 clean
	$(MAKE) -C tests/test3 clean

.PHONY: build all check clean
