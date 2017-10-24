# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

LLVMPREFIX	?= $(GATEDIR)/../wag-toolchain/out

CC		:= $(LLVMPREFIX)/bin/clang --target=wasm32-unknown-unknown
CXX		:= $(LLVMPREFIX)/bin/clang --target=wasm32-unknown-unknown
LLVMAS		:= $(LLVMPREFIX)/bin/llvm-as
LLVMLINK	:= $(LLVMPREFIX)/bin/llvm-link

CPPFLAGS	+= \
	-D_LIBCPP_HAS_MUSL_LIBC \
	-D_LIBCPP_HAS_NO_THREADS \
	-D_LIBCPP_NO_EXCEPTIONS \
	-D__ELF__ \
	-nostdlibinc \
	-I$(GATEDIR)/capi/include \
	-I$(GATEDIR)/libcxx/libcxx/include \
	-I$(GATEDIR)/libc/include \
	-I$(GATEDIR)/libc/musl/include \
	-I$(GATEDIR)/libc/musl/arch/wasm32

CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -Oz
CXXFLAGS	+= -std=c++14 -fno-exceptions -fno-rtti
LDFLAGS		+= -nostdlib -Wl,--allow-undefined-file=$(GATEDIR)/capi/abi.list -Wl,--check-signatures
LIBS		+= $(GATEDIR)/crt/crt.bc $(GATEDIR)/capi/gate.bc

prog.wasm: $(OBJECTS)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $(OBJECTS) $(LIBS)

%.bc: %.c
	$(CC) $(CPPFLAGS) $(CFLAGS) -emit-llvm -c -o $@ $*.c

%.bc: %.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -include $(GATEDIR)/crt/main.hpp -emit-llvm -c -o $@ $*.cpp
