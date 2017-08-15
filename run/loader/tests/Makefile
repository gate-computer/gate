PYTHON		?= python

CPPFLAGS	+= -DPIE
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector
LDFLAGS		+= -static

build: signal stack

check: signal stack
	./signal
	$(PYTHON) ./stack.py

signal: signal.o ../runtime.o
	$(CC) $(CFLAGS) $(LDFLAGS) -Wl,-Ttext-segment=0x40000000 -Wl,--section-start=.runtime=0x50000000 -o $@ signal.o ../runtime.o

signal.o: signal.c ../../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ signal.c

stack: stack.o ../start.o
	$(CC) $(CFLAGS) $(LDFLAGS) -nostartfiles -o $@ ../start.o stack.o

stack.o: stack.c Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ stack.c

clean:
	rm -f signal stack *.o

.PHONY: build check clean
