# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

PKGCONFIG	?= pkg-config

CGROUP_BACKEND	?= systemd
UMASK		?= 0022

pkgs		:= libcap
ifeq ($(CGROUP_BACKEND),systemd)
pkgs		+= libsystemd
endif

CFLAGS		+= -std=gnu99 -g -Wall -Wextra -Wno-unused-parameter
CFLAGS		+= $(shell $(PKGCONFIG) --cflags $(pkgs))
LIBS		+= $(shell $(PKGCONFIG) --libs $(pkgs))

build: ../../lib/container-cgroup-$(CGROUP_BACKEND)
	ln -f ../../lib/container-cgroup-$(CGROUP_BACKEND) ../../lib/container

../../lib/container-cgroup-$(CGROUP_BACKEND): container.o cgroup-$(CGROUP_BACKEND).o Makefile
	umask $(UMASK) && mkdir -p $(dir $@)
	umask $(UMASK) && $(CC) $(CFLAGS) $(LDFLAGS) -o $@ container.o cgroup-$(CGROUP_BACKEND).o $(LIBS)

container.o: container.c cgroup.h ../defs.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ container.c

cgroup-$(CGROUP_BACKEND).o: cgroup-$(CGROUP_BACKEND).c cgroup.h Makefile
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ cgroup-$(CGROUP_BACKEND).c

clean:
	rm -f *.o

.PHONY: build clean
