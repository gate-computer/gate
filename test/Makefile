GATEDIR		:= ..

CFLAGS		+= -g -Wall -Wextra -Wno-unused-parameter
CXXFLAGS	+= -std=c++14

OBJECTS		:= test.bc

build: prog.wasm

test.bc: test.cpp $(GATEDIR)/include/gate.h

clean:
	rm -f prog.* *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.make
