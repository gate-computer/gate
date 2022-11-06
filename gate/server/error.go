// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"net/http"
	"time"

	"gate.computer/internal/error/grpc"
	"gate.computer/gate/server/event"
)

// Unauthenticated error.  The reason will be shown to the client.
func Unauthenticated(publicReason string) error {
	return authenticationError(publicReason)
}

type authenticationError string

func (s authenticationError) Error() string            { return string(s) }
func (s authenticationError) PublicError() string      { return string(s) }
func (s authenticationError) Unauthenticated() bool    { return true }
func (s authenticationError) Status() int              { return http.StatusUnauthorized }
func (s authenticationError) GRPCCode() int            { return grpc.Unauthenticated }
func (s authenticationError) FailType() event.FailType { return event.FailAuthDenied }

// PermissionDenied error.  The details are not exposed to the client.
func PermissionDenied(internalDetails string) error {
	return permissionError(internalDetails)
}

type permissionError string

func (s permissionError) Error() string            { return string(s) }
func (s permissionError) PublicError() string      { return "permission denied" }
func (s permissionError) PermissionDenied() bool   { return true }
func (s permissionError) Status() int              { return http.StatusForbidden }
func (s permissionError) GRPCCode() int            { return grpc.PermissionDenied }
func (s permissionError) FailType() event.FailType { return event.FailResourceDenied }

// Unavailable service error.  The details are not exposed to the client.
func Unavailable(internal error) error {
	return availabilityError{internal}
}

type availabilityError struct {
	internal error
}

func (e availabilityError) Unwrap() error       { return e.internal }
func (e availabilityError) Error() string       { return e.internal.Error() }
func (e availabilityError) PublicError() string { return "service unavailable" }
func (e availabilityError) Unavailable() bool   { return true }
func (e availabilityError) Status() int         { return http.StatusServiceUnavailable }
func (e availabilityError) GRPCCode() int       { return grpc.Unavailable }

// RetryAfter creates a TooManyRequests error with the earliest time when the
// request should be retried.
func RetryAfter(t time.Time) error {
	return rateError{t}
}

type rateError struct {
	retryAfter time.Time
}

func (e rateError) Error() string            { return e.PublicError() }
func (e rateError) PublicError() string      { return "request rate limit exceeded" }
func (e rateError) Unavailable() bool        { return true }
func (e rateError) TooManyRequests() bool    { return true }
func (e rateError) Status() int              { return http.StatusTooManyRequests }
func (e rateError) GRPCCode() int            { return grpc.Unavailable }
func (e rateError) FailType() event.FailType { return event.FailRateLimit }

func (e rateError) RetryAfter() time.Duration {
	d := time.Until(e.retryAfter)
	if d < 1 {
		d = 1
	}
	return d
}
