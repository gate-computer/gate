GATEDIR		:= ../..

include ../clang.make
include ../llvm.make

CPPFLAGS	:= -isystem arch -isystem musl/include -isystem musl/obj/include -I../include
CFLAGS		:= -g -Wall -Wextra -Wno-unused-parameter -emit-llvm -Os
CXXFLAGS	:= -std=c++14

MUSL_OBJECTS := \
	musl/obj/src/internal/libc.o \
	musl/obj/src/malloc/malloc.o

OBJECTS := \
	$(MUSL_OBJECTS) \
	dummy.bc \
	errno.bc \
	expand_heap.bc \
	memcpy.bc \
	memset.bc

CXXOBJECTS := \
	new.bc

build: libc.bc libc++.bc

libc.bc: $(OBJECTS)
	$(LLVMLINK) -o $@ $(OBJECTS)

libc++.bc: $(CXXOBJECTS)
	$(LLVMLINK) -o $@ $(CXXOBJECTS)

$(MUSL_OBJECTS): $(wildcard arch/*.h)
	cd musl && ./configure CC="$(CLANG)" CFLAGS="-g -emit-llvm"
	$(MAKE) -C musl ARCH=../../arch $(MUSL_OBJECTS:musl/%=%)

clean:
	- $(MAKE) -C musl clean
	rm -f *.bc

.PHONY: build clean

include ../crt/rules.make
