GO		?= go

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -Os -g -Wall -Wextra -Wno-unused-parameter
LDFLAGS		+=

LOADER_CPPFLAGS	+= $(CPPFLAGS) -DPIE -I../include
LOADER_CFLAGS	+= $(CFLAGS) -fPIE -fomit-frame-pointer -fno-stack-protector -Wno-main
LOADER_LDFLAGS	+= $(LDFLAGS) -static -nostartfiles -nostdlib -Wl,--section-start=.abi=0xfffff000 -Wl,-Ttext=0x100000000 -Wl,-Tbss=0x100001000 -Wl,--build-id=none

PWD		:= $(shell pwd)
TEST_ELF	:= $(PWD)/../crt/test/prog.elf

build: executor loader

executor: executor.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ executor.o

executor.o: executor.c stack.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ executor.c

loader: loader.o abi.o
	$(CC) $(LOADER_CFLAGS) $(LOADER_LDFLAGS) -o $@ loader.o abi.o

loader.o: loader.c abi.h stack.h ../include/gate/args.h
	$(CC) $(LOADER_CPPFLAGS) $(LOADER_CFLAGS) -c -o $@ loader.c

abi.o: abi.S
	$(CC) $(LOADER_CPPFLAGS) -c -o $@ abi.S

check: executor loader
	GATE_RUN_TEST_EXECUTOR=$(PWD)/executor GATE_RUN_TEST_LOADER=$(PWD)/loader GATE_RUN_TEST_ELF=$(TEST_ELF) $(GO) test -v ./*.go

clean:
	rm -f executor loader *.o

.PHONY: build check clean
