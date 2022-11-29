// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"io/ioutil"
	"os"
	"path"

	. "import.name/pan/mustcheck"
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
			if info := Must(f.Stat()); info.Mode().IsRegular() {
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
		out = Must(ioutil.TempFile(path.Dir(filename), ".*.wasm"))
		defer func() {
			if out != nil {
				os.Remove(out.Name())
			}
		}()
	}

	if Must(io.Copy(out, in)) != length {
		fatal(io.ErrUnexpectedEOF)
	}
	Check(out.Close())

	if temp {
		Check(os.Rename(out.Name(), filename))
		out = nil
	}
}
