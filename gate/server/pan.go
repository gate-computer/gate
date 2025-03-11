// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"import.name/pan"
)

var z = new(pan.Zone)

func must[T any](x T, err error) T {
	z.Check(err)
	return x
}

func must2[T1, T2 any](x1 T1, x2 T2, err error) (T1, T2) {
	z.Check(err)
	return x1, x2
}
