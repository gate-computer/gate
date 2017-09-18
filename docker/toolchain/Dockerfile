FROM tsavola/wag-toolchain

COPY docker/toolchain/compile /usr/local/bin/compile
COPY docker/toolchain/compile++ /usr/local/bin/compile++
COPY docker/toolchain/link /usr/local/bin/link

COPY libc/musl/arch/wasm32/ /usr/include/
COPY libc/musl/include/ /usr/include/
COPY libcxx/libcxx/include/ /usr/include/c++/
COPY capi/include/ /usr/include/

COPY crt/crt.bc /lib/wasm32/
COPY crt/main.hpp /lib/wasm32/
COPY libc/compat.bc /lib/wasm32/
COPY libc/libc_build/*.bc /lib/wasm32/
COPY malloc/all.bc /lib/wasm32/malloc.bc
COPY libcxx/*.bc /lib/wasm32/c++/
COPY capi/gate.bc /lib/wasm32/
COPY capi/service.bc /lib/wasm32/gate/
