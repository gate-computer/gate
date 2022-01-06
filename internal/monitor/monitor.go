// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitor

import (
	"log"

	"gate.computer/gate/server/event"
	"google.golang.org/protobuf/proto"
)

type Event interface {
	EventName() string
	EventType() int32
	proto.Message
}

// Default monitor prints internal errors to default log.
func Default(ev Event, err error) {
	if ev.EventType() <= int32(event.Type_FAIL_INTERNAL) {
		if err == nil {
			log.Printf("%v  event:%s", ev, ev.EventName())
		} else {
			log.Printf("%v  event:%s  error:%q", ev, ev.EventName(), err.Error())
		}
	}
}
