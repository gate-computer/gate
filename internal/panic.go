// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

// ServerPanic configures public server API behavior.  If this is changed to
// "1", API functions will panic instead of returning error values.  The stack
// traces can be helpful for debugging.
//
// This can be set during linking:
//
//     go build -ldflags="-X gate.computer/gate/internal.ServerPanic=1"
//
// This is not a stable feature: it may change or disappear at any time.
var ServerPanic string
