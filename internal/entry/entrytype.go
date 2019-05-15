// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"github.com/tsavola/wag/wa"
)

func CheckType(sig wa.FuncType) bool {
	return len(sig.Params) == 0 && (sig.Result == wa.Void || sig.Result == wa.I32)
}
