FROM tsavola/wag-toolchain

RUN mv /usr/local/bin/compile /usr/local/bin/wag-compile && \
    mv /usr/local/bin/link /usr/local/bin/wag-link

COPY docker/toolchain/compile /usr/local/bin/compile
COPY docker/toolchain/compile++ /usr/local/bin/compile++
COPY docker/toolchain/link /usr/local/bin/link

COPY include/ /usr/include/
COPY libc/musl/arch/wasm32/ /usr/include/
COPY libc/musl/include/ /usr/include/

COPY crt/main.hpp /lib/wasm32/
COPY libc/compat.bc /lib/wasm32/
COPY libc/libc_build/ /lib/wasm32/
COPY malloc/all.bc /lib/wasm32/malloc.bc
