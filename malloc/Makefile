# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= \
	-isystem $(GATEDIR)/libc/musl/arch/wasm32 \
	-isystem $(GATEDIR)/libc/musl/include \
	-include $(GATEDIR)/include/gate.h \
	-include morecore.h \
	-DABORT='gate_exit(1)' \
	-DHAVE_MMAP=0 \
	-DHAVE_MREMAP=0 \
	-DLACKS_TIME_H=1 \
	-DMORECORE=__malloc_morecore \
	-DMORECORE_CANNOT_TRIM=1 \
	-DNDEBUG \
	-DNO_MALLINFO=1 \
	-DNO_MALLOC_STATS=1 \
	-DUSE_LOCKS=0 \
	-Dmalloc_getpagesize=4096

CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -ffreestanding -Oz

OBJECTS		:= malloc.bc morecore.bc wasm.bc

build: all.bc

include $(GATEDIR)/crt/rules.mk

all.bc: $(OBJECTS)
	$(LLVMLINK) -o $@ $(OBJECTS)

malloc.bc: malloc.c morecore.h $(GATEDIR)/include/gate.h Makefile $(GATEDIR)/crt/rules.mk
morecore.bc: morecore.c morecore.h $(GATEDIR)/include/gate.h Makefile $(GATEDIR)/crt/rules.mk

wasm.bc: wasm.ll Makefile $(GATEDIR)/crt/rules.mk
	$(LLVMAS) -o $@ wasm.ll

clean:
	rm -f *.bc

.PHONY: build clean
