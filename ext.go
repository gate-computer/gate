// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shell

import (
	"context"

	"gate.computer/gate/service"
)

const extName = "shell"

var extConfig struct {
	Enabled bool
}

var Ext = service.Extend(extName, &extConfig, func(ctx context.Context, r *service.Registry) error {
	if !extConfig.Enabled {
		return nil
	}

	return r.Register(shell{})
})
