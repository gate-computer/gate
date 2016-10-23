CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -std=gnu99 -Os -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+=

../../bin/executor: executor.o
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ executor.o

executor.o: executor.c ../defs.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ executor.c

clean:
	rm -f *.o

.PHONY: clean
