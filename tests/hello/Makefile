GATEDIR		:= ../..

include $(GATEDIR)/tests/test.mk

check:
	$(GATEDIR)/bin/runner --executor=$(realpath $(GATEDIR)/bin/executor) --loader=$(GATEDIR)/bin/loader --loader-symbols=$(GATEDIR)/bin/loader.symbols prog.wasm

.PHONY: check
