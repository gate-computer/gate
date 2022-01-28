// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"
	"testing"
	"time"

	"gate.computer/gate/server/api"
)

func TestErrorTypes(t *testing.T) {
	if err := Unauthenticated("test"); api.AsUnauthenticated(err) == nil {
		t.Error(err)
	}

	if err := PermissionDenied("test"); api.AsPermissionDenied(err) == nil {
		t.Error(err)
	}

	if err := Unavailable(io.ErrUnexpectedEOF); api.AsUnavailable(err) == nil {
		t.Error(err)
	}

	if err := RetryAfter(time.Now().Add(time.Minute)); api.AsTooManyRequests(err) == nil {
		t.Error(err)
	}
}
