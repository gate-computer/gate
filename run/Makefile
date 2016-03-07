GO		?= go

PWD		:= $(shell pwd)
TEST_ELF	:= $(dir $(PWD))/test/prog.elf

nothing:

check:
	GATE_RUN_TEST_EXECUTOR=$(PWD)/executor/executor GATE_RUN_TEST_LOADER=$(PWD)/loader/loader GATE_RUN_TEST_ELF=$(TEST_ELF) $(GO) test -v .

.PHONY: nothing check
