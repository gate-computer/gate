CPPFLAGS	+= -pthread -I../include -DNDEBUG
CFLAGS		+= -std=gnu99 -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -pthread -static

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-executor.$(GATE_INTERNAL_API_VERSION)
objects		:= executor.o reaper.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)

%.o: %.c $(wildcard *.h ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

clean:
	rm -f *.o

.PHONY: install clean
