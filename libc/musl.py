# Copyright (c) 2016 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import libc


libc.WARNINGS += [
	"-Wno-absolute-value",         # src/math/fabsl.c:5
	"-Wno-logical-op-parentheses", # src/internal/shgetc.c:16
	"-Wno-macro-redefined",
]


if __name__ == "__main__":
	args = libc.getargs()
	exit(libc.run(args.clang_dir, args.binaryen_dir, args.sexpr_wasm, args.musl, args.arch, args.out, args.save_temps))
