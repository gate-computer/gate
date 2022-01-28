// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"time"
)

// Unauthorized access error.  The client is denied access to the server.
type Unauthorized interface {
	error
	Unauthorized() bool
}

// Forbidden access error.  The client is denied access to a resource.
type Forbidden interface {
	error
	Forbidden() bool
}

// TooManyRequests error occurs when request rate limit has been exceeded.
type TooManyRequests interface {
	error
	TooManyRequests() bool
	RetryAfter() time.Duration // Zero means unknown.
}
