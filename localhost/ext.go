// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"gate.computer/gate/service"

	. "import.name/type/context"
)

const extName = "localhost"

var extConfig Config

var Ext = service.Extend(extName, &extConfig, func(ctx Context, r *service.Registry) error {
	if extConfig.Addr == "" {
		return nil
	}

	l, err := New(&extConfig)
	if err != nil {
		return err
	}
	return r.Register(l)
})
