GATEDIR		:= ..

CPPFLAGS	+= -isystem musl/arch/wasm32 -isystem musl/include -DNDEBUG
CFLAGS		+= -Wall -Wextra -Wno-unused-parameter -fomit-frame-pointer -Os

LIBC_OBJECTS := $(addprefix libc_build/, \
	__errno_location.bc \
	calloc.bc \
	expand_heap.bc \
	libc.bc \
	malloc.bc \
	memcpy.bc \
	memset.bc \
)

build: $(LIBC_OBJECTS) compat.bc wasm.bc

include $(GATEDIR)/crt/rules.make

LLVMBINDIR	:= $(WAGTOOLCHAIN)/llvm-build/bin
BINARYENBINDIR	:= $(WAGTOOLCHAIN)/binaryen-build/bin

libc_build/%.ll:
	musl/libc.py --clang_dir=$(realpath $(LLVMBINDIR)) --binaryen_dir=$(realpath $(BINARYENBINDIR)) --musl=$(realpath musl) --save-temps

libc_build/%.bc: libc_build/%.ll
	$(LLVMBINDIR)/llvm-as -o $@ libc_build/$*.ll

compat.bc: compat.c $(GATEDIR)/include/gate.h

wasm.bc: wasm.ll
	$(LLVMBINDIR)/llvm-as -o $@ wasm.ll

clean:
	rm -f *.bc *.s *.wast
	rm -rf libc_build

.PHONY: build clean
