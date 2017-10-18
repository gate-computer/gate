# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

GATEDIR		:= ..

CPPFLAGS	+= -D_LIBCPP_BUILDING_LIBRARY

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

include $(GATEDIR)/crt/rules.mk

compat.bc: compat.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -emit-llvm -c -o $@ compat.cpp

%.bc: libcxx/src/%.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -emit-llvm -c -o $@ libcxx/src/$*.cpp

clean:
	rm -f *.bc

.PHONY: build clean
