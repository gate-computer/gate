CMAKE		?= cmake

../lib/libgatepass.so: build/Makefile
	$(MAKE) -C build
	mkdir -p $(dir $@)
	cp build/libgatepass.so $@

build/Makefile: CMakeLists.txt
	mkdir -p build
	cd build && $(CMAKE) ..

clean:
	rm -rf build

.PHONY: clean
