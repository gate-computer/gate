WAT2WASM	?= wat2wasm
PYTHON		?= python

build:
	$(WAT2WASM) -o library.wasm library.wat
	$(PYTHON) -c 'open("library.go", "w").write("// Generated from library.wat.  DO NOT EDIT.\n\npackage abi\n\nvar libraryWASM = [...]byte{" + ", ".join(["0x%02x" % ord(x) for x in open("library.wasm").read()]) + "}\n")'

.PHONY: build
