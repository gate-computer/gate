// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gate.computer/gate/server/api"
	werrors "gate.computer/wag/errors"
	"github.com/stretchr/testify/assert"

	. "import.name/testing/mustr"
)

func FuzzServerUploadModule(f *testing.F) {
	for _, filename := range Must(f, R(filepath.Glob("../testdata/*.wasm"))) {
		f.Add(Must(f, R(os.ReadFile(filename))))
	}

	s := Must(f, R(newServer()))

	f.Fuzz(func(t *testing.T, wasm []byte) {
		wasmHash := hex.EncodeToString(api.KnownModuleHash.New().Sum(wasm))

		upload := &api.ModuleUpload{
			Stream: io.NopCloser(bytes.NewReader(wasm)),
			Length: int64(len(wasm)),
			Hash:   wasmHash,
		}

		resultHash, err := s.UploadModule(context.Background(), upload, nil)

		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) { // TODO: should be public
			if werrors.AsPublicError(err) == nil {
				t.Fatal(err)
			}
		}

		if err == nil || resultHash != "" {
			assert.Equal(t, resultHash, wasmHash)
		}
	})
}
