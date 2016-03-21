CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -Os -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+=

build: executor

executor: executor.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ executor.o

executor.o: executor.c ../stack.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ executor.c

clean:
	rm -f executor *.o

.PHONY: build clean
