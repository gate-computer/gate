GATEDIR		:= ..

include variables.make

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -fomit-frame-pointer -Os -g -Wall -Wextra -Wno-unused-parameter

build: $(CRTOBJECTS)

start.bc: start.c $(INCLUDEDIR)/gate.h $(INCLUDEDIR)/gate/args.h
memcpy.bc: memcpy.c
memset.bc: memset.c

clean:
	rm -f *.bc

.PHONY: build clean

include rules.make
