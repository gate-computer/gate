// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logger

import (
	"log/slog"

	"gate.computer/internal/service"

	. "import.name/type/context"
)

// MustContextual returns a logger, or panics if called outside of extension or
// service initialization.  Service initialization should retain the logger for
// service instances.
func MustContextual(ctx Context) *slog.Logger {
	if l := service.ContextLogger(ctx); l != nil {
		return l
	}
	panic("service logger not found within context")
}
