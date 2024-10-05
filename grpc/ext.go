// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"gate.computer/gate/service"
	"gate.computer/gate/service/logger"

	. "import.name/type/context"
)

const extName = "grpc"

var extConfig Config

var Ext = service.Extend(extName, &extConfig, func(ctx Context, r *service.Registry) error {
	if err := extConfig.Init(ctx, logger.MustContextual(ctx)); err != nil {
		return err
	}

	return extConfig.Register(r)
})
