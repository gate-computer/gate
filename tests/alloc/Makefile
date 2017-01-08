GATEDIR		:= ../..

LIBS := \
	$(GATEDIR)/libc/compat.bc \
	$(GATEDIR)/libc/libc_build/memcpy.bc \
	$(GATEDIR)/libc/libc_build/memset.bc \
	$(GATEDIR)/malloc/malloc.bc \
	$(GATEDIR)/malloc/morecore.bc \
	$(GATEDIR)/malloc/wasm.bc

include $(GATEDIR)/tests/test.mk

check:
	$(GATEDIR)/bin/runner --executor=$(realpath $(GATEDIR)/bin/executor) --loader=$(GATEDIR)/bin/loader --loader-symbols=$(GATEDIR)/bin/loader.symbols prog.wasm

.PHONY: check
