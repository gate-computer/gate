GATEDIR		:= ..

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter

build: $(GATEDIR)/crt/start.bc

start.bc: start.c $(GATEDIR)/include/gate.h
memcpy.bc: memcpy.c
memset.bc: memset.c

clean:
	rm -f *.bc

.PHONY: build clean

include rules.make
