CC		:= clang
WAT2WASM	:= wat2wasm

CPPFLAGS	+= -nostdlibinc -I../include
CFLAGS		+= --target=wasm32-unknown-unknown -Os -fomit-frame-pointer -fno-inline -Wall -Wextra
LDFLAGS		+= -nostdlib -Wl,--allow-undefined -Wl,--no-entry

-include ../config.mk

all: $(patsubst %.c,%.wasm,$(wildcard *.c)) nop.wasm

%.o: %.c Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.wasm: %.o
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $*.o
	chmod -x $@

hello.wasm: LDFLAGS += -Wl,--export=greet -Wl,--export=twice -Wl,--export=multi -Wl,--export=repl -Wl,--export=fail -Wl,--export=test_plugin
hello-debug.wasm: LDFLAGS += -Wl,--export=log
randomseed.wasm: LDFLAGS += -Wl,--export=check
suspend.wasm: LDFLAGS += -Wl,--export=loop
time.wasm: LDFLAGS += -Wl,--export=check

nop.wasm: nop.wat
	$(WAT2WASM) -o $@ nop.wat

clean:
	rm -f *.o

.PHONY: all clean
