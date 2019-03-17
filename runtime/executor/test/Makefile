GTEST		?= /usr/src/googletest/googletest

CPPFLAGS	+= -pthread -I$(GTEST) -I../../include -I..
CFLAGS		+= -g -Wall -Wextra
CXXFLAGS	+= -std=gnu++11
LDFLAGS		+= -pthread

include ../../include/runtime.mk

objects		:= gtest-all.o gtest_main.o map_test.o

check: test
	./test

all: test

test: $(objects)
	$(CXX) $(CXXFLAGS) $(CFLAGS) $(LDFLAGS) -o $@ $(objects)

%.o: %.cpp $(wildcard ../*.h ../../include/*.h) Makefile ../../include/runtime.mk
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(CFLAGS) -c -o $@ $*.cpp

gtest-all.o: $(GTEST)/src/gtest-all.cc
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(CFLAGS) -c -o $@ $^

gtest_main.o: $(GTEST)/src/gtest_main.cc
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(CFLAGS) -c -o $@ $^

clean:
	rm -f test *.o

.PHONY: check all clean
