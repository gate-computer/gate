// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

// IsValidCall checks outgoing or incoming service call packet's header.
// Packet content is disregarded.
func IsValidCall(b Buf, c Code) bool {
	return len(b) >= HeaderSize && b.Code() == c && b.Domain() == DomainCall && b[offsetReserved] == 0
}
