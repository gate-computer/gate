GATEDIR		:= ..

PYTHON		?= python

CPPFLAGS	+= -isystem musl/arch/wasm32 -isystem musl/include -DNDEBUG
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz

include $(GATEDIR)/crt/rules.mk

LLVMBINDIR	:= $(WAGTOOLCHAIN)/llvm-build/bin
BINARYENBINDIR	:= $(WAGTOOLCHAIN)/binaryen-build/bin

.DEFAULT_GOAL	:= build

build: libc_build compat.bc
	$(MAKE) libc-objects

libc_build:
	PYTHONPATH=musl $(PYTHON) -B musl.py --clang_dir=$(realpath $(LLVMBINDIR)) --binaryen_dir=$(realpath $(BINARYENBINDIR)) --musl=$(realpath musl) --save-temps

libc-objects: $(patsubst %.ll,%.bc,$(wildcard libc_build/*.ll))

libc_build/%.bc: libc_build/%.ll
	$(LLVMBINDIR)/llvm-as -o $@ libc_build/$*.ll

compat.bc: compat.c $(GATEDIR)/include/gate.h Makefile $(GATEDIR)/crt/rules.mk

clean:
	rm -f *.bc *.wast
	rm -rf libc_build

.PHONY: build libc-objects clean
