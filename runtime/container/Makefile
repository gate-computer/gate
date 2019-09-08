PKGCONFIG	?= pkg-config
SETCAP		?= setcap

CGROUP_BACKEND	?= systemd

ARCH		:= $(shell $(CC) -dumpmachine | cut -d- -f1)

pkgs		:=
ifeq ($(CGROUP_BACKEND),systemd)
pkgs		+= libsystemd
endif

CPPFLAGS	+= -I../include/$(ARCH) -I../include
CFLAGS		+= -std=gnu99 -Og -g -Wall -Wextra -Wno-unused-parameter
CFLAGS		+= $(shell $(PKGCONFIG) --cflags $(pkgs))
LIBS		+= $(shell $(PKGCONFIG) --libs $(pkgs))

include ../include/runtime.mk

binary		:= ../../lib/gate/runtime/gate-runtime-container.$(GATE_RUNTIME_BINARY_SUFFIX)
variant		:= ../../lib/runtime-container-cgroup-$(CGROUP_BACKEND)

all: $(variant)
	mkdir -p $(dir $(binary))
	ln -f $(variant) $(binary)

$(variant): container.o cgroup-$(CGROUP_BACKEND).o
	mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ container.o cgroup-$(CGROUP_BACKEND).o $(LIBS)

container.o: container.c $(wildcard *.h ../include/$(ARCH)/*.h ../include/*.h) Makefile ../include/runtime.mk
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ container.c

cgroup-$(CGROUP_BACKEND).o: cgroup-$(CGROUP_BACKEND).c $(wildcard *.h ../include/$(ARCH)/*.h ../include/*.h)
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ cgroup-$(CGROUP_BACKEND).c

install:
	install -m 755 $(binary) $(LIBDIR)/runtime

capabilities:
	test -O $(LIBDIR)/runtime
	chmod -R go-w $(LIBDIR)/runtime
	chmod go-wx $(LIBDIR)/runtime/$(notdir $(binary))
	$(SETCAP) cap_sys_admin,cap_dac_override,cap_setuid+ep $(LIBDIR)/runtime/$(notdir $(binary))

clean:
	rm -f *.o

.PHONY: all install capabilities clean
