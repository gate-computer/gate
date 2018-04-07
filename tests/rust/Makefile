# Copyright (c) 2018 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

RUSTC		?= rustc

SOURCES		:= $(wildcard *.rs)

build: prog.wasm

prog.wasm: $(SOURCES) Makefile
	$(RUSTC) --target=wasm32-unknown-unknown -o $@ $(SOURCES)

clean:
	rm -f prog.* *.bc

.PHONY: build clean
