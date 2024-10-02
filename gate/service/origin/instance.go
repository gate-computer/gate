// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"gate.computer/gate/packet"
	"gate.computer/gate/packet/packetio"
	"gate.computer/gate/service"
	"gate.computer/internal/varint"
	"import.name/lock"

	. "import.name/type/context"
)

type instance struct {
	service.InstanceBase

	Config
	packet.Service

	send      chan<- packet.Thunk // Send packets to the user program.
	wakeup    chan struct{}       // accepting, replying or shutting changed.
	mu        sync.Mutex          // Protects the fields below.
	streams   map[int32]*stream
	accepting int32
	replying  bool
	shutting  bool
	notify    sync.Cond // replying unset while shutting.
}

func makeInstance(config Config) instance {
	return instance{
		Config:  config,
		Service: packet.Service{Code: -1},
		wakeup:  make(chan struct{}, 1),
		streams: make(map[int32]*stream),
	}
}

// init is invoked by Connector when the program instance is starting.
func (inst *instance) init(service packet.Service) {
	if inst.Service.Code >= 0 {
		panic("origin instance reused")
	}
	inst.Service = service
	inst.notify.L = &inst.mu // instance has its final location by now.
}

// restore is invoked by Connector when the program instance is being resumed.
func (inst *instance) restore(input []byte) error {
	if len(input) == 0 {
		return nil
	}

	accepting, input, err := varint.Scan(input)
	if err != nil {
		return err
	}
	inst.accepting = accepting
	// TODO: if inst.accepting < 0
	if inst.accepting > 0 {
		poke(inst.wakeup)
	}

	if len(input) == 0 {
		return nil
	}

	numStreams, input, err := varint.Scan(input)
	if err != nil {
		return err
	}

	// Length of the input buffer puts a practical limit on stream count.
	// Restored streams consume few resources (they share a single goroutine).
	for i := int32(0); i < numStreams; i++ {
		var id int32

		id, input, err = varint.Scan(input)
		if err != nil {
			return err
		}

		s := newStream(inst.BufSize)
		input, err = s.Unmarshal(input, inst.Service)
		if err != nil {
			return err
		}

		if _, exist := inst.streams[id]; exist {
			return errors.New("origin service resumed stream with duplicate id")
		}
		inst.streams[id] = s
	}

	return nil
}

func (inst *instance) Start(ctx Context, send chan<- packet.Thunk, abort func(error)) error {
	inst.send = send

	// All streams at this point are restored ones.
	if len(inst.streams) > 0 {
		restored := make([]int32, 0, len(inst.streams))
		for id := range inst.streams {
			restored = append(restored, id)
		}

		go inst.drainRestored(ctx, restored)
	}

	return nil
}

func (inst *instance) Handle(ctx Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	switch p.Domain() {
	case packet.DomainCall:
		var ok bool
		lock.Guard(&inst.mu, func() {
			if n := inst.accepting + 1; n > 0 {
				inst.accepting = n
				ok = true
			}
		})
		if !ok {
			return nil, errors.New("TODO: too many simultaneous origin accept calls")
		}

		poke(inst.wakeup)

	case packet.DomainFlow:
		p := packet.FlowBuf(p)

		for i := 0; i < p.Len(); i++ {
			flow := p.At(i)

			var s *stream
			lock.Guard(&inst.mu, func() {
				s = inst.streams[flow.ID]
			})
			if s == nil {
				return nil, errors.New("TODO: stream not found")
			}

			if _, ok := flow.Note(); ok {
				// TODO
			} else if err := packetio.Subscribe(s, flow.Value); err != nil {
				return nil, fmt.Errorf("TODO: %v", err)
			}
		}

	case packet.DomainData:
		p := packet.DataBuf(p)

		var s *stream
		lock.Guard(&inst.mu, func() {
			s = inst.streams[p.ID()]
		})
		if s == nil {
			return nil, errors.New("TODO: stream not found")
		}

		if p.DataLen() != 0 {
			if _, err := s.Write(p.Data()); err != nil {
				return nil, fmt.Errorf("TODO (%v)", err)
			}
		} else {
			if err := s.CloseWrite(); err != nil {
				return nil, fmt.Errorf("TODO (%v)", err)
			}
		}
	}

	return nil, nil
}

func (inst *instance) connect(ctx Context, connectorClosed <-chan struct{}) func(Context, io.Reader, io.WriteCloser) error {
	var (
		id int32
		s  *stream
	)

	for s == nil {
		select {
		case <-inst.wakeup:
			var ok bool
			lock.Guard(&inst.mu, func() {
				if inst.accepting > 0 && !inst.replying && !inst.shutting {
					for id = 0; inst.streams[id] != nil; id++ {
					}
					s = newStream(inst.BufSize)
					inst.streams[id] = s
					inst.replying = true
				}
				ok = inst.shutting
			})
			if ok {
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
		case inst.send <- reply.Thunk():
			reply = nil

		case <-inst.wakeup:
			lock.Guard(&inst.mu, func() {
				cancel = inst.shutting
			})

		case <-connectorClosed:
			cancel = true

		case <-ctx.Done():
			cancel = true
		}
	}

	lock.Guard(&inst.mu, func() {
		if cancel {
			delete(inst.streams, id)
		} else {
			inst.accepting--
		}
		inst.replying = false
		if inst.shutting {
			inst.notify.Broadcast()
		}
	})

	poke(inst.wakeup)

	if cancel {
		return nil
	}

	return func(ctx Context, r io.Reader, w io.WriteCloser) error {
		err := s.transfer(ctx, inst.Service, id, r, w, inst.send)

		if !s.Live() {
			lock.Guard(&inst.mu, func() {
				if !inst.shutting {
					delete(inst.streams, id)
				}
			})
		}

		return err
	}
}

// drainRestored streams (without associated connections) one after another
// until they are fully closed.
func (inst *instance) drainRestored(ctx Context, restored []int32) {
	// If context gets done, it will cause also the remaining transfer calls to
	// exit immediately, so just loop through and collect the states.

	for _, id := range restored {
		var s *stream
		lock.Guard(&inst.mu, func() {
			s = inst.streams[id]
		})

		// Errors would be I/O errors, but there is no connection.
		_ = s.transfer(ctx, inst.Service, id, nil, nil, inst.send)

		lock.Guard(&inst.mu, func() {
			if !inst.shutting {
				delete(inst.streams, id)
			}
		})
	}
}

func (inst *instance) Shutdown(ctx Context, suspend bool) ([]byte, error) {
	lock.Guard(&inst.mu, func() {
		inst.shutting = true
		for inst.replying {
			inst.notify.Wait()
		}
	})

	poke(inst.wakeup)

	for _, s := range inst.streams {
		s.StopTransfer()
	}
	for _, s := range inst.streams {
		<-s.stopped
	}

	if !suspend {
		return nil, nil
	}

	numStreams := int32(len(inst.streams))
	if inst.accepting == 0 && numStreams == 0 {
		return nil, nil
	}

	size := varint.Len(inst.accepting)
	if numStreams > 0 {
		size += varint.Len(numStreams)
		for id, s := range inst.streams {
			size += varint.Len(id)
			size += s.MarshaledSize()
		}
	}

	output := make([]byte, size)
	b := varint.Put(output, inst.accepting)
	if numStreams > 0 {
		b = varint.Put(b, numStreams)
		for id, s := range inst.streams {
			b = varint.Put(b, id)
			b = s.Marshal(b)
		}
	}

	return output, nil
}
