GO		:= go
DESTDIR		:=
PREFIX		:= /usr/local
LIBDIR		:= $(PREFIX)/lib/gate

check: build
	GO111MODULE=on $(GO) test $(GOTESTFLAGS) ./...

build:
	GO111MODULE=on $(GO) build $(GOBUILDFLAGS) -buildmode=plugin -o localhost.so
	GO111MODULE=on $(GO) vet $(GOVETFLAGS) ./...

install:
	install -m 755 -d $(DESTDIR)$(LIBDIR)/plugin
	install -m 644 localhost.so $(DESTDIR)$(LIBDIR)/plugin/

clean:
	rm -f localhost.so

.PHONY: check build install clean
