NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

CPPFLAGS	+= -I../include -DNDEBUG -DPIE -DGATE_LOADER_ADDR=0x200000000 -DGATE_LOADER_SIZE=0x1000
CFLAGS		+= -std=gnu99 -Os -fPIE -fomit-frame-pointer -fno-stack-protector -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x200000000 -Wl,--build-id=none

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-loader.$(GATE_INTERNAL_API_VERSION)
objects		:= start.o loader.o runtime.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)
ifeq ($(GATE_SANDBOX),1)
	$(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	$(STRIP) $@
endif

loader.o: loader.c $(wildcard *.h ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

start.o: start.S Makefile ../include/runtime.mk
runtime.o: runtime.S $(wildcard *.h ../include/*.h) Makefile ../include/runtime.mk

%.o: %.S
	$(CC) $(CPPFLAGS) -c -o $@ $*.S

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

clean:
	rm -f *.o

.PHONY: install clean
