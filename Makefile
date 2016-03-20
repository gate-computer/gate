build:
	$(MAKE) -C llvmpass
	$(MAKE) -C run
	$(MAKE) -C crt

all: build
	$(MAKE) -C crt/test

check: all
	$(MAKE) -C run check

clean:
	$(MAKE) -C llvmpass clean
	$(MAKE) -C run clean
	$(MAKE) -C crt clean
	$(MAKE) -C crt/test clean

.PHONY: build all check clean
