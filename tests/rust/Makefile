# Copyright (c) 2018 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

RUSTC		?= rustc

LLVMPREFIX	?= $(GATEDIR)/../wag-toolchain/out
CC		:= $(LLVMPREFIX)/bin/clang --target=wasm32-unknown-unknown

GATEDIR		:= ../..

SOURCE		:= $(firstword $(wildcard *.rs))
OBJECT		:= $(SOURCE:%.rs=%.bc)

LDFLAGS		:= -nostdlib -Wl,--allow-undefined-file=$(GATEDIR)/capi/abi.list -Wl,--check-signatures

LIBS := \
	$(GATEDIR)/rustrt/rustrt.bc

build: prog.wasm

prog.wasm: $(OBJECT)
	$(CC) $(LDFLAGS) -o $@ $(OBJECT) $(LIBS)

$(OBJECT): $(SOURCE) Makefile
	$(RUSTC) --target=wasm32-unknown-unknown --emit=llvm-bc -o $@ $(SOURCE)

clean:
	rm -f prog.* *.bc

.PHONY: build clean
