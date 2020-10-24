// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
)

var (
	wasmHello      = readFile("testdata/hello.wasm")
	wasmHelloDebug = readFile("testdata/hello-debug.wasm")
	wasmNop        = readFile("testdata/nop.wasm")
	wasmRandomSeed = readFile("testdata/randomseed.wasm")
	wasmSuspend    = readFile("testdata/suspend.wasm")
	wasmTime       = readFile("testdata/time.wasm")
)

var (
	hashNop     = sha256hex(wasmNop)
	hashHello   = sha256hex(wasmHello)
	hashSuspend = sha256hex(wasmSuspend)
)

func readFile(filename string) (data []byte) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return
}

func sha256hex(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
