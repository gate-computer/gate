// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dedup

import (
	"sort"
)

type uint64s []uint64

func (a uint64s) Len() int           { return len(a) }
func (a uint64s) Less(i, j int) bool { return a[i] < a[j] }
func (a uint64s) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// SortUint64 slice with deduplication.
func SortUint64(a []uint64) []uint64 {
	sort.Sort(uint64s(a))

	for i := 1; i < len(a); {
		if a[i-1] == a[i] {
			a = append(a[:i-1], a[i:]...)
		} else {
			i++
		}
	}

	return a
}
