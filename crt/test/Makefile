GATEDIR		:= ../..

include ../variables.make

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter
CXXFLAGS	+= -std=c++14
LDFLAGS		+= -T$(GATEDIR)/assemble/link.ld

PASS_PLUGIN	:= $(GATEDIR)/llvmpass/build/libgatepass.so

OBJECTS		:= test.bc

build: prog.bc prog.elf

prog.elf: prog.o
	$(LD) $(LDFLAGS) -o $@ prog.o

prog.o: prog.bc
	$(OPT) -load=$(PASS_PLUGIN) -gate < prog.bc | $(LLC) | $(AS) -o $@

test.bc: test.cpp $(INCLUDEDIR)/gate.h $(INCLUDEDIR)/gate/args.h

clean:
	rm -f prog.* *.bc

.PHONY: build clean

include ../rules.make
