// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package monitor implements server.Monitor.
package monitor

import (
	"context"

	"gate.computer/gate/server"
	"gate.computer/gate/server/event"
)

const (
	DefaultBufSize = 1360
)

type Config struct {
	BufSize int
}

type Item struct {
	Event server.Event
	Error error
}

type req struct {
	sub       chan<- Item
	subscribe chan<- *State
}

type MonitorState struct {
	current *State
	items   chan Item
	kill    chan struct{}
	reqs    chan req
	subs    map[chan<- Item]struct{}
	done    <-chan struct{}
}

func New(ctx context.Context, config Config) (func(server.Event, error), *MonitorState) {
	bufsize := config.BufSize
	if bufsize == 0 {
		bufsize = DefaultBufSize
	}

	s := &MonitorState{
		items: make(chan Item, bufsize),
		kill:  make(chan struct{}),
		reqs:  make(chan req),
		subs:  make(map[chan<- Item]struct{}),
		done:  ctx.Done(),
	}

	go s.loop()

	return s.monitor, s
}

func (s *MonitorState) Subscribe(ctx context.Context, sub chan<- Item) (snapshot *State, err error) {
	resp := make(chan *State)

	select {
	case s.reqs <- req{sub, resp}:
		select {
		case snapshot = <-resp:
			return

		case <-ctx.Done():
			err = ctx.Err()
			return
		}

	case <-s.done: // monitor
		err = context.Canceled
		return

	case <-ctx.Done(): // caller
		err = ctx.Err()
		return
	}
}

func (s *MonitorState) Unsubscribe(ctx context.Context, sub chan Item) error {
	for {
		select {
		case _, open := <-sub:
			if open {
				// we unclogged loop()
			} else {
				return nil // loop() has killed us prematurely
			}

		case s.reqs <- req{sub, nil}:
			return nil

		case <-s.done: // monitor
			return context.Canceled

		case <-ctx.Done(): // caller
			return ctx.Err()
		}
	}
}

func (s *MonitorState) monitor(ev server.Event, err error) {
	item := Item{Event: ev, Error: err}

	select {
	case s.items <- item:
		// ok

	default:
		select {
		case s.kill <- struct{}{}:
			select {
			case s.items <- item:
				// ok

			case <-s.done:
			}

		case <-s.done:
		}
	}
}

func (s *MonitorState) loop() {
	for {
		select {
		case i := <-s.items:
			if i.Event != nil {
				s.current.update(i.Event)
			}

			var killed []chan<- Item

			for sub := range s.subs {
				select {
				case sub <- i:
					// ok

				case <-s.kill:
					killed = append(killed, sub)
				}
			}

			for _, sub := range killed {
				delete(s.subs, sub)
				close(sub)
			}

		case r := <-s.reqs:
			if r.subscribe != nil {
				s.subs[r.sub] = struct{}{}
				r.subscribe <- s.current
			} else {
				delete(s.subs, r.sub)
				close(r.sub)
			}

		case <-s.done:
			return
		}
	}
}

func (s *State) update(x server.Event) {
	switch x.(type) {
	case *event.ModuleUploadNew, *event.ModuleSourceNew:
		s.ProgramsLoaded++

	case *event.ModuleUploadExist, *event.ModuleSourceExist:
		s.ProgramLinks++

	case *event.InstanceCreateKnown, *event.InstanceCreateStream:
		s.Instances++

	case *event.InstanceDelete:
		s.Instances--
	}
}
