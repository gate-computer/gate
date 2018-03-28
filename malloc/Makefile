# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= \
	-include morecore.h \
	-DHAVE_MMAP=0 \
	-DHAVE_MREMAP=0 \
	-DLACKS_TIME_H=1 \
	-DMORECORE=__malloc_morecore \
	-DMORECORE_CANNOT_TRIM=1 \
	-DNDEBUG \
	-DNO_MALLINFO=1 \
	-DNO_MALLOC_STATS=1 \
	-DUSE_LOCKS=0 \
	-Dmalloc_getpagesize=65536

OBJECTS		:= malloc.bc wasm.bc

build: all.bc

include $(GATEDIR)/crt/rules.mk

CFLAGS		+= -fno-builtin-calloc -fno-builtin-free -fno-builtin-malloc -fno-builtin-memalign -fno-builtin-posix_memalign -fno-builtin-pvalloc -fno-builtin-realloc -fno-builtin-valloc -Wno-bitwise-op-parentheses -Wno-null-pointer-arithmetic -Wno-shift-op-parentheses

all.bc: $(OBJECTS)
	$(LLVMLINK) -o $@ $(OBJECTS)

malloc.bc: malloc.c morecore.h Makefile $(GATEDIR)/crt/rules.mk

wasm.bc: wasm.ll Makefile $(GATEDIR)/crt/rules.mk
	$(LLVMAS) -o $@ wasm.ll

clean:
	rm -f *.bc

.PHONY: build clean
