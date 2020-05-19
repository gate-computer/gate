PYTHON		?= python3

ARCH		:= $(shell $(CC) -dumpmachine | cut -d- -f1)

CPPFLAGS	+= -I../../include/$(ARCH) -I../../include -DPIE
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector
LDFLAGS		+= -static

include ../../include/runtime.mk

all: signal stack

check: signal.check stack.check

signal: signal.o ../$(ARCH)/runtime.o
	$(CC) $(CFLAGS) $(LDFLAGS) -Wl,-Ttext-segment=0x40000000 -Wl,--section-start=.runtime=0x50000000 -o $@ signal.o ../$(ARCH)/runtime.o

signal.o: signal.c Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ signal.c

stack: stack.o ../$(ARCH)/start.o
	$(CC) $(CFLAGS) $(LDFLAGS) -nostartfiles -nostdlib -o $@ ../$(ARCH)/start.o stack.o

stack-debug: stack.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ stack.o

stack.o: stack.c $(wildcard ../../include/$(ARCH)/*.h) Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ stack.c

signal.check: signal Makefile
	./signal
	touch $@

stack.check: stack.py stack Makefile
	$(PYTHON) stack.py
	touch $@

clean:
	rm -f signal stack *.o *.check

.PHONY: all check clean
