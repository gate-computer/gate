build:
	$(MAKE) -C llvmpass
	$(MAKE) -C run/executor
	$(MAKE) -C run/loader
	$(MAKE) -C crt
	$(MAKE) -C elf2payload

all: build
	$(MAKE) -C crt/test

check: all
	$(MAKE) -C crt/test check
	$(MAKE) -C assemble check
	$(MAKE) -C run check

clean:
	$(MAKE) -C llvmpass clean
	$(MAKE) -C run/executor clean
	$(MAKE) -C run/loader clean
	$(MAKE) -C crt clean
	$(MAKE) -C crt/test clean
	$(MAKE) -C elf2payload clean

.PHONY: build all check clean
