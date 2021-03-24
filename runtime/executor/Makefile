ifeq (${origin CC},default)
CC		:= musl-gcc
endif

CPPFLAGS	+= -I../include -DNDEBUG
CFLAGS		+= -std=gnu99 -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-executor-$(GATE_RUNTIME_BINARY_SUFFIX)
objects		:= executor.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)

%.o: %.c $(wildcard ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

clean:
	rm -f *.o

.PHONY: install clean
