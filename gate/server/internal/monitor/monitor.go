// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitor

import (
	"log/slog"

	"gate.computer/gate/server/event"

	. "import.name/type/context"
)

// Log internal errors using default [slog.Logger].
func LogFailInternal(ctx Context, ev *event.Event, err error) {
	if ev.Type != event.TypeFailInternal {
		return
	}
	logger := slog.Default()
	if !logger.Enabled(ctx, slog.LevelError) {
		return
	}
	_ = logger.Handler().Handle(ctx, event.NewRecord(ev, err))
}
