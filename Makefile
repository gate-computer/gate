PWD		:= $(shell pwd)

GO		?= go
export GOPATH	:= $(PWD)

GOPACKAGEPREFIX	:= github.com/tsavola/gate

-include config.make

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/client \
	$(GOPACKAGEPREFIX)/run \
	$(GOPACKAGEPREFIX)/server \
	$(GOPACKAGEPREFIX)/stream \
	$(GOPACKAGEPREFIX)/stream/tlsconfig

export GATE_TEST_EXECUTOR	:= $(PWD)/bin/executor
export GATE_TEST_LOADER		:= $(PWD)/bin/loader
export GATE_TEST_WASM		:= $(PWD)/test/prog.wasm

build:
	$(GO) get github.com/tsavola/wag
	$(GO) get golang.org/x/net/http2
	$(GO) get golang.org/x/net/http2/hpack
	$(GO) vet $(GOPACKAGES)
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/client
	$(GO) install $(GOBUILDFLAGS) $(GOPACKAGEPREFIX)/server

all: build
	$(MAKE) -C crt
	$(MAKE) -C test

check: all
	$(GO) test -race -v $(GOPACKAGES)

clean:
	rm -rf bin lib pkg
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C crt clean
	$(MAKE) -C test clean

.PHONY: build all check clean
