// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

// Service packet properties.
type Service struct {
	MaxSendSize int // Maximum size which the program is prepared to receive.
	Code        Code
}
