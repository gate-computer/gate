// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"encoding/binary"
	"sync"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"

	. "import.name/type/context"
)

const maxRequests = 10 // Cannot be greater than 256.

type instance struct {
	service.InstanceBase

	local *Localhost
	packet.Service

	handlers sync.WaitGroup
	handled  chan<- handled
	unsent   <-chan []packet.Buf
	s        sender
}

func newInstance(local *Localhost, config service.InstanceConfig) *instance {
	inst := &instance{
		local:   local,
		Service: config.Service,
	}
	inst.s.init()
	return inst
}

func (inst *instance) restore(snapshot []byte) error {
	if len(snapshot) > 0 {
		panic("TODO")
	}

	return nil
}

func (inst *instance) Start(ctx Context, send chan<- packet.Thunk, abort func(error)) error {
	c := make(chan handled)
	inst.unsent = inst.s.start(send, c)
	inst.handled = c
	return nil
}

func (inst *instance) Handle(ctx Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	if !inst.s.registerRequest(p) {
		return nil, nil
	}

	inst.handlers.Add(1)
	go func() {
		defer inst.handlers.Done()
		inst.handled <- handle(ctx, inst.local, inst.Service, p)
	}()

	return nil, nil
}

func (inst *instance) Shutdown(ctx Context, suspend bool) ([]byte, error) {
	inst.handlers.Wait()

	if inst.handled != nil {
		close(inst.handled)
		inst.handled = nil
	}

	requests := inst.s.wait()

	var unsent []packet.Buf
	if inst.unsent != nil {
		unsent = <-inst.unsent
		inst.unsent = nil
	}

	if !suspend {
		return nil, nil
	}

	n := binary.MaxVarintLen32 * 2
	for _, p := range requests {
		n += len(p)
	}
	for _, p := range unsent {
		n += len(p)
	}

	b := make([]byte, 0, n)

	b = appendUvarint(b, len(requests))
	for _, p := range requests {
		b = append(b, p...)
	}

	b = appendUvarint(b, len(unsent))
	for _, p := range unsent {
		b = append(b, p...)
	}

	return b, nil
}

type sender struct {
	mu       sync.Mutex
	cond     sync.Cond
	requests []packet.Buf // Nil means not started or shut down.
	sending  bool
}

func (s *sender) init() {
	s.cond.L = &s.mu
}

func (s *sender) start(send chan<- packet.Thunk, handled <-chan handled) <-chan []packet.Buf {
	unsent := make(chan []packet.Buf, 1)

	// Locking not necessary.
	s.requests = []packet.Buf{}
	s.sending = true
	go s.loop(unsent, send, handled)

	return unsent
}

func (s *sender) loop(unsent chan<- []packet.Buf, send chan<- packet.Thunk, handled <-chan handled) {
	var buffered []packet.Buf

	defer func() {
		unsent <- buffered
	}()

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.sending = false
		s.cond.Signal()
	}()

	for {
		var (
			sending  chan<- packet.Thunk
			sendable packet.Buf
		)
		if len(buffered) > 0 {
			sending = send
			sendable = buffered[0]
		}

		select {
		case h, ok := <-handled:
			if !ok {
				return
			}

			index := func() uint8 {
				s.mu.Lock()
				defer s.mu.Unlock()

				// See maxRequests.
				for i, req := range s.requests {
					if &req[0] == &h.req[0] {
						s.requests = append(s.requests[:i], s.requests[i+1:]...)
						return uint8(i)
					}
				}
				panic("request not found")
			}()

			p := h.res
			p.SetIndex(index)
			buffered = append(buffered, p)

		case sending <- sendable.Thunk():
			buffered = buffered[1:]
		}
	}
}

func (s *sender) registerRequest(req packet.Buf) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.requests) >= maxRequests && s.sending {
		s.cond.Wait()
	}
	s.requests = append(s.requests, req)
	return s.sending
}

func (s *sender) wait() (requests []packet.Buf) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.requests != nil && s.sending {
		s.cond.Wait()
	}
	requests = s.requests
	s.requests = nil
	return
}

// appendUvarint without reallocating the underlying array.
func appendUvarint(b []byte, value int) []byte {
	n := binary.PutUvarint(b[len(b):len(b)+binary.MaxVarintLen32], uint64(value))
	return b[:len(b)+n]
}
