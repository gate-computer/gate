// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"log"

	"gate.computer/gate/server/event"
)

// defaultMonitor prints internal errors to default log.
func defaultMonitor(ev Event, err error) {
	if ev.EventType() <= int32(event.Type_FAIL_INTERNAL) {
		if err == nil {
			log.Printf("%v  event:%s", ev, ev.EventName())
		} else {
			log.Printf("%v  event:%s  error:%q", ev, ev.EventName(), err.Error())
		}
	}
}

// MultiMonitor combines multiple event monitors.
func MultiMonitor(monitors ...func(Event, error)) func(Event, error) {
	return func(ev Event, err error) {
		for _, f := range monitors {
			f(ev, err)
		}
	}
}

// ErrorEventLogger creates an event monitor which prints log messages.
// Internal errors are printed to errorLog and other events to eventLog.
func ErrorEventLogger(errorLog, eventLog Logger) func(Event, error) {
	return func(ev Event, err error) {
		if ev.EventType() <= int32(event.Type_FAIL_INTERNAL) {
			printToLogger(errorLog, ev, err)
		} else {
			printToLogger(eventLog, ev, err)
		}
	}
}

// ErrorLogger creates an event monitor which prints log messages.  Internal
// errors are printed to errorLog and other events are ignored.
func ErrorLogger(errorLog Logger) func(Event, error) {
	return func(ev Event, err error) {
		if ev.EventType() <= int32(event.Type_FAIL_INTERNAL) {
			printToLogger(errorLog, ev, err)
		}
	}
}

func printToLogger(l Logger, ev Event, err error) {
	if err == nil {
		l.Printf("%v  event:%s", ev, ev.EventName())
	} else {
		l.Printf("%v  event:%s  error:%q", ev, ev.EventName(), err.Error())
	}
}
