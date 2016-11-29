export WAGTOOLCHAIN_ALLOCATE_STACK := 1048576

GATEDIR		:= ../..

CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -ffreestanding -Oz
CXXFLAGS	+= -std=c++14

OBJECTS		:= test2.bc

build: prog.wasm

test2.bc: test2.cpp $(GATEDIR)/include/gate.h

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.make
