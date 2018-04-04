# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

SOURCE		?= $(firstword $(wildcard *.c *.cpp))
OBJECT		?= $(patsubst %.cpp,%.bc,$(patsubst %.c,%.bc,$(SOURCE)))

OBJECTS		:= $(OBJECT)

build: prog.wasm

$(OBJECT): $(SOURCE) $(GATEDIR)/capi/include/gate.h Makefile $(GATEDIR)/tests/test.mk $(GATEDIR)/crt/rules.mk

clean:
	rm -f prog.* *.bc

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
