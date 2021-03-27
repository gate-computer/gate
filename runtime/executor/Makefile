DESTDIR		:=
PREFIX		:= /usr/local
LIBEXECDIR	:= $(PREFIX)/lib/gate

include ../include/runtime.mk

CPPFLAGS	+= -I../include -DNDEBUG -DGATE_COMPAT_VERSION='"'$(GATE_COMPAT_VERSION)'"'
CFLAGS		+= -std=gnu99 -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static

binary		:= ../../lib/gate/gate-runtime-executor.$(GATE_COMPAT_MAJOR)
objects		:= executor.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)

%.o: %.c $(wildcard ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

install:
	install -m 755 -d $(DESTDIR)$(LIBEXECDIR)
	install -m 755 $(binary) $(DESTDIR)$(LIBEXECDIR)

clean:
	rm -f *.o

.PHONY: install clean
