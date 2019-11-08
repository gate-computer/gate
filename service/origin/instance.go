// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tsavola/gate/internal/varint"
	"github.com/tsavola/gate/packet"
)

const flagInCall = 1 << 0

type instance struct {
	Config
	packet.Service

	accept chan struct{}     // Pending streams may have changed.
	send   chan<- packet.Buf // Send packets to the user program.
	inCall bool              // User program's call is missing a response.

	mu       sync.Mutex        // Protects the fields below.
	streams  map[int32]*stream // Don't mutate when shutting down.
	pending  []int32           // Don't mutate when shutting down.
	shutting bool              // The user program is being shut down.
}

func makeInstance(config Config) instance {
	return instance{
		Config: config,
		accept: make(chan struct{}, 1),
	}
}

// init is invoked by Connector when the program instance is starting.
func (inst *instance) init(service packet.Service) {
	if inst.streams != nil {
		panic("origin instance reused")
	}

	inst.Service = service
	inst.streams = make(map[int32]*stream)
}

// restore is invoked by Connector when the program instance is being resumed.
func (inst *instance) restore(input []byte) (err error) {
	if len(input) == 0 {
		return
	}

	flags, input, err := varint.Scan(input)
	if err != nil {
		return
	}

	inst.inCall = flags&flagInCall != 0

	numStreams, input, err := varint.Scan(input)
	if err != nil {
		return
	}

	// Length of the input buffer puts a practical limit on stream count.
	// Restored streams consume few resources (they share a single goroutine).
	for i := int32(0); i < numStreams; i++ {
		var id int32

		id, input, err = varint.Scan(input)
		if err != nil {
			return
		}

		s := newStream(inst.BufSize)
		input, err = s.Unmarshal(input, inst.Service)
		if err != nil {
			return
		}

		if _, exist := inst.streams[id]; exist {
			err = errors.New("origin service resumed stream with duplicate id")
			return
		}
		inst.streams[id] = s
	}

	return
}

func (inst *instance) Resume(ctx context.Context, send chan<- packet.Buf) {
	if inst.send == nil {
		inst.send = send
	}

	// All streams at this point are restored ones.
	restored := make([]int32, 0, len(inst.streams))
	for id := range inst.streams {
		restored = append(restored, id)
	}

	go inst.drainRestored(ctx, restored)

	if inst.inCall {
		inst.respondToCall(ctx)
	}
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Buf, p packet.Buf) {
	if inst.send == nil {
		inst.send = send
	}

	switch p.Domain() {
	case packet.DomainCall:
		inst.inCall = true
		inst.respondToCall(ctx)

	case packet.DomainFlow:
		p := packet.FlowBuf(p)

		for i := 0; i < p.Num(); i++ {
			id, readable := p.Get(i)

			inst.mu.Lock()
			s, exist := inst.streams[id]
			if !exist {
				s = newStream(inst.BufSize)
				inst.streams[id] = s
				inst.pending = append(inst.pending, id)
			}
			inst.mu.Unlock()

			if readable > 0 {
				if err := s.Subscribe(readable); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			} else {
				if err := s.SubscribeEOF(); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			}

			if !exist {
				select {
				case inst.accept <- struct{}{}:
				default:
				}
			}
		}

	case packet.DomainData:
		p := packet.DataBuf(p)

		inst.mu.Lock()
		s := inst.streams[p.ID()]
		inst.mu.Unlock()

		if s != nil {
			if p.DataLen() > 0 {
				if _, err := s.Write(p.Data()); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			} else {
				if err := s.WriteEOF(); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			}
		} else {
			panic("TODO")
		}

	default:
		panic("TODO")
	}
}

func (inst *instance) respondToCall(ctx context.Context) {
	select {
	case inst.send <- packet.MakeCall(inst.Code, 0):
		inst.inCall = false

	case <-ctx.Done():
		return
	}
}

func (inst *instance) connect(ctx context.Context, connectorClosed <-chan struct{},
) func(context.Context, io.Reader, io.Writer) error {
	var pend int
	var id int32
	var s *stream

	for s == nil {
		select {
		case <-inst.accept:
			inst.mu.Lock()
			shut := inst.shutting
			pend = len(inst.pending)
			if !shut && pend > 0 {
				id = inst.pending[0]
				s = inst.streams[id]
				inst.pending = inst.pending[1:]
			}
			inst.mu.Unlock()
			if shut {
				return nil
			}

		case <-connectorClosed:
			return nil

		case <-ctx.Done():
			return nil
		}
	}

	// Pay it forward in case there was more.
	if pend > 1 {
		select {
		case inst.accept <- struct{}{}:
		default:
		}
	}

	return func(ctx context.Context, r io.Reader, w io.Writer) (err error) {
		err = s.transfer(ctx, inst.Service, id, r, w, inst.send)

		if !s.Live() {
			inst.mu.Lock()
			if !inst.shutting {
				delete(inst.streams, id)
			}
			inst.mu.Unlock()
		}

		return
	}
}

// drainRestored streams (without associated connections) one after another
// until they are fully closed.
func (inst *instance) drainRestored(ctx context.Context, restored []int32) {
	// If context gets done, it will cause also the remaining transfer calls to
	// exit immediately, so just loop through and collect the states.

	for _, id := range restored {
		inst.mu.Lock()
		s := inst.streams[id]
		inst.mu.Unlock()

		// Errors would be I/O errors, but there is no connection.
		_ = s.transfer(ctx, inst.Service, id, nil, nil, inst.send)

		inst.mu.Lock()
		if !inst.shutting {
			delete(inst.streams, id)
		}
		inst.mu.Unlock()
	}
}

func (inst *instance) Suspend() (output []byte) {
	inst.Shutdown()

	var flags int32
	if inst.inCall {
		flags |= flagInCall
	}

	if flags == 0 && len(inst.streams) == 0 {
		return
	}

	size := 1 // Flags.
	size += varint.Len(int32(len(inst.streams)))

	for id, s := range inst.streams {
		size += varint.Len(id)
		size += s.MarshaledSize()
	}

	output = make([]byte, size)

	b := output
	b = varint.Put(b, flags)
	b = varint.Put(b, int32(len(inst.streams)))

	for id, s := range inst.streams {
		b = varint.Put(b, id)
		b = s.Marshal(b)
	}

	return
}

func (inst *instance) Shutdown() {
	inst.mu.Lock()
	inst.shutting = true
	inst.mu.Unlock()

	for _, s := range inst.streams {
		s.Stop()
	}

	if len(inst.pending) > 0 {
		for _, id := range inst.pending {
			// Any error would be an I/O error, but there is no connection.
			_ = inst.streams[id].transfer(context.Background(), inst.Service, id, nil, nil, inst.send)
		}
	}

	for _, s := range inst.streams {
		<-s.stopped
	}
}
