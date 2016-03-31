include $(GATEDIR)/llvm.make
include $(GATEDIR)/clang.make

PASS_PLUGIN	:= $(GATEDIR)/lib/libgatepass.so
ELF2PAYLOAD	:= $(GATEDIR)/bin/elf2payload

CPPFLAGS	+= -I$(GATEDIR)/include
CFLAGS		+= -emit-llvm -ffreestanding -fno-stack-protector
CXXFLAGS	+= -fno-exceptions
LDFLAGS		+= -T$(GATEDIR)/assemble/link.ld

CRTOBJECTS	:= $(GATEDIR)/crt/start.bc $(GATEDIR)/crt/memcpy.bc $(GATEDIR)/crt/memset.bc
