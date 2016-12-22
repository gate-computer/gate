NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

CPPFLAGS	+= -DNDEBUG -DPIE
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector -Wno-main
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x100000000 -Wl,--build-id=none

ifeq ($(DEBUG),1)
CPPFLAGS	+= -DGATE_DEBUG_PATH='"/tmp/gate-debug.log"'
endif

OBJECTS		:= loader.o runtime.o

../../bin/loader: $(OBJECTS)
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(OBJECTS)
	$(NM) $@ | grep " __gate_" > $@.symbols
	$(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	$(STRIP) $@

loader.o: loader.c ../defs.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

runtime.o: runtime.S ../defs.h
	$(CC) $(CPPFLAGS) -c -o $@ runtime.S

clean:
	rm -f *.o

.PHONY: clean
