# Copyright (c) 2018 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -DNDEBUG
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz

build: crt.bc

crt.bc: crt.c $(GATEDIR)/capi/include/gate.h rules.mk

clean:
	rm -f *.bc

.PHONY: build clean

include rules.mk
