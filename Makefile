build:
	$(MAKE) -C llvmpass
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader

all: build
	$(MAKE) -C crt
	$(MAKE) -C libc
	$(MAKE) -C elf2payload
	$(MAKE) -C test

check: all
	$(MAKE) -C test check
	$(MAKE) -C assemble check
	$(MAKE) -C run check

clean:
	$(MAKE) -C llvmpass clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C crt clean
	$(MAKE) -C libc clean
	$(MAKE) -C elf2payload clean
	$(MAKE) -C test clean

.PHONY: build all check clean
