; Copyright (c) 2016 Timo Savola. All rights reserved.
; Use of this source code is governed by a BSD-style
; license that can be found in the LICENSE file.

target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown"

declare i32 @llvm.wasm.current.memory.i32() nounwind readonly
declare i32 @llvm.wasm.grow.memory.i32(i32) nounwind

define i32 @__malloc_current_memory() {
	%ret = call i32 @llvm.wasm.current.memory.i32()
	ret i32 %ret
}

define i32 @__malloc_grow_memory(i32 %increment) {
	%ret = call i32 @llvm.wasm.grow.memory.i32(i32 %increment)
	ret i32 %ret
}
