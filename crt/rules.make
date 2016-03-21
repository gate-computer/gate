prog.bc: $(OBJECTS)
	$(LLVMLINK) -o $@ $(CRTOBJECTS) $(OBJECTS)

prog.S: prog.bc
	$(OPT) -load=$(PASS_PLUGIN) -gate < prog.bc | $(LLC) -o $@

prog.o: prog.S
	$(AS) -o $@ prog.S

prog.elf: prog.o
	$(LD) $(LDFLAGS) -o $@ prog.o

prog.payload: prog.elf
	$(ELF2PAYLOAD) > $@ < prog.elf || (rm -f $@; false)

%.bc: %.c
	$(CLANG) $(CPPFLAGS) $(CFLAGS) -c -o $@ $*.c

%.bc: %.cpp
	$(CLANGPP) $(CPPFLAGS) $(CFLAGS) $(CXXFLAGS) -include $(GATEDIR)/crt/main.hpp -c -o $@ $*.cpp
