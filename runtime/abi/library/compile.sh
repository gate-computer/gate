#!/bin/sh -e

library=$(dirname "$0")
project="${library}/../../.."

exec ${WASM_CC:-${CC:-clang}} \
	--target=wasm32 \
	-Os -finline-functions -fomit-frame-pointer \
	-Wall -Wextra -Wno-unused-parameter \
	-nostdlib \
	-I"${project}/include" \
	-c \
	"$@" \
	"${library}/library.c"
