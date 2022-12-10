// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.20

package packetio

import (
	"errors"
)

func dualError(err1, err2 error) error {
	return errors.Join(err1, err2)
}
