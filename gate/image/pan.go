// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"import.name/pan"
)

var z = new(pan.Zone)

func must[T any](x T, err error) T {
	z.Check(err)
	return x
}
