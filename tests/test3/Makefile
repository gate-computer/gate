GATEDIR		:= ../..

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz

OBJECTS		:= test3.bc

LIBS := \
	$(GATEDIR)/libc/compat.bc \
	$(GATEDIR)/libc/libc_build/memcpy.bc \
	$(GATEDIR)/libc/libc_build/memset.bc \
	$(GATEDIR)/malloc/malloc.bc \
	$(GATEDIR)/malloc/morecore.bc \
	$(GATEDIR)/malloc/wasm.bc

build: prog.wasm

test3.bc: test3.c $(GATEDIR)/include/gate.h Makefile

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
