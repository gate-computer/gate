GATEDIR		:= ../..

export WAGTOOLCHAIN_ALLOCATE_STACK := 131072

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz

ifeq ($(DEBUG),1)
CPPFLAGS	+= -DGATE_DEBUG
endif

OBJECTS		:= nonblock.bc

build: prog.wasm

nonblock.bc: nonblock.c $(GATEDIR)/include/gate.h Makefile

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.make
