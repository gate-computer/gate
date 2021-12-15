// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package gate_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"path/filepath"
	"testing"

	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	werrors "gate.computer/wag/errors"
)

func FuzzServerUploadModule(f *testing.F) {
	filenames, err := filepath.Glob("testdata/*.wasm")
	if err != nil {
		f.Fatal(err)
	}
	for _, filename := range filenames {
		wasm, err := ioutil.ReadFile(filename)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(wasm)
	}

	s, err := newServer()
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, wasm []byte) {
		wasmHash := hex.EncodeToString(api.KnownModuleHash.New().Sum(wasm))

		upload := &server.ModuleUpload{
			Stream: ioutil.NopCloser(bytes.NewReader(wasm)),
			Length: int64(len(wasm)),
			Hash:   wasmHash,
		}

		resultHash, err := s.UploadModule(context.Background(), upload, nil)

		if err != nil && err != io.ErrUnexpectedEOF { // TODO: should be public
			var public werrors.PublicError
			if !errors.As(err, &public) {
				t.Fatal(err)
			}
		}

		if err == nil || resultHash != "" {
			if resultHash != wasmHash {
				t.Errorf("incorrect module hash: %q", resultHash)
			}
		}

	})
}
