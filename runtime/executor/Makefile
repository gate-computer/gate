DESTDIR		:=
PREFIX		:= /usr/local
LIBEXECDIR	:= $(PREFIX)/lib/gate

include ../include/runtime.mk

CPPFLAGS	+= -I../include -DNDEBUG -DGATE_COMPAT_VERSION='"'$(GATE_COMPAT_VERSION)'"'
CFLAGS		+= -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter -Wimplicit-fallthrough
CXXFLAGS	+= -std=c++17 -fno-exceptions -Wno-write-strings
LDFLAGS		+= -static

binary		:= ../../lib/gate/gate-runtime-executor.$(GATE_COMPAT_MAJOR)
objects		:= executor.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -o $@ $(objects)

%.o: %.cpp $(wildcard ../include/*.h) Makefile ../include/runtime.mk
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ $*.cpp

install:
	install -m 755 -d $(DESTDIR)$(LIBEXECDIR)
	install -m 755 $(binary) $(DESTDIR)$(LIBEXECDIR)

clean:
	rm -f *.o

.PHONY: install clean
