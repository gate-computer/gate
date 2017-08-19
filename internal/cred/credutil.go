// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cred

import (
	"fmt"
	"strconv"
)

func FormatId(i uint) string {
	return strconv.FormatUint(uint64(i), 10)
}

// ValidateId makes sure that id is not root.
func ValidateId(kind string, id uint) (err error) {
	if id == 0 {
		err = fmt.Errorf("one of the configured %s ids is 0", kind)
	}
	return
}

// ValidateIds makes sure that ids don't conflict between themselves, with the
// current process, or root.
func ValidateIds(kind string, currentId, untilIndex int, ids ...uint) (err error) {
	for i, id := range ids[:untilIndex] {
		err = ValidateId(kind, ids[i])
		if err != nil {
			return
		}

		if currentId >= 0 && id == uint(currentId) {
			err = fmt.Errorf("one of the configured %s ids is same as the current %s id (%d)", kind, kind, currentId)
			return
		}
	}

	for i := range ids[:untilIndex] {
		for j := i + 1; j < len(ids); j++ { // all ids, not just untilIndex
			if ids[i] == ids[j] {
				err = fmt.Errorf("%s id %d appears multiple times in configuration", kind, ids[i])
				return
			}
		}
	}

	return
}
