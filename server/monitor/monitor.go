// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package monitor implements server.Monitor.
package monitor

import (
	"context"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
)

const (
	DefaultBufSize = 1360
)

type Config struct {
	BufSize int
}

type Item struct {
	Position *detail.Position
	Event    server.Event
	Err      error
}

type req struct {
	sub       chan<- Item
	subscribe chan<- State
}

type MonitorState struct {
	current State
	items   chan Item
	kill    chan struct{}
	reqs    chan req
	subs    map[chan<- Item]struct{}
	done    <-chan struct{}
}

func New(ctx context.Context, config *Config) (m server.Monitor, s *MonitorState) {
	bufsize := config.BufSize
	if bufsize == 0 {
		bufsize = DefaultBufSize
	}

	s = &MonitorState{
		items: make(chan Item, bufsize),
		kill:  make(chan struct{}),
		reqs:  make(chan req),
		subs:  make(map[chan<- Item]struct{}),
		done:  ctx.Done(),
	}

	m.MonitorError = s.monitorError
	m.MonitorEvent = s.monitorEvent

	go s.loop()

	return
}

func (s *MonitorState) Subscribe(ctx context.Context, sub chan<- Item) (snapshot State, err error) {
	resp := make(chan State)

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

func (s *MonitorState) monitorError(p *detail.Position, err error) {
	s.monitorItem(Item{Position: p, Err: err})
}

func (s *MonitorState) monitorEvent(ev server.Event, err error) {
	s.monitorItem(Item{Event: ev, Err: err})
}

func (s *MonitorState) monitorItem(i Item) {
	select {
	case s.items <- i:
		// ok

	default:
		select {
		case s.kill <- struct{}{}:
			select {
			case s.items <- i:
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
	case *event.ProgramLoad:
		s.ProgramsLoaded++

	case *event.ProgramCreate:
		s.ProgramLinks++

	case *event.InstanceCreate:
		s.Instances++

	case *event.InstanceDelete:
		s.Instances--
	}
}
