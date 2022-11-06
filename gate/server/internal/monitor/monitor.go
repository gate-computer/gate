// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitor

import (
	"log"

	"gate.computer/gate/server/event"
)

// Default monitor prints internal errors to default log.
func Default(ev *event.Event, err error) {
	if ev.Type == event.TypeFailInternal {
		if err == nil {
			log.Printf("%v", ev)
		} else {
			log.Printf("%v  error:%q", ev.Type, err.Error())
		}
	}
}
