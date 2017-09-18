# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include -isystem $(GATEDIR)/libc/include

build: gate.bc service.bc

gate.bc: $(GATEDIR)/capi/include/gate.h
service.bc: $(GATEDIR)/capi/include/gate.h $(GATEDIR)/capi/include/gate/service.h

clean:
	rm -f *.bc

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
