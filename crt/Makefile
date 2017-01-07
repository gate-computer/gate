GATEDIR		:= ..

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -ffreestanding -Oz

build: $(GATEDIR)/crt/start.bc

start.bc: start.c $(GATEDIR)/include/gate.h rules.mk

clean:
	rm -f *.bc

.PHONY: build clean

include rules.mk
