GDB		?= gdb

GATEDIR		:= ../..

include ../variables.make

CFLAGS		+= -g -Wall -Wextra -Wno-unused-parameter
CXXFLAGS	+= -std=c++14

OBJECTS		:= test.bc

build: prog.bc prog.elf prog.payload

test.bc: test.cpp $(GATEDIR)/include/gate.h $(GATEDIR)/include/gate/args.h

check: prog.payload
	$(GATEDIR)/run/executor/executor $(GATEDIR)/run/loader/loader < prog.payload
	$(GDB) -q -ex "add-symbol-file prog.elf 0x400000" -ex "run < prog.payload" --batch $(GATEDIR)/run/loader/loader

clean:
	rm -f prog.* *.bc

.PHONY: build check clean

include ../rules.make
