WAGTOOLCHAIN	?= $(GATEDIR)/../wag-toolchain

CC		:= $(WAGTOOLCHAIN)/bin/compile
CXX		:= $(WAGTOOLCHAIN)/bin/compile
LINKER		:= $(WAGTOOLCHAIN)/bin/link

CPPFLAGS	+= -isystem $(GATEDIR)/include

prog.wasm: $(OBJECTS)
	$(LINKER) -o $@ $(GATEDIR)/crt/start.bc $(OBJECTS) $(LIBS)

%.bc: %.c
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.bc: %.cpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -include $(GATEDIR)/crt/main.hpp -fno-exceptions -c -o $@ $*.cpp
