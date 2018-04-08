// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#![feature(lang_items)]
#![no_main]
#![no_std]

extern {
    fn __gate_exit(status: i32) -> !;

    fn __wasm_call_ctors();

    pub fn main();
}

#[no_mangle]
pub fn _start() {
    unsafe {
        __wasm_call_ctors();
        main()
    }
}

#[lang = "panic_fmt"]
fn panic_fmt() -> ! {
    unsafe {
        __gate_exit(1)
    }
}
