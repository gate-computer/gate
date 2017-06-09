NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

CPPFLAGS	+= -DNDEBUG -DPIE -DGATE_LOADER_TEXT_ADDR=0x200000000 -DGATE_LOADER_TEXT_SIZE=0x1000
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector -Wno-main
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x200000000 -Wl,--section-start=.runtime=0x200001000 -Wl,--build-id=none

OBJECTS		:= loader.o runtime.o

../../bin/loader: $(OBJECTS) Makefile
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(OBJECTS)
	$(NM) $@ | grep " __gate_" > $@.symbols
	$(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	$(STRIP) $@

loader.o: loader.c ../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

runtime.o: runtime.S ../defs.h Makefile
	$(CC) $(CPPFLAGS) -c -o $@ runtime.S

clean:
	rm -f *.o

.PHONY: clean
