// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"io"
	"log/slog"

	"gate.computer/gate/source"

	. "import.name/type/context"
)

// Source wraps a source with logging.  If logger is nil, default logger is
// used.
func Source(s source.Source, logger *slog.Logger) source.Source {
	return &loggingSource{s, logger}
}

type loggingSource struct {
	source.Source
	logger *slog.Logger
}

func (s *loggingSource) OpenURI(ctx Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
	l := s.logger
	if l == nil {
		l = slog.Default()
	}

	l.InfoContext(ctx, "source opening", "uri", uri)

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
