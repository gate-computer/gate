# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

NM		?= nm
OBJCOPY		?= objcopy
STRIP		?= strip

UMASK		?= 0022

CPPFLAGS	+= -DNDEBUG -DPIE -DGATE_LOADER_TEXT_ADDR=0x200000000 -DGATE_LOADER_TEXT_SIZE=0x1000
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,-z,noexecstack -Wl,-Ttext-segment=0x200000000 -Wl,--section-start=.runtime=0x200001000 -Wl,--build-id=none

OBJECTS		:= start.o loader.o runtime.o

../../lib/loader: $(OBJECTS)
	umask $(UMASK) && mkdir -p $(dir $@)
	umask $(UMASK) && $(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(OBJECTS)
	umask $(UMASK) && $(NM) $@ | grep " __gate_" > $(dir $@)runtime.map
	umask $(UMASK) && $(OBJCOPY) --remove-section=.comment --remove-section=.eh_frame $@
	umask $(UMASK) && $(STRIP) $@

loader.o: loader.c ../defs.h ../fdpath.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

start.o: start.S Makefile
runtime.o: runtime.S ../defs.h Makefile

%.o: %.S
	$(CC) $(CPPFLAGS) -c -o $@ $*.S

clean:
	rm -f *.o

.PHONY: clean
