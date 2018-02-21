# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

export WAGTOOLCHAIN_ALLOCATE_STACK := 1048576

SHA512SUM	?= sha512sum

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include -isystem $(GATEDIR)/libc/include
CXXFLAGS	+= -std=c++14

SOURCE		?= $(firstword $(wildcard *.c *.cpp))
OBJECT		?= $(patsubst %.cpp,%.bc,$(patsubst %.c,%.bc,$(SOURCE)))

OBJECTS		:= $(OBJECT)

build: prog.wasm prog.wasm.sha512sum

$(OBJECT): $(SOURCE) $(GATEDIR)/capi/include/gate.h Makefile $(GATEDIR)/tests/test.mk $(GATEDIR)/crt/rules.mk

prog.wasm.sha512sum: prog.wasm
	$(SHA512SUM) prog.wasm > $@

clean:
	rm -f prog.* *.bc

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
