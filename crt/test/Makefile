CRTDIR		:= ..

include $(CRTDIR)/variables.make

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter
CXXFLAGS	+= -std=c++14
LDFLAGS		+= -Tlink.ld

PASSPLUGIN	:= $(CRTDIR)/../llvmpass/build/libgatepass.so

OBJECTS		:= test.bc

build: prog.bc prog.elf

prog.elf: prog.o
	$(LD) $(LDFLAGS) -o $@ prog.o

prog.o: prog.bc
	$(LLVMOPT) -load=$(PASSPLUGIN) -gate < prog.bc | $(LLC) | $(AS) -o $@

test.bc: test.cpp $(INCLUDEDIR)/gate.h $(INCLUDEDIR)/gate/args.h

clean:
	rm -f prog.* *.bc

.PHONY: build clean

include $(CRTDIR)/rules.make
