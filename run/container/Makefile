CFLAGS		+= -std=gnu99 -g -Wall -Wextra -Wno-unused-parameter
LIBS		+= -lcap

../../lib/container: container.o Makefile
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ container.o $(LIBS)

container.o: container.c ../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ container.c

clean:
	rm -f *.o

.PHONY: clean
