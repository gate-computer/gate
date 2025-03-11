// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/server/api"
)

func mustValidateUpload(opt *api.ModuleUpload) {
	h := api.KnownModuleHash.New()

	if _, err := io.Copy(h, opt.Stream); err != nil {
		z.Panic(wrapContentError(err))
	}

	if err := opt.TakeStream().Close(); err != nil {
		z.Panic(wrapContentError(err))
	}

	mustValidateHashBytes(opt.Hash, h.Sum(nil))
}
