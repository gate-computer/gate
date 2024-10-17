// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracelog

import (
	"io"
	"log/slog"

	"gate.computer/gate/source"
	"gate.computer/gate/trace"
	"gate.computer/internal/traceid"

	. "import.name/type/context"
)

// Source wraps a source with a trace span with logging.
func Source(s source.Source, logger *slog.Logger) source.Source {
	return &loggingSource{s, logger}
}

type loggingSource struct {
	source.Source
	logger *slog.Logger
}

func (s *loggingSource) OpenURI(ctx Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
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

	l := s.logger
	if l == nil {
		l = slog.Default()
	}

	attrs := make([]any, 0, 2)
	attrs = append(attrs, slog.String("trace", traceID.String()))
	attrs = append(attrs, slog.String("span", spanID.String()))
	l = l.With(attrs...)

	if nested {
		l.InfoContext(ctx, "source opening", "uri", uri, "parent", parentID)
	} else {
		l.InfoContext(ctx, "source opening", "uri", uri)
	}

	r, n, err := s.Source.OpenURI(ctx, uri, maxSize)

	switch {
	case err != nil:
		l.InfoContext(ctx, "source error", "uri", uri, "error", err)
	case r == nil && n != 0:
		l.InfoContext(ctx, "source content too long", "uri", uri, "maxsize", maxSize)
	case r == nil && n == 0:
		l.InfoContext(ctx, "source not found", "uri", uri)
	default:
		l.InfoContext(ctx, "source opened", "uri", uri, "length", n)
	}

	return r, n, err
}
