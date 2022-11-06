// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"sort"
)

func searchUint64(a []uint64, x uint64) int {
	return sort.Search(len(a), func(i int) bool {
		return a[i] >= x
	})
}
