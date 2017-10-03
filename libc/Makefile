# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -isystem musl/arch/wasm32 -isystem musl/include -DNDEBUG
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -Oz
CXXFLAGS	+= -std=c++11

include $(GATEDIR)/crt/rules.mk

.DEFAULT_GOAL	:= build

build: libc_build/done compat.bc
	$(MAKE) libc-objects

libc_build/done:
	mkdir -p libc_build
	export PYTHONPATH=musl && $(PYTHON) -B musl.py --clang_dir=$(LLVMBINDIR) --binaryen_dir=$(BINARYENBINDIR) --musl=$(realpath musl) --save-temps
	touch $@

libc-objects: $(patsubst %.ll,%.bc,$(wildcard libc_build/*.ll))

libc_build/%.bc: libc_build/%.ll
	$(LLVMAS) -o $@ libc_build/$*.ll

compat.bc: compat.cpp $(GATEDIR)/capi/include/gate.h Makefile $(GATEDIR)/crt/rules.mk

clean:
	rm -f *.bc *.wast
	rm -rf libc_build

.PHONY: build libc-objects clean
