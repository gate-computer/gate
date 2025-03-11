// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"os"
	"path"
)

func download(filename string, get func() (io.Reader, int64)) {
	var (
		out  *os.File
		temp bool
	)

	if filename == "" {
		out = os.Stdout
	} else {
		f, err := os.OpenFile(filename, os.O_WRONLY, 0)
		if err == nil {
			if info := must(f.Stat()); info.Mode().IsRegular() {
				f.Close()
				temp = true
			} else {
				out = f
			}
		} else {
			if os.IsNotExist(err) {
				temp = true
			} else {
				fatal(err)
			}
		}
	}

	in, length := get()

	if temp {
		out = must(os.CreateTemp(path.Dir(filename), ".*.wasm"))
		defer func() {
			if out != nil {
				os.Remove(out.Name())
			}
		}()
	}

	if must(io.Copy(out, in)) != length {
		fatal(io.ErrUnexpectedEOF)
	}
	z.Check(out.Close())

	if temp {
		z.Check(os.Rename(out.Name(), filename))
		out = nil
	}
}
