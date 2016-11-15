target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32-unknown-unknown"

declare i32 @llvm.wasm.current.memory.i32() nounwind readonly
declare void @llvm.wasm.grow.memory.i32(i32) nounwind

define i32 @__wasm_current_memory() {
	%ret = call i32 @llvm.wasm.current.memory.i32()
	ret i32 %ret
}

define void @__wasm_grow_memory(i32 %increment) {
	call void @llvm.wasm.grow.memory.i32(i32 %increment)
	ret void
}
