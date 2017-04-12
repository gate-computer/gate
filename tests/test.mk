export WAGTOOLCHAIN_ALLOCATE_STACK := 1048576

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -ffreestanding -Oz
CXXFLAGS	+= -std=c++14

SOURCE		?= $(firstword $(wildcard *.c *.cpp))
OBJECT		?= $(patsubst %.cpp,%.bc,$(patsubst %.c,%.bc,$(SOURCE)))

OBJECTS		:= $(OBJECT)

build: prog.wasm

$(OBJECT): $(SOURCE) $(GATEDIR)/include/gate.h Makefile $(GATEDIR)/tests/test.mk $(GATEDIR)/crt/rules.mk

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
