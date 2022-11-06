// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"gate.computer/gate/server/event"
)

// MultiMonitor combines multiple event monitors.
func MultiMonitor(monitors ...func(*event.Event, error)) func(*event.Event, error) {
	return func(ev *event.Event, err error) {
		for _, f := range monitors {
			f(ev, err)
		}
	}
}

// ErrorEventLogger creates an event monitor which prints log messages.
// Internal errors are printed to errorLog and other events to eventLog.
func ErrorEventLogger(errorLog, eventLog Logger) func(*event.Event, error) {
	return func(ev *event.Event, err error) {
		if ev.Type == event.TypeFailInternal {
			printToLogger(errorLog, ev, err)
		} else {
			printToLogger(eventLog, ev, err)
		}
	}
}

// ErrorLogger creates an event monitor which prints log messages.  Internal
// errors are printed to errorLog and other events are ignored.
func ErrorLogger(errorLog Logger) func(*event.Event, error) {
	return func(ev *event.Event, err error) {
		if ev.Type == event.TypeFailInternal {
			printToLogger(errorLog, ev, err)
		}
	}
}

func printToLogger(l Logger, ev *event.Event, err error) {
	if err == nil {
		l.Printf("%v", ev)
	} else {
		l.Printf("%v  error:%q", ev, err.Error())
	}
}
