// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"time"

	"gate.computer/gate/server/api"
	"google.golang.org/grpc/codes"
)

// RetryAfter creates a TooManyRequests error with the earliest time when the
// request should be retried.
func RetryAfter(t time.Time) api.TooManyRequests {
	return rateLimited{t}
}

type rateLimited struct {
	retryAfter time.Time
}

func (e rateLimited) Error() string         { return e.PublicError() }
func (e rateLimited) PublicError() string   { return "request rate limit exceeded" }
func (e rateLimited) TooManyRequests() bool { return true }
func (e rateLimited) Code() codes.Code      { return codes.Unavailable }

func (e rateLimited) RetryAfter() (d time.Duration) {
	d = time.Until(e.retryAfter)
	if d < 1 {
		d = 1
	}
	return
}
