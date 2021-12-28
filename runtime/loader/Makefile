CCAS		:= $(CC) # Assembler but invoked like cc.

DESTDIR		:=
PREFIX		:= /usr/local
LIBEXECDIR	:= $(PREFIX)/lib/gate

include ../include/runtime.mk

ARCH		:= $(shell $(CC) -dumpmachine | cut -d- -f1)

CPPFLAGS	+= -I$(ARCH) -I../include/$(ARCH) -I../include -DNDEBUG -DPIE -DGATE_LOADER_ADDR=0x200000000
CFLAGS		+= -Os -fPIE -fomit-frame-pointer -fno-stack-protector -g -Wall -Wextra -Wno-unused-parameter -Wimplicit-fallthrough
CXXFLAGS	+= -std=c++17 -fno-exceptions
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x200000000 -Wl,--build-id=none

binary		:= ../../lib/gate/gate-runtime-loader.$(GATE_COMPAT_VERSION)
objects		:= $(ARCH)/start.o loader.o $(ARCH)/runtime2.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -o $@ $(objects)

loader.o: loader.cpp $(wildcard $(ARCH)/*.hpp ../include/$(ARCH)/*.hpp ../include/*.hpp) Makefile ../include/runtime.mk
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ loader.cpp

$(ARCH)/start.o: $(ARCH)/start.S Makefile ../include/runtime.mk
$(ARCH)/runtime2.o: $(ARCH)/runtime2.S $(ARCH)/runtime.S poll.S seccomp.S $(wildcard ../include/$(ARCH)/*.hpp ../include/*.hpp) Makefile ../include/runtime.mk

%.o: %.S
	$(CCAS) $(CPPFLAGS) -c -o $@ $*.S

install:
	install -m 755 -d $(DESTDIR)$(LIBEXECDIR)
	install -m 755 $(binary) $(DESTDIR)$(LIBEXECDIR)

clean:
	rm -f *.o $(ARCH)/*.o

.PHONY: install clean
