// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"crypto/sha512"
	"encoding/base64"
	"io/ioutil"
)

var (
	wasmNop        = readFile("testdata/nop.wasm")
	wasmHello      = readFile("testdata/hello.wasm")
	wasmHelloDebug = readFile("testdata/hello-debug.wasm")
	wasmSuspend    = readFile("testdata/suspend.wasm")
)

var (
	hashNop     = sha384(wasmNop)
	hashHello   = sha384(wasmHello)
	hashSuspend = sha384(wasmSuspend)
)

func readFile(filename string) (data []byte) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return
}

func sha384(data []byte) string {
	hash := sha512.New384()
	hash.Write(data)
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}
