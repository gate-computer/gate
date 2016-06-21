PWD		:= $(shell pwd)

GO		?= go
export GOPATH	:= $(PWD)

include llvm.make

GOPACKAGEPREFIX	:= github.com/tsavola/gate

-include config.make

GOPACKAGES := \
	$(GOPACKAGEPREFIX)/assemble \
	$(GOPACKAGEPREFIX)/client \
	$(GOPACKAGEPREFIX)/run \
	$(GOPACKAGEPREFIX)/server \
	$(GOPACKAGEPREFIX)/stream \
	$(GOPACKAGEPREFIX)/stream/tlsconfig

export GATE_TEST_OPT		:= $(OPT)
export GATE_TEST_LLC		:= $(LLC)
export GATE_TEST_AS		:= $(AS)
export GATE_TEST_LD		:= $(LD)
export GATE_TEST_LINK_SCRIPT	:= $(PWD)/assemble/link.ld
export GATE_TEST_PASS_PLUGIN	:= $(PWD)/lib/libgatepass.so
export GATE_TEST_EXECUTOR	:= $(PWD)/bin/executor
export GATE_TEST_LOADER		:= $(PWD)/bin/loader
export GATE_TEST_BITCODE	:= $(PWD)/test/prog.bc
export GATE_TEST_ELF		:= $(PWD)/test/prog.elf

build:
	$(GO) get golang.org/x/net/http2
	$(GO) get golang.org/x/net/http2/hpack
	$(GO) vet $(GOPACKAGES)
	$(MAKE) -C llvmpass
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(GO) install $(GOPACKAGEPREFIX)/client
	$(GO) install $(GOPACKAGEPREFIX)/server

all: build
	$(GO) install $(GOPACKAGEPREFIX)/elf2payload
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C test

check: all
	$(MAKE) -C test check
	$(GO) test -race -v $(GOPACKAGES)

clean:
	rm -rf bin lib pkg
	$(MAKE) -C llvmpass clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C test clean

.PHONY: build all check clean
