// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gatedebug

package debug

import (
	"fmt"
)

func Printf(format string, args ...interface{}) {
	print(fmt.Sprintf(format+"\n", args...))
}
