GATEDIR		:= ..

PYTHON		?= python

CPPFLAGS	+= -isystem musl/arch/wasm32 -isystem musl/include -DNDEBUG
CFLAGS		+= -Wall -Wextra -fomit-frame-pointer -Oz

LIBC_OBJECTS	:= $(addprefix libc_build/, \
	atoi.bc \
	memcpy.bc \
	memset.bc \
	stpcpy.bc \
	stpncpy.bc \
	strcasecmp.bc \
	strcmp.bc \
	strcpy.bc \
	strlen.bc \
	strncasecmp.bc \
	strncmp.bc \
	strncpy.bc \
	tolower.bc \
)

build: $(LIBC_OBJECTS) compat.bc

include $(GATEDIR)/crt/rules.make

LLVMBINDIR	:= $(WAGTOOLCHAIN)/llvm-build/bin
BINARYENBINDIR	:= $(WAGTOOLCHAIN)/binaryen-build/bin

libc_build/%.ll:
	PYTHONPATH=musl $(PYTHON) -B musl.py --clang_dir=$(realpath $(LLVMBINDIR)) --binaryen_dir=$(realpath $(BINARYENBINDIR)) --musl=$(realpath musl) --save-temps

libc_build/%.bc: libc_build/%.ll
	$(LLVMBINDIR)/llvm-as -o $@ libc_build/$*.ll

compat.bc: compat.c $(GATEDIR)/include/gate.h Makefile

clean:
	rm -f *.bc *.wast
	rm -rf libc_build

.PHONY: build clean
