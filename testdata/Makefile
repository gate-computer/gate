CC		:= clang
WAT2WASM	:= wat2wasm

CPPFLAGS	+= -nostdlibinc -I../include
CFLAGS		+= --target=wasm32-unknown-unknown -fomit-frame-pointer -Oz -Wall -Wextra
LDFLAGS		+= -nostdlib -Wl,--allow-undefined -Wl,--entry=__wasm_call_ctors -Wl,--export=main

-include ../config.mk

all: $(patsubst %.c,%.wasm,$(wildcard *.c)) nop.wasm

%.o: %.c Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.wasm: %.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $*.o
	chmod -x $@

hello.wasm: LDFLAGS += -Wl,--export=twice -Wl,--export=multi -Wl,--export=test_plugin

nop.wasm: nop.wat
	$(WAT2WASM) -o $@ nop.wat

clean:
	rm -f *.o

.PHONY: all clean
