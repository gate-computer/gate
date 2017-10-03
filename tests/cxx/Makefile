# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ../..

CXXFLAGS	+= -std=c++14

LIBS := \
	$(GATEDIR)/libc/compat.bc \
	$(GATEDIR)/libc/libc_build/memcpy.bc \
	$(GATEDIR)/libc/libc_build/memset.bc \
	$(GATEDIR)/libcxx/new.bc \
	$(GATEDIR)/libcxx/vector.bc \
	$(GATEDIR)/malloc/malloc.bc \
	$(GATEDIR)/malloc/morecore.bc \
	$(GATEDIR)/malloc/wasm.bc

include $(GATEDIR)/tests/test.mk
