# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

DOCKER		?= docker

GATETOOLCHAIN	?= tsavola/gate-toolchain

uid		:= $(shell id -u)
gid		:= $(shell id -g)
pwd		:= $(shell pwd)
dockertoolchain	:= $(DOCKER) run --rm -i -u $(uid):$(gid) -v $(pwd):$(pwd) -w $(pwd) -e WAGTOOLCHAIN_ALLOCATE_STACK=$(WAGTOOLCHAIN_ALLOCATE_STACK) $(GATETOOLCHAIN)

CC		:= $(dockertoolchain) compile
CXX		:= $(dockertoolchain) compile++
LINKER		:= $(dockertoolchain) link

CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz
CXXFLAGS	+= -std=c++14

LIBS := \
	/lib/wasm32/c++/new.bc \
	/lib/wasm32/gate/service.bc \
	/lib/wasm32/malloc.bc \
	/lib/wasm32/memcpy.bc \
	/lib/wasm32/memset.bc \
	/lib/wasm32/stpcpy.bc \
	/lib/wasm32/strcpy.bc \
	/lib/wasm32/strlen.bc

example.wasm: example.bc
	$(LINKER) -o $@ example.bc $(LIBS)

example.bc: example.cpp Makefile
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -o $@ example.cpp

clean:
	rm -f *.wasm *.bc

.PHONY: clean
