// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"io"
	"testing"
	"time"

	"gate.computer/gate/server/api"
	"github.com/stretchr/testify/assert"
)

func TestErrorTypes(t *testing.T) {
	assert.Error(t, api.AsUnauthenticated(Unauthenticated("test")))
	assert.Error(t, api.AsPermissionDenied(PermissionDenied("test")))
	assert.Error(t, api.AsUnavailable(Unavailable(io.ErrUnexpectedEOF)))
	assert.Error(t, api.AsTooManyRequests(RetryAfter(time.Now().Add(time.Minute))))
}
