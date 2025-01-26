// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing

import (
	"gate.computer/gate/server/event"

	. "import.name/type/context"
)

func EventAdder() func(Context, *event.Event, error) {
	return addEvent
}

func addEvent(ctx Context, ev *event.Event, err error) {
	// TODO: add event to span
}
