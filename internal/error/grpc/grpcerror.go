// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"context"
	"errors"

	werrors "gate.computer/wag/errors"
	"google.golang.org/grpc/codes"
)

type codeError interface {
	error
	Code() codes.Code
}

func Code(err error) codes.Code {
	if x := codeError(nil); errors.As(err, &x) {
		if c := x.Code(); c != codes.OK {
			return c
		}
		return codes.Unknown // Defensive measure.
	}

	if x := werrors.ModuleError(nil); errors.As(err, &x) {
		return codes.InvalidArgument
	}

	if x := werrors.ResourceLimit(nil); errors.As(err, &x) {
		return codes.ResourceExhausted
	}

	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}

	return codes.Internal
}
