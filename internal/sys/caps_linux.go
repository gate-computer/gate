// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

import (
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

func ClearCaps() error {
	if err := cap.NewSet().SetProc(); err != nil {
		return err
	}

	if err := cap.ResetAmbient(); err != nil {
		return err
	}

	return nil
}
