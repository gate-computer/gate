#!/bin/sh -e

library=$(dirname "$0")
project="${library}/../../.."

set -x

exec ${WASM_CXX:-${CXX:-clang++}} \
	--target=wasm32 \
	-std=c++17 \
	-Os \
	-finline-functions \
	-fno-exceptions \
	-fomit-frame-pointer \
	-Wall \
	-Wextra \
	-Wno-return-type-c-linkage \
	-Wno-unused-parameter \
	-Wno-unused-private-field \
	-nostdlib \
	-I"${project}/include" \
	$@ \
	"${library}/library.cpp"
