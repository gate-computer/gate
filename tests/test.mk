export WAGTOOLCHAIN_ALLOCATE_STACK := 1048576

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -ffreestanding -Oz
CXXFLAGS	+= -std=c++14

SOURCE		?= $(firstword $(wildcard *.c *.cpp))
OBJECT		?= $(patsubst %.cpp,%.bc,$(patsubst %.c,%.bc,$(SOURCE)))

OBJECTS		:= $(OBJECT)

build: prog.wasm test.html

$(OBJECT): $(SOURCE) $(GATEDIR)/include/gate.h Makefile $(GATEDIR)/tests/test.mk $(GATEDIR)/crt/rules.mk

test.html: prog.wasm
	echo > $@ \
		"<script src=\"../../run/run.js\"></script>" \
		"<script src=\"../../run/run_test.js\"></script>" \
		"<script>testRun('data:;base64,"$$(base64 -w0 prog.wasm)"')</script>"

clean:
	rm -f prog.* test.html *.bc *.s *.wast

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
