CPPFLAGS	+= -DNDEBUG -DPIE
CFLAGS		+= -Os -g -Wall -Wextra -Wno-unused-parameter -fPIE -fomit-frame-pointer -fno-stack-protector -Wno-main
LDFLAGS		+= -static -nostartfiles -nostdlib -Wl,--section-start=.abi=0xfffff000 -Wl,-Ttext=0x100000000 -Wl,-Tbss=0x100001000 -Wl,--build-id=none

../../bin/loader: loader.o abi.o
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ loader.o abi.o

loader.o: loader.c abi.h ../stack.h ../../include/gate/args.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ loader.c

abi.o: abi.S
	$(CC) $(CPPFLAGS) -c -o $@ abi.S

clean:
	rm -f *.o

.PHONY: clean
