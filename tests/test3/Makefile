GATEDIR		:= ../..

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -Os

OBJECTS		:= test3.bc

LIBS := \
	$(wildcard $(GATEDIR)/libc/libc_build/*.bc) \
	$(GATEDIR)/libc/compat.bc \
	$(GATEDIR)/libc/wasm.bc

build: prog.wasm

test3.bc: test3.c $(GATEDIR)/include/gate.h

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.make
