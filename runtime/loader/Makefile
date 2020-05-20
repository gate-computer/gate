NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

CC_S		:= $(CC)

ARCH		:= $(shell $(CC) -dumpmachine | cut -d- -f1)

CPPFLAGS	+= -I$(ARCH) -I../include/$(ARCH) -I../include -DNDEBUG -DPIE -DGATE_LOADER_ADDR=0x200000000
CFLAGS		+= -std=gnu99 -Os -fPIE -fomit-frame-pointer -fno-stack-protector -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x200000000 -Wl,--build-id=none

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-loader.$(GATE_RUNTIME_BINARY_SUFFIX)
objects		:= $(ARCH)/start.o loader.o $(ARCH)/runtime.o

$(binary): $(objects)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)
ifeq ($(GATE_SANDBOX),1)
	$(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	$(STRIP) $@
endif

loader.o: loader.c $(wildcard $(ARCH)/*.h ../include/$(ARCH)/*.h ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

$(ARCH)/start.o: $(ARCH)/start.S Makefile ../include/runtime.mk
$(ARCH)/runtime.o: $(ARCH)/runtime.S poll.S seccomp.S $(wildcard ../include/$(ARCH)/*.h ../include/*.h) Makefile ../include/runtime.mk

%.o: %.S
	$(CC_S) $(CPPFLAGS) -c -o $@ $*.S

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

clean:
	rm -f *.o $(ARCH)/*.o

.PHONY: install clean
