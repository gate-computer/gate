# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -D_LIBCPP_HAS_MUSL_LIBC -D_LIBCPP_HAS_NO_THREADS -D__ELF__ -isystem libcxx/include -isystem $(GATEDIR)/libc/musl/arch/wasm32 -isystem $(GATEDIR)/libc/musl/include
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -Oz
CXXFLAGS	+= -std=c++14 -fno-exceptions

build: \
	algorithm.bc \
	any.bc \
	bind.bc \
	chrono.bc \
	condition_variable.bc \
	functional.bc \
	future.bc \
	hash.bc \
	ios.bc \
	iostream.bc \
	locale.bc \
	memory.bc \
	mutex.bc \
	new.bc \
	optional.bc \
	random.bc \
	regex.bc \
	shared_mutex.bc \
	stdexcept.bc \
	string.bc \
	strstream.bc \
	system_error.bc \
	thread.bc \
	typeinfo.bc \
	utility.bc \
	valarray.bc \
	variant.bc \
	vector.bc

%.bc: libcxx/src/%.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ libcxx/src/$*.cpp

clean:
	rm -f *.bc

.PHONY: build clean

include $(GATEDIR)/crt/rules.mk
