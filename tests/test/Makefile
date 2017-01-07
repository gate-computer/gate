GATEDIR		:= ../..

CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -ffreestanding -Oz
CXXFLAGS	+= -std=c++14

OBJECTS		:= test.bc

build: prog.wasm

test.bc: test.cpp $(GATEDIR)/include/gate.h

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
