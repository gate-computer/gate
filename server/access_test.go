// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"testing"
)

func TestAccessAuthorizers(*testing.T) {
	var _ AccessAuthorizer = NoAccess{}
	var _ AccessAuthorizer = new(PublicAccess)
}
