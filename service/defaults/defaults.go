// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Importing this package causes the service.Defaults registry to get populated
// with the built-in services.
package defaults

import (
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/peer"
)

func init() {
	echo.Default.Register(service.Defaults)
	origin.Default.Register(service.Defaults)
	peer.Default.Register(service.Defaults)
}
