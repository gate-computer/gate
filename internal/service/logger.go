// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"log/slog"

	. "import.name/type/context"
)

type loggerKey struct{}

func ContextWithLogger(ctx Context, l *slog.Logger) Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// ContextLogger may return nil.
func ContextLogger(ctx Context) *slog.Logger {
	l, _ := ctx.Value(loggerKey{}).(*slog.Logger)
	return l
}
