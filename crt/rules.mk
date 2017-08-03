ifeq ($(TOOLCHAINDIR),)
DOCKER		?= docker

TOOLCHAINIMAGE	?= tsavola/wag-toolchain

uid		:= $(shell id -u)
gid		:= $(shell id -g)
gatedir		:= $(shell realpath $(GATEDIR))
pwd		:= $(shell pwd)
dockertoolchain	:= $(DOCKER) run --rm -i -u $(uid):$(gid) -v $(gatedir):$(gatedir) -w $(pwd) -e PYTHONPATH=$$(echo $$PYTHONPATH) -e WAGTOOLCHAIN_ALLOCATE_STACK=$$(echo $$WAGTOOLCHAIN_ALLOCATE_STACK) $(TOOLCHAINIMAGE)

# directories are inside Docker container
LLVMBINDIR	:= /usr/local/llvm-build/bin
BINARYENBINDIR	:= /usr/local/binaryen-build/bin

PYTHON		:= $(dockertoolchain) python
CC		:= $(dockertoolchain) compile
CXX		:= $(dockertoolchain) compile
LINKER		:= $(dockertoolchain) link
LLVMAS		:= $(dockertoolchain) $(LLVMBINDIR)/llvm-as
else
PYTHON		?= python

LLVMBINDIR	:= $(TOOLCHAINDIR)/llvm-build/bin
BINARYENBINDIR	:= $(TOOLCHAINDIR)/binaryen-build/bin

CC		:= $(TOOLCHAINDIR)/bin/compile
CXX		:= $(TOOLCHAINDIR)/bin/compile
LINKER		:= $(TOOLCHAINDIR)/bin/link
LLVMAS		:= $(LLVMBINDIR)/llvm-as
endif

CPPFLAGS	+= -isystem $(GATEDIR)/include

prog.wasm: $(OBJECTS)
	$(LINKER) -o $@ $(OBJECTS) $(LIBS)

%.bc: %.c
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.bc: %.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -include $(GATEDIR)/crt/main.hpp -fno-exceptions -c -o $@ $*.cpp
