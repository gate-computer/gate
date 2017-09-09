# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

UMASK		?= 0022

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -std=gnu99 -O2 -fomit-frame-pointer -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -static

../../lib/executor: executor.o
	umask $(UMASK) && mkdir -p $(dir $@)
	umask $(UMASK) && $(CC) $(CFLAGS) $(LDFLAGS) -o $@ executor.o

executor.o: executor.c buffer.h ../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ executor.c

clean:
	rm -f *.o

.PHONY: clean
