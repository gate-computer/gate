NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

CPPFLAGS	+= -DNDEBUG -DPIE
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector -Wno-main
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext=0x100000000 -Wl,--build-id=none

../../bin/loader: loader.o abi.o
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ loader.o abi.o
	$(NM) $@ | grep " __gate_" > $@.symbols
	$(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	$(STRIP) $@

loader.o: loader.c ../defs.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

abi.o: abi.S
	$(CC) $(CPPFLAGS) -c -o $@ abi.S

clean:
	rm -f *.o

.PHONY: clean
