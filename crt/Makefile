CRTDIR		:= .

include $(CRTDIR)/variables.make

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter

build: crt.bc

crt.bc: crt.c $(INCLUDEDIR)/gate.h $(INCLUDEDIR)/gate/args.h

clean:
	rm -f *.bc

.PHONY: build clean

include $(CRTDIR)/rules.make
