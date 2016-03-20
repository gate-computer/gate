LLVM_PREFIX	:=
LLVM_SUFFIX	:=
LLC		:= $(LLVM_PREFIX)llc$(LLVM_SUFFIX)
LLVMLINK	:= $(LLVM_PREFIX)llvm-link$(LLVM_SUFFIX)
LLVMOPT		:= $(LLVM_PREFIX)opt$(LLVM_SUFFIX)
LLVMDIS		:= $(LLVM_PREFIX)llvm-dis$(LLVM_SUFFIX)

CLANG_PREFIX	:=
CLANG_SUFFIX	:= -3.7
CLANG		:= $(CLANG_PREFIX)clang$(CLANG_SUFFIX)
CLANGPP		:= $(CLANG_PREFIX)clang++$(CLANG_SUFFIX)

INCLUDEDIR	:= $(CRTDIR)/../include

CPPFLAGS	+= -I$(INCLUDEDIR)
CFLAGS		+= -emit-llvm -ffreestanding -fno-stack-protector
CXXFLAGS	+= -fno-exceptions

CRTOBJECTS	:= $(CRTDIR)/start.bc $(CRTDIR)/memcpy.bc $(CRTDIR)/memset.bc
