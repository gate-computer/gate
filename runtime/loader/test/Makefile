PYTHON		?= python3

ARCH		:= $(shell $(CC) -dumpmachine | cut -d- -f1)

CPPFLAGS	+= -I../../include/$(ARCH) -I../../include -DPIE
CFLAGS		+= -Os -g -Wall -Wextra -Wno-unused-parameter -Wimplicit-fallthrough -fPIE -fomit-frame-pointer -fno-stack-protector
CXXFLAGS	+= -std=c++17 -fno-exceptions
LDFLAGS		+= -static

all: signal stack

check: signal.check stack.check

signal: signal.o ../$(ARCH)/runtime2.o
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -Wl,-Ttext-segment=0x40000000 -Wl,--section-start=.runtime=0x50000000 -o $@ signal.o ../$(ARCH)/runtime2.o

signal.o: signal.cpp Makefile
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ signal.cpp

stack: stack.o ../$(ARCH)/start.o
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -nostartfiles -nostdlib -o $@ ../$(ARCH)/start.o stack.o

stack-debug: stack.o
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -o $@ stack.o

stack.o: stack.cpp $(wildcard ../../include/$(ARCH)/*.hpp) Makefile
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ stack.cpp

signal.check: signal Makefile
	./signal
	touch $@

stack.check: stack.py stack Makefile
	$(PYTHON) stack.py
	touch $@

clean:
	rm -f signal stack *.o *.check

.PHONY: all check clean
