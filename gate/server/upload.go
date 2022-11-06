// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/server/api"
)

func _validateUpload(opt *api.ModuleUpload) {
	h := api.KnownModuleHash.New()

	if _, err := io.Copy(h, opt.Stream); err != nil {
		_check(wrapContentError(err))
	}

	if err := opt.TakeStream().Close(); err != nil {
		_check(wrapContentError(err))
	}

	_validateHashBytes(opt.Hash, h.Sum(nil))
}
