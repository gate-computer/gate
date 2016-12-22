CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter

build: signal

check: signal
	./signal

signal: signal.o ../runtime.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ signal.o ../runtime.o

signal.o: signal.c ../../defs.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ signal.c

clean:
	rm -f signal *.o

.PHONY: build check clean
