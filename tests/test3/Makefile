GATEDIR		:= ../..

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -Os

OBJECTS		:= test3.bc

LIBS := \
	$(GATEDIR)/libc/libc_build/__errno_location.bc \
	$(GATEDIR)/libc/libc_build/calloc.bc \
	$(GATEDIR)/libc/libc_build/expand_heap.bc \
	$(GATEDIR)/libc/libc_build/libc.bc \
	$(GATEDIR)/libc/libc_build/malloc.bc \
	$(GATEDIR)/libc/libc_build/memcpy.bc \
	$(GATEDIR)/libc/libc_build/memset.bc \
	$(GATEDIR)/libc/compat.bc \
	$(GATEDIR)/libc/wasm.bc

build: prog.wasm

test3.bc: test3.c $(GATEDIR)/include/gate.h Makefile

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.make
