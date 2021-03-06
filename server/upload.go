// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/server/api"
)

// ModuleUpload parameters.  Server may take possession of Stream; Close must
// be called in case it remains non-nil.
type ModuleUpload struct {
	Stream io.ReadCloser
	Length int64
	Hash   string
}

func (opt *ModuleUpload) takeStream() io.ReadCloser {
	r := opt.Stream
	opt.Stream = nil
	return r
}

func (opt *ModuleUpload) _validate() {
	h := api.KnownModuleHash.New()

	if _, err := io.Copy(h, opt.Stream); err != nil {
		panic(wrapContentError(err))
	}

	if err := opt.takeStream().Close(); err != nil {
		panic(wrapContentError(err))
	}

	_validateHashBytes(opt.Hash, h.Sum(nil))
}
