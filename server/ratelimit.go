// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"time"
)

// TooManyRequests error occurs when request rate limit has been exceeded.
type TooManyRequests interface {
	error
	RetryAfter() time.Duration // Zero means unknown.
}

// RetryAfter creates a TooManyRequests error with the earliest time when the
// request should be retried.
func RetryAfter(t time.Time) TooManyRequests {
	return rateLimited{t}
}

type rateLimited struct {
	retryAfter time.Time
}

func (e rateLimited) Error() string       { return e.PublicError() }
func (e rateLimited) PublicError() string { return "request rate limit exceeded" }

func (e rateLimited) RetryAfter() (d time.Duration) {
	d = time.Until(e.retryAfter)
	if d < 1 {
		d = 1
	}
	return
}
