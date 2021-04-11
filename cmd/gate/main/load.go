// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
)

func download(filename string, get func() (io.Reader, int64)) {
	var (
		out  *os.File
		temp bool
		err  error
	)

	if filename == "" {
		out = os.Stdout
	} else {
		f, err := os.OpenFile(filename, os.O_WRONLY, 0)
		if err == nil {
			info, err := f.Stat()
			check(err)
			if info.Mode().IsRegular() {
				f.Close()
				temp = true
			} else {
				out = f
			}
		} else {
			if os.IsNotExist(err) {
				temp = true
			} else {
				log.Fatal(err)
			}
		}
	}

	in, length := get()

	if temp {
		out, err = ioutil.TempFile(path.Dir(filename), ".*.wasm")
		check(err)
		defer func() {
			if out != nil {
				os.Remove(out.Name())
			}
		}()
	}

	if checkCopy(out, in) != length {
		log.Fatal(io.ErrUnexpectedEOF)
	}
	check(out.Close())

	if temp {
		check(os.Rename(out.Name(), filename))
		out = nil
	}
}
