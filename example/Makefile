SYSROOT		?= ../../wasi-sysroot
GATEDIR		?= ../../gate
FLATBUFFERSDIR	?= ../../flatbuffers

CXX		:= clang++
FLATC		:= flatc
GATE		:= gate

CPPFLAGS	+= -I$(GATEDIR)/include -I$(FLATBUFFERSDIR)/include
CXXFLAGS	+= -std=c++17
CFLAGS		+= --target=wasm32-wasi --sysroot=$(SYSROOT) -fno-exceptions -g -Wall -Wextra
LDFLAGS		+= -Wl,--allow-undefined

build: test.wasm

test.wasm: test.o
	$(CXX) $(CFLAGS) $(CXXFLAGS) $(LDFLAGS) -o $@ test.o

%.o: %.cpp localhost_generated.h Makefile
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -c -o $@ $*.cpp

localhost_generated.h: ../localhost.fbs Makefile
	$(FLATC) --cpp --gen-object-api --gen-nullable ../localhost.fbs

check: test.wasm
	$(GATE) call test.wasm

clean:
	rm -f *.o *.wasm

.PHONY: build check clean
