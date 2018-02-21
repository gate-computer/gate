# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -isystem musl/arch/wasm32 -isystem musl/include -isystem include -DNDEBUG

LIBC_SOURCES	:= $(shell find musl/src -maxdepth 2 -name '*.c' \
	-not -name vfwscanf.c \
	-not -name wcstod.c \
	-not -name wcstol.c \
)

LIBC_OBJECTS	:= $(LIBC_SOURCES:musl/%.c=libc_build/obj/%.bc)

include $(GATEDIR)/crt/rules.mk

.DEFAULT_GOAL	:= build

build: libc_build/done compat.bc

libc_build/done: $(LIBC_OBJECTS)
	@ set -e; for obj in $(LIBC_OBJECTS:libc_build/%=%); do ln -sf $$obj libc_build/; done
	touch libc_build/done

libc_build/obj/%.bc: musl/%.c libc_build/config.mak
	$(MAKE) -C libc_build obj/$*.o
	mv libc_build/obj/$*.o libc_build/obj/$*.bc

libc_build/config.mak:
	mkdir -p libc_build
	cd libc_build && ../musl/configure CC="$(CC) --target=wasm32-unknown-unknown -emit-llvm"

compat.bc: compat.c $(GATEDIR)/capi/include/gate.h Makefile $(GATEDIR)/crt/rules.mk

clean:
	rm -f *.bc
	rm -rf libc_build

.PHONY: build clean
