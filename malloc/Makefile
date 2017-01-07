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

build: malloc.bc morecore.bc wasm.bc

include $(GATEDIR)/crt/rules.mk

LLVMBINDIR	:= $(WAGTOOLCHAIN)/llvm-build/bin

malloc.bc: malloc.c morecore.h $(GATEDIR)/include/gate.h Makefile
morecore.bc: morecore.c morecore.h $(GATEDIR)/include/gate.h Makefile

wasm.bc: wasm.ll Makefile
	$(LLVMBINDIR)/llvm-as -o $@ wasm.ll

clean:
	rm -f *.bc

.PHONY: build clean
