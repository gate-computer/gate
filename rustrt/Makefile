# Copyright (c) 2018 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

RUSTC		?= rustc

GATEDIR		:= ..

build: rustrt.bc

include $(GATEDIR)/crt/rules.mk

rustrt.bc: rustrt.rs Makefile $(GATEDIR)/crt/rules.mk
	$(RUSTC) --target=wasm32-unknown-unknown --emit=llvm-bc -o $@ rustrt.rs

clean:
	rm -f *.bc

.PHONY: build clean
