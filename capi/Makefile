# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include -isystem $(GATEDIR)/libc/include

build: service.bc

clean:
	rm -f *.bc

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
