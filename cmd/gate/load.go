// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
)

func download(get func() (io.Reader, int64)) {
	var (
		out  *os.File
		temp bool
		err  error
	)

	if flag.NArg() == 1 {
		out = os.Stdout
	} else {
		f, err := os.OpenFile(flag.Arg(1), os.O_WRONLY, 0)
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
		out, err = ioutil.TempFile(path.Dir(flag.Arg(1)), ".*.wasm")
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
		check(os.Rename(out.Name(), flag.Arg(1)))
		out = nil
	}
}
