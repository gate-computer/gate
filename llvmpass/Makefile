CMAKE		?= cmake

build: build/Makefile
	$(MAKE) -C build

build/Makefile: CMakeLists.txt
	mkdir -p build
	cd build && $(CMAKE) ..

clean:
	rm -rf build

.PHONY: build clean
