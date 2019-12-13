// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tsavola/gate/internal/varint"
	"github.com/tsavola/gate/packet"
)

type instance struct {
	Config
	packet.Service

	send      chan<- packet.Buf // Send packets to the user program.
	wakeup    chan struct{}     // accepting, replying or shutting changed.
	mu        sync.Mutex        // Protects the fields below.
	streams   map[int32]*stream
	accepting int32
	replying  bool
	shutting  bool
	notify    sync.Cond // replying unset while shutting.
}

func makeInstance(config Config) (inst instance) {
	inst = instance{
		Config:  config,
		Service: packet.Service{Code: -1},
		wakeup:  make(chan struct{}, 1),
		streams: make(map[int32]*stream),
	}
	inst.notify.L = &inst.mu
	return
}

// init is invoked by Connector when the program instance is starting.
func (inst *instance) init(service packet.Service) {
	if inst.Service.Code >= 0 {
		panic("origin instance reused")
	}
	inst.Service = service
}

// restore is invoked by Connector when the program instance is being resumed.
func (inst *instance) restore(input []byte) (err error) {
	if len(input) == 0 {
		return
	}

	inst.accepting, input, err = varint.Scan(input)
	if err != nil {
		return
	}
	if inst.accepting < 0 {
		// TODO
	}
	if inst.accepting > 0 {
		poke(inst.wakeup)
	}

	if len(input) == 0 {
		return
	}

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
	if len(inst.streams) > 0 {
		restored := make([]int32, 0, len(inst.streams))
		for id := range inst.streams {
			restored = append(restored, id)
		}

		go inst.drainRestored(ctx, restored)
	}
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Buf, p packet.Buf) {
	if inst.send == nil {
		inst.send = send
	}

	switch p.Domain() {
	case packet.DomainCall:
		var ok bool

		inst.mu.Lock()
		if n := inst.accepting + 1; n > 0 {
			inst.accepting = n
			ok = true
		}
		inst.mu.Unlock()
		if !ok {
			panic("TODO: too many simultaneous origin accept calls")
		}

		poke(inst.wakeup)

	case packet.DomainFlow:
		p := packet.FlowBuf(p)

		for i := 0; i < p.Num(); i++ {
			id, increment := p.Get(i)

			inst.mu.Lock()
			s := inst.streams[id]
			inst.mu.Unlock()
			if s == nil {
				panic("TODO: stream not found")
			}

			if increment != 0 {
				if err := s.Subscribe(increment); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			} else {
				if err := s.SubscribeEOF(); err != nil {
					panic(fmt.Errorf("TODO (%v)", err))
				}
			}
		}

	case packet.DomainData:
		p := packet.DataBuf(p)

		inst.mu.Lock()
		s := inst.streams[p.ID()]
		inst.mu.Unlock()
		if s == nil {
			panic("TODO: stream not found")
		}

		if p.DataLen() != 0 {
			if _, err := s.Write(p.Data()); err != nil {
				panic(fmt.Errorf("TODO (%v)", err))
			}
		} else {
			if err := s.WriteEOF(); err != nil {
				panic(fmt.Errorf("TODO (%v)", err))
			}
		}
	}
}

func (inst *instance) connect(ctx context.Context, connectorClosed <-chan struct{},
) func(context.Context, io.Reader, io.Writer) error {
	var (
		id int32
		s  *stream
	)

	for s == nil {
		select {
		case <-inst.wakeup:
			inst.mu.Lock()
			shut := inst.shutting
			if inst.accepting > 0 && !inst.replying && !shut {
				for id = 0; inst.streams[id] != nil; id++ {
				}
				s = newStream(inst.BufSize)
				inst.streams[id] = s
				inst.replying = true
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

	reply := packet.MakeCall(inst.Code, 8)
	binary.LittleEndian.PutUint32(reply.Content(), uint32(id))

	var cancel bool

	for reply != nil && !cancel {
		select {
		case inst.send <- reply:
			reply = nil

		case <-inst.wakeup:
			inst.mu.Lock()
			cancel = inst.shutting
			inst.mu.Unlock()

		case <-connectorClosed:
			cancel = true

		case <-ctx.Done():
			cancel = true
		}
	}

	inst.mu.Lock()
	if cancel {
		delete(inst.streams, id)
	} else {
		inst.accepting--
	}
	inst.replying = false
	if inst.shutting {
		inst.notify.Broadcast()
	}
	inst.mu.Unlock()

	poke(inst.wakeup)

	if cancel {
		return nil
	}

	return func(ctx context.Context, r io.Reader, w io.Writer) error {
		err := s.transfer(ctx, inst.Service, id, r, w, inst.send)

		if !s.Live() {
			inst.mu.Lock()
			if !inst.shutting {
				delete(inst.streams, id)
			}
			inst.mu.Unlock()
		}

		return err
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

	numStreams := int32(len(inst.streams))
	if inst.accepting == 0 && numStreams == 0 {
		return
	}

	size := varint.Len(inst.accepting)
	if numStreams > 0 {
		size += varint.Len(numStreams)
		for id, s := range inst.streams {
			size += varint.Len(id)
			size += s.MarshaledSize()
		}
	}

	output = make([]byte, size)
	b := varint.Put(output, inst.accepting)
	if numStreams > 0 {
		b = varint.Put(b, numStreams)
		for id, s := range inst.streams {
			b = varint.Put(b, id)
			b = s.Marshal(b)
		}
	}

	return
}

func (inst *instance) Shutdown() {
	inst.mu.Lock()
	inst.shutting = true
	for inst.replying {
		inst.notify.Wait()
	}
	inst.mu.Unlock()

	poke(inst.wakeup)

	for _, s := range inst.streams {
		s.Stop()
	}
	for _, s := range inst.streams {
		<-s.stopped
	}
}
