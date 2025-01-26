// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"log/slog"
	"net/http"
	"strings"

	"gate.computer/gate/principal"
	"gate.computer/gate/server/api"

	. "import.name/type/context"
)

// HTTPSpanStarter returns a function which can be used as
// webserver.Config.StartSpan callback.  If logger is nil, default logger is
// used.
func HTTPSpanStarter(logger *slog.Logger, prefix string) func(*http.Request, string) (Context, func(Context)) {
	var (
		msgStart = prefix + "request"
		msgEnd   = prefix + "request handled"
	)

	return func(r *http.Request, pattern string) (Context, func(Context)) {
		attrs := make([]any, 0, 9)
		attrs = append(attrs, slog.String("method", r.Method))
		attrs = append(attrs, slog.String("pattern", pattern))
		attrs = append(attrs, slog.String("path", r.URL.Path))
		if strings.Contains(r.RequestURI, "?") {
			attrs = append(attrs, slog.String("query", r.URL.RawQuery))
		}
		attrs = append(attrs, slog.String("proto", r.Proto))
		attrs = append(attrs, slog.String("remote", r.RemoteAddr))

		return startSpan(r.Context(), logger, msgStart, msgEnd, attrs)
	}
}

// SpanStarter returns a function which can be used as server.Config.StartSpan
// callback.  If logger is nil, default logger is used.
func SpanStarter(logger *slog.Logger, prefix string) func(Context, api.Op) (Context, func(Context)) {
	var (
		msgStart = prefix + "op"
		msgEnd   = prefix + "op done"
	)

	return func(ctx Context, op api.Op) (Context, func(Context)) {
		attrs := make([]any, 0, 5)
		attrs = append(attrs, slog.String("op", op.String()))
		if pri := principal.ContextID(ctx); pri != nil {
			attrs = append(attrs, slog.String("principal", pri.String()))
		}

		return startSpan(ctx, logger, msgStart, msgEnd, attrs)
	}
}

func startSpan(ctx Context, logger *slog.Logger, msgStart, msgEnd string, attrs []any) (Context, func(Context)) {
	if logger == nil {
		logger = slog.Default()
	}

	logger.InfoContext(ctx, msgStart)

	return ctx, func(ctx Context) {
		logger.InfoContext(ctx, msgEnd)
	}
}
