// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/server/api"
)

func validateUpload(pan icky, opt *api.ModuleUpload) {
	h := api.KnownModuleHash.New()

	if _, err := io.Copy(h, opt.Stream); err != nil {
		pan.check(wrapContentError(err))
	}

	if err := opt.TakeStream().Close(); err != nil {
		pan.check(wrapContentError(err))
	}

	validateHashBytes(pan, opt.Hash, h.Sum(nil))
}
