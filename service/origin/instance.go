// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"sync"

	"gate.computer/gate/internal/varint"
	"gate.computer/gate/packet"
	"gate.computer/gate/service"
	"github.com/tsavola/mu"
)

type instance struct {
	service.InstanceBase

	Config
	packet.Service

	send      chan<- packet.Buf // Send packets to the user program.
	wakeup    chan struct{}     // accepting, replying or shutting changed.
	mu        mu.Mutex          // Protects the fields below.
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

func (inst *instance) Start(ctx context.Context, send chan<- packet.Buf) {
	inst.send = send

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
	switch p.Domain() {
	case packet.DomainCall:
		if !inst.mu.GuardBool(func() bool {
			if n := inst.accepting + 1; n > 0 {
				inst.accepting = n
				return true
			}
			return false
		}) {
			log.Print("TODO: too many simultaneous origin accept calls")
			return
		}

		poke(inst.wakeup)

	case packet.DomainFlow:
		p := packet.FlowBuf(p)

		for i := 0; i < p.Num(); i++ {
			id, increment := p.Get(i)

			var s *stream
			inst.mu.Guard(func() {
				s = inst.streams[id]
			})
			if s == nil {
				log.Print("TODO: stream not found")
				return
			}

			if increment != 0 {
				if err := s.Subscribe(increment); err != nil {
					log.Printf("TODO: %v", err)
					return
				}
			} else {
				if err := s.SubscribeEOF(); err != nil {
					log.Printf("TODO: %v", err)
					return
				}
			}
		}

	case packet.DomainData:
		p := packet.DataBuf(p)

		var s *stream
		inst.mu.Guard(func() {
			s = inst.streams[p.ID()]
		})
		if s == nil {
			log.Print("TODO: stream not found")
			return
		}

		if p.DataLen() != 0 {
			if _, err := s.Write(p.Data()); err != nil {
				log.Printf("TODO (%v)", err)
				return
			}
		} else {
			if err := s.WriteEOF(); err != nil {
				log.Printf("TODO (%v)", err)
				return
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
			if inst.mu.GuardBool(func() bool {
				if inst.accepting > 0 && !inst.replying && !inst.shutting {
					for id = 0; inst.streams[id] != nil; id++ {
					}
					s = newStream(inst.BufSize)
					inst.streams[id] = s
					inst.replying = true
				}
				return inst.shutting
			}) {
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
			cancel = inst.mu.GuardBool(func() bool {
				return inst.shutting
			})

		case <-connectorClosed:
			cancel = true

		case <-ctx.Done():
			cancel = true
		}
	}

	inst.mu.Guard(func() {
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

	return func(ctx context.Context, r io.Reader, w io.Writer) error {
		err := s.transfer(ctx, inst.Service, id, r, w, inst.send)

		if !s.Live() {
			inst.mu.Guard(func() {
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
func (inst *instance) drainRestored(ctx context.Context, restored []int32) {
	// If context gets done, it will cause also the remaining transfer calls to
	// exit immediately, so just loop through and collect the states.

	for _, id := range restored {
		var s *stream
		inst.mu.Guard(func() {
			s = inst.streams[id]
		})

		// Errors would be I/O errors, but there is no connection.
		_ = s.transfer(ctx, inst.Service, id, nil, nil, inst.send)

		inst.mu.Guard(func() {
			if !inst.shutting {
				delete(inst.streams, id)
			}
		})
	}
}

func (inst *instance) shutdown() {
	inst.mu.Guard(func() {
		inst.shutting = true
		for inst.replying {
			inst.notify.Wait()
		}
	})

	poke(inst.wakeup)

	for _, s := range inst.streams {
		s.Stop()
	}
	for _, s := range inst.streams {
		<-s.stopped
	}
}

func (inst *instance) Shutdown(ctx context.Context) error {
	inst.shutdown()
	return nil
}

func (inst *instance) Suspend(ctx context.Context) ([]byte, error) {
	inst.shutdown()

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
