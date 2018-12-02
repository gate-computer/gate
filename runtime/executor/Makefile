CPPFLAGS	+= -I../include -DNDEBUG
CFLAGS		+= -std=gnu99 -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-executor.$(GATE_INTERNAL_API_VERSION)

$(binary): executor.o
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ executor.o

executor.o: executor.c buffer.h $(wildcard *.h ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ executor.c

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

clean:
	rm -f *.o

.PHONY: install clean
