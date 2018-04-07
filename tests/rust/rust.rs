// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#![feature(lang_items)]
#![no_main]
#![no_std]

#[no_mangle]
pub fn _start() {
}

#[lang = "panic_fmt"]
extern "C" fn panic_fmt() -> ! {
    loop {}
}
