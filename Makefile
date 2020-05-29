GO		:= go
DESTDIR		:=
PREFIX		:= /usr/local
LIBDIR		:= $(PREFIX)/lib/gate
FILENAME	:= $(notdir $(shell $(GO) list)).so

check: build
	GO111MODULE=on $(GO) test $(GOTESTFLAGS) ./...

build:
	GO111MODULE=on $(GO) build -trimpath $(GOBUILDFLAGS) -buildmode=plugin -o $(FILENAME) ./plugin
	GO111MODULE=on $(GO) vet $(GOVETFLAGS) ./...

install:
	install -m 755 -d $(DESTDIR)$(LIBDIR)/plugin
	install -m 644 $(FILENAME) $(DESTDIR)$(LIBDIR)/plugin/

clean:
	rm -f *.so

.PHONY: check build install clean
