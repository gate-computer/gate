include $(GATEDIR)/llvm.make
include $(GATEDIR)/clang.make

INCLUDEDIR	:= $(GATEDIR)/include

CPPFLAGS	+= -I$(INCLUDEDIR)
CFLAGS		+= -emit-llvm -ffreestanding -fno-stack-protector
CXXFLAGS	+= -fno-exceptions

CRTOBJECTS	:= $(GATEDIR)/crt/start.bc $(GATEDIR)/crt/memcpy.bc $(GATEDIR)/crt/memset.bc
