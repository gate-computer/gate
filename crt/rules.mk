# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

LLVMPREFIX	?= $(GATEDIR)/../wag-toolchain/out

CC		:= $(LLVMPREFIX)/bin/clang
CXX		:= $(LLVMPREFIX)/bin/clang
LLVMAS		:= $(LLVMPREFIX)/bin/llvm-as
LLVMLINK	:= $(LLVMPREFIX)/bin/llvm-link

CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -Oz
CPPFLAGS	+= -isystem $(GATEDIR)/capi/include

prog.wasm: $(OBJECTS)
	$(CC) -nostdlib -Wl,--allow-undefined-file=$(GATEDIR)/capi/abi.list -Wl,--check-signatures $(CFLAGS) $(LDFLAGS) -o $@ $(GATEDIR)/crt/crt.bc $(OBJECTS) $(LIBS)

%.bc: %.c
	$(CC) --target=wasm32-unknown-unknown -emit-llvm $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.bc: %.cpp
	$(CXX) --target=wasm32-unknown-unknown -emit-llvm $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -D_LIBCPP_HAS_MUSL_LIBC -D_LIBCPP_HAS_NO_THREADS -D__ELF__ -isystem $(GATEDIR)/libcxx/libcxx/include -include $(GATEDIR)/crt/main.hpp -fno-exceptions -c -o $@ $*.cpp
