// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server/detail"
)

func MultiMonitor(monitors ...Monitor) (combined Monitor) {
	var (
		errors []func(*detail.Position, error)
		events []func(Event, error)
	)

	for _, m := range monitors {
		if m.MonitorError != nil {
			errors = append(errors, m.MonitorError)
		}
		if m.MonitorEvent != nil {
			events = append(events, m.MonitorEvent)
		}
	}

	if len(errors) > 0 {
		combined.MonitorError = MultiError(errors...)
	}
	if len(events) > 0 {
		combined.MonitorEvent = MultiEvent(events...)
	}
	return
}

func MultiError(monitors ...func(*detail.Position, error)) func(*detail.Position, error) {
	return func(p *detail.Position, err error) {
		for _, f := range monitors {
			f(p, err)
		}
	}
}

func MultiEvent(monitors ...func(Event, error)) func(Event, error) {
	return func(ev Event, err error) {
		for _, f := range monitors {
			f(ev, err)
		}
	}
}

func ErrorLogger(l run.Logger) func(*detail.Position, error) {
	return func(p *detail.Position, err error) {
		l.Printf("%v: %v", p, err)
	}
}

func EventLogger(l run.Logger) func(Event, error) {
	return func(ev Event, err error) {
		if err == nil {
			l.Printf("%v", ev)
		} else {
			l.Printf("%v: %v", ev, err)
		}
	}
}
