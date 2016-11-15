GATEDIR		:= ..

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter -ffreestanding -Oz

build: $(GATEDIR)/crt/start.bc

start.bc: start.c $(GATEDIR)/include/gate.h

clean:
	rm -f *.bc

.PHONY: build clean

include rules.make
