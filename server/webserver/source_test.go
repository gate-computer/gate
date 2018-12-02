// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"io/ioutil"

	"github.com/tsavola/gate/internal/test/runtimeutil"
)

func sha384(data []byte) string {
	hash := sha512.New384()
	hash.Write(data)
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

var (
	testProgHello = runtimeutil.MustReadFile("../../testdata/hello.wasm")
	testHashHello = sha384(testProgHello)
)

type testSource struct{}

func (testSource) OpenURI(ctx context.Context, uri string, maxSize int) (contentLength int64, content io.ReadCloser, err error) {
	switch uri {
	case "/test/hello":
		contentLength = int64(len(testProgHello))
		content = ioutil.NopCloser(bytes.NewReader(testProgHello))

	default:
		panic(uri)
	}

	return
}
