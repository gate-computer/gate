// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"gate.computer/internal"
)

func DontPanic() bool {
	return internal.ServerPanic == ""
}
