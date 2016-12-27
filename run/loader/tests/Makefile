CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+= -Wl,-Ttext-segment=0x40000000 -Wl,--section-start=.runtime=0x50000000

build: signal

check: signal
	./signal

signal: signal.o ../runtime.o Makefile
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ signal.o ../runtime.o

signal.o: signal.c ../../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ signal.c

clean:
	rm -f signal *.o

.PHONY: build check clean
