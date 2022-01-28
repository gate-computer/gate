// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"time"
)

// PublicError with separate level of detail for clients and internal logging.
type PublicError interface {
	error
	PublicError() string
}

// AsPublicError returns the error if it is public (PublicError method returns
// non-empty string).
func AsPublicError(err error) PublicError {
	var e PublicError
	if errors.As(err, &e) && e.PublicError() != "" {
		return e
	}
	return nil
}

// Unauthenticated error.  The client doesn't have authentication credentials
// or they are invalid.
type Unauthenticated interface {
	PublicError
	Unauthenticated() bool
}

// AsUnauthenticated returns the error if it is an authentication error
// (Unauthenticated method returns true).
func AsUnauthenticated(err error) Unauthenticated {
	var e Unauthenticated
	if errors.As(err, &e) && e.Unauthenticated() {
		return e
	}
	return nil
}

// PermissionDenied error.  The client is denied access to a resource.
type PermissionDenied interface {
	PublicError
	PermissionDenied() bool
}

// AsPermissionDenied returns the error if it is an authorization error
// (PermissionDenied method returns true).
func AsPermissionDenied(err error) PermissionDenied {
	var e PermissionDenied
	if errors.As(err, &e) && e.PermissionDenied() {
		return e
	}
	return nil
}

// Unavailable error.  Service is not available for the client at the moment.
type Unavailable interface {
	PublicError
	Unavailable() bool
}

// AsUnavailable returns the error if it is an availability error (Unavailable
// method returns true).
func AsUnavailable(err error) Unavailable {
	var e Unavailable
	if errors.As(err, &e) && e.Unavailable() {
		return e
	}
	return nil
}

// TooManyRequests error occurs when request rate limit has been exceeded.
type TooManyRequests interface {
	Unavailable
	TooManyRequests() bool
	RetryAfter() time.Duration // Zero means unknown.
}

// AsTooManyRequests returns the error if it is a rate limit error
// (TooManyRequests method returns true).
func AsTooManyRequests(err error) TooManyRequests {
	var e TooManyRequests
	if errors.As(err, &e) && e.TooManyRequests() {
		return e
	}
	return nil
}

// NotFound error.
type NotFound interface {
	PublicError
	NotFound() bool
}

// AsNotFound returns the error if it is an existence error (NotFound method
// returns true).
func AsNotFound(err error) NotFound {
	var e NotFound
	if errors.As(err, &e) && e.NotFound() {
		return e
	}
	return nil
}

// ModuleNotFound error.
type ModuleNotFound interface {
	NotFound
	ModuleNotFound() bool
}

// AsModuleNotFound returns the error if it is an existence error
// (ModuleNotFound method returns true).
func AsModuleNotFound(err error) ModuleNotFound {
	var e ModuleNotFound
	if errors.As(err, &e) && e.ModuleNotFound() {
		return e
	}
	return nil
}

// InstanceNotFound error.
type InstanceNotFound interface {
	NotFound
	InstanceNotFound() bool
}

// AsInstanceNotFound returns the error if it is an existence error
// (InstanceNotFound method returns true).
func AsInstanceNotFound(err error) InstanceNotFound {
	var e InstanceNotFound
	if errors.As(err, &e) && e.InstanceNotFound() {
		return e
	}
	return nil
}

// FunctionNotFound error.
type FunctionNotFound interface {
	NotFound
	FunctionNotFound() bool
}

// AsFunctionNotFound returns the error if it is an existence error
// (FunctionNotFound method returns true).
func AsFunctionNotFound(err error) FunctionNotFound {
	var e FunctionNotFound
	if errors.As(err, &e) && e.FunctionNotFound() {
		return e
	}
	return nil
}
