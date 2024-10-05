// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"

	"gate.computer/gate/server/api"
	"import.name/pan"
)

func mustValidateUpload(opt *api.ModuleUpload) {
	h := api.KnownModuleHash.New()

	if _, err := io.Copy(h, opt.Stream); err != nil {
		pan.Panic(wrapContentError(err))
	}

	if err := opt.TakeStream().Close(); err != nil {
		pan.Panic(wrapContentError(err))
	}

	mustValidateHashBytes(opt.Hash, h.Sum(nil))
}
