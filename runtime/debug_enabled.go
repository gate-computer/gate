// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build debug gateruntimedebug

package runtime

import (
	"fmt"
)

func debugf(format string, args ...interface{}) {
	print(fmt.Sprintf("runtime/"+format+"\n", args...))
}
