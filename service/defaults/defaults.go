// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package defaults

import (
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/peer"
)

func init() {
	origin.Register(nil) // code 1 for unit tests

	echo.Register(nil)
	peer.Register(nil)
}
