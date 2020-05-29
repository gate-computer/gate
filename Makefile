GO		:= go
DESTDIR		:=
PREFIX		:= /usr/local
LIBDIR		:= $(PREFIX)/lib/gate
FILENAME	:= $(notdir $(shell $(GO) list)).so

-include config.mk

build:
	GO111MODULE=on $(GO) build $(GOBUILDFLAGS) -buildmode=plugin -o $(FILENAME) ./plugin

check: build
	GO111MODULE=on $(GO) test $(GOTESTFLAGS) ./...
	GO111MODULE=on $(GO) vet $(GOVETFLAGS) ./...

install:
	install -m 755 -d $(DESTDIR)$(LIBDIR)/plugin
	install -m 644 $(FILENAME) $(DESTDIR)$(LIBDIR)/plugin/

clean:
	rm -f *.so

.PHONY: build check install clean
