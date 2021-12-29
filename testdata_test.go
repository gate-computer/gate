// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"strings"
)

var (
	wasmABI           = readFile("testdata/abi.wasm")
	wasmHello         = readFile("testdata/hello.wasm")
	wasmHelloDebug    = readFile("testdata/hello-debug.wasm")
	wasmNop           = readFile("testdata/nop.wasm")
	wasmRandomSeed    = readFile("testdata/randomseed.wasm")
	wasmSnapshotAMD64 = readFile("testdata/snapshot.amd64.wasm.gz")
	wasmSnapshotARM64 = readFile("testdata/snapshot.arm64.wasm.gz")
	wasmSuspend       = readFile("testdata/suspend.wasm")
	wasmTime          = readFile("testdata/time.wasm")
)

var (
	hashNop     = sha256hex(wasmNop)
	hashHello   = sha256hex(wasmHello)
	hashSuspend = sha256hex(wasmSuspend)
)

func readFile(filename string) []byte {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	if !strings.HasSuffix(filename, ".gz") {
		return data
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}

	data, err = ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}

	if err := r.Close(); err != nil {
		panic(err)
	}

	return data
}

func sha256hex(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
