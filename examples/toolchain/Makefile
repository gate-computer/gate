DOCKER		?= docker

GATETOOLCHAIN	?= tsavola/gate-toolchain

uid		:= $(shell id -u)
gid		:= $(shell id -g)
pwd		:= $(shell pwd)
dockertoolchain	:= $(DOCKER) run --rm -i -u $(uid):$(gid) -v $(pwd):$(pwd) -w $(pwd) -e WAGTOOLCHAIN_ALLOCATE_STACK=$(WAGTOOLCHAIN_ALLOCATE_STACK) $(GATETOOLCHAIN)

CC		:= $(dockertoolchain) compile
CXX		:= $(dockertoolchain) compile++
LINKER		:= $(dockertoolchain) link

LIBS := \
	/lib/wasm32/malloc.bc \
	/lib/wasm32/memcpy.bc \
	/lib/wasm32/memset.bc \
	/lib/wasm32/stpcpy.bc \
	/lib/wasm32/strcpy.bc \
	/lib/wasm32/strlen.bc

example.wasm: example.bc
	$(LINKER) -o $@ example.bc $(LIBS)

example.bc: example.c Makefile
	$(CC) -o $@ example.c

clean:
	rm -f *.wasm *.bc

.PHONY: clean
