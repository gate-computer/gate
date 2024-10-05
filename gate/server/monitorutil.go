// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"log/slog"

	"gate.computer/gate/server/event"

	. "import.name/type/context"
)

// MultiMonitor combines multiple event monitors.
func MultiMonitor(monitors ...func(Context, *event.Event, error)) func(Context, *event.Event, error) {
	return func(ctx Context, ev *event.Event, err error) {
		for _, f := range monitors {
			f(ctx, ev, err)
		}
	}
}

// NewLogger creates an event monitor which logs structured messages.  Internal
// errors are logged at error level and other events at info level.
func NewLogger(logger *slog.Logger) func(Context, *event.Event, error) {
	if logger == nil {
		logger = slog.Default()
	}

	return func(ctx Context, ev *event.Event, err error) {
		level := slog.LevelInfo
		if ev.Type == event.TypeFailInternal {
			level = slog.LevelError
		}
		if !logger.Enabled(ctx, level) {
			return
		}
		_ = logger.Handler().Handle(ctx, event.NewRecord(ev, err))
	}
}
