// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracelog

import (
	"log/slog"
	"net/http"
	"strings"

	"gate.computer/gate/principal"
	"gate.computer/gate/server/api"
	"gate.computer/gate/trace"
	"gate.computer/internal/traceid"

	. "import.name/type/context"
)

// HTTPSpanStarter returns a function which can be used as
// webserver.Config.StartSpan callback.
func HTTPSpanStarter(logger *slog.Logger, prefix string) func(*http.Request, string, ...*trace.Link) (Context, func(Context)) {
	var (
		msgStart = prefix + "request"
		msgEnd   = prefix + "request handled"
	)

	return func(r *http.Request, pattern string, links ...*trace.Link) (Context, func(Context)) {
		attrs := make([]any, 0, 9)
		attrs = append(attrs, slog.String("method", r.Method))
		attrs = append(attrs, slog.String("pattern", pattern))
		attrs = append(attrs, slog.String("path", r.URL.Path))
		if strings.Contains(r.RequestURI, "?") {
			attrs = append(attrs, slog.String("query", r.URL.RawQuery))
		}
		attrs = append(attrs, slog.String("proto", r.Proto))
		attrs = append(attrs, slog.String("remote", r.RemoteAddr))

		return startSpan(r.Context(), logger, msgStart, msgEnd, attrs, links)
	}
}

// SpanStarter returns a function which can be used as server.Config.StartSpan
// callback.
func SpanStarter(logger *slog.Logger, prefix string) func(Context, api.Op, ...*trace.Link) (Context, func(Context)) {
	var (
		msgStart = prefix + "op"
		msgEnd   = prefix + "op done"
	)

	return func(ctx Context, op api.Op, links ...*trace.Link) (Context, func(Context)) {
		attrs := make([]any, 0, 5)
		attrs = append(attrs, slog.String("op", op.String()))
		if pri := principal.ContextID(ctx); pri != nil {
			attrs = append(attrs, slog.String("principal", pri.String()))
		}

		return startSpan(ctx, logger, msgStart, msgEnd, attrs, links)
	}
}

func startSpan(ctx Context, logger *slog.Logger, msgStart, msgEnd string, attrs []any, links []*trace.Link) (Context, func(Context)) {
	var parentID trace.SpanID

	traceID, nested := trace.ContextTraceID(ctx)
	if nested {
		parentID, nested = trace.ContextSpanID(ctx)
	}
	if !nested {
		traceID = traceid.MakeTraceID()
		ctx = trace.ContextWithTraceID(ctx, traceID)
	}
	spanID := traceid.MakeSpanID()
	ctx = trace.ContextWithSpanID(ctx, spanID)

	if logger == nil {
		logger = slog.Default()
	}
	if !logger.Enabled(ctx, slog.LevelInfo) {
		return ctx, func(Context) {}
	}

	attrs = append(attrs, slog.String("trace", traceID.String()))
	attrs = append(attrs, slog.String("span", spanID.String()))

	// TODO: all links, with span ids
	for _, l := range trace.ContextAutoLinks(ctx) {
		attrs = append(attrs, slog.String("link", l.TraceID.String()))
		break
	}

	logger = logger.With(attrs...)

	if nested {
		logger.InfoContext(ctx, msgStart, slog.String("parent", parentID.String()))
	} else {
		logger.InfoContext(ctx, msgStart)
	}

	return ctx, func(ctx Context) {
		logger.DebugContext(ctx, msgEnd)
	}
}
