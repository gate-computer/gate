// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package random

import (
	"context"
	"crypto/rand"
	"math"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
)

const (
	serviceName     = "random"
	serviceRevision = "0"
)

const DefaultBitsPerMinute = 16 * 8 * 60 // 16 bytes per second.

type Config struct {
	BitsPerMinute int
}

var DefaultConfig = Config{
	BitsPerMinute: DefaultBitsPerMinute,
}

type Service struct {
	config Config
}

func New(c *Config) *Service {
	s := new(Service)
	if c != nil {
		s.config = *c
	}
	return s
}

func (s *Service) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (s *Service) Discoverable(context.Context) bool {
	return s.config.BitsPerMinute > 0
}

func (s *Service) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	inst := newInstance(s.config, config)
	inst.restore(snapshot)
	return inst, nil
}

type instance struct {
	service.InstanceBase

	code         packet.Code
	byteInterval time.Duration
	lastSent     time.Time // Unknown if waiting is non-nil.
	waiting      chan time.Time
}

func newInstance(s Config, i service.InstanceConfig) *instance {
	if s.BitsPerMinute > math.MaxInt32 {
		s.BitsPerMinute = math.MaxInt32
	}

	var (
		durationPerByte = 8 * float64(time.Minute) / float64(s.BitsPerMinute)
		byteInterval    = time.Duration(math.Ceil(durationPerByte))
	)

	return &instance{
		code:         i.Code,
		byteInterval: byteInterval,
		lastSent:     time.Now(),
	}
}

func (inst *instance) restore(snapshot []byte) {
	if len(snapshot) > 0 && snapshot[0] > 0 {
		inst.waiting = make(chan time.Time, 1)
	}
}

func (inst *instance) Start(ctx context.Context, send chan<- packet.Thunk, abort func(error)) error {
	if inst.waiting != nil {
		go inst.wait(ctx, inst.waiting, send, 0, 1)
	}

	return nil
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	if inst.waiting != nil {
		select {
		case sentAt := <-inst.waiting:
			if sentAt.IsZero() {
				// Shutdown in progress.  Wait goroutine has already exited.
				// Stash the item back for Shutdown method.
				inst.waiting <- sentAt

				// Match extraneous call.  It will be buffered by runtime.
				return packet.MakeCall(inst.code, 0), nil
			}

			inst.lastSent = sentAt
			inst.waiting = nil

		default:
			// Match extraneous call without providing data.
			return packet.MakeCall(inst.code, 0), nil
		}
	}

	var count int
	if c := p.Content(); len(c) > 0 {
		count = int(c[0])
	}
	if count == 0 {
		return packet.MakeCall(inst.code, 0), nil
	}

	var (
		interval = inst.byteInterval * time.Duration(count)
		sendAt   = inst.lastSent.Add(interval)
		now      = time.Now()
	)

	if delay := sendAt.Sub(now); delay > 0 {
		inst.waiting = make(chan time.Time, 1)
		go inst.wait(ctx, inst.waiting, send, delay, count)
		return nil, nil
	}

	reply, err := inst.makeReply(count)
	if err != nil {
		return nil, err
	}
	inst.lastSent = now
	return reply, nil
}

func (inst *instance) Shutdown(ctx context.Context, suspend bool) ([]byte, error) {
	if suspend && inst.waiting != nil {
		if sentAt := <-inst.waiting; sentAt.IsZero() {
			return []byte{1}, nil
		}
	}
	return nil, nil
}

func (inst *instance) wait(ctx context.Context, waited chan<- time.Time, send chan<- packet.Thunk, delay time.Duration, count int) {
	var sentAt time.Time
	defer func() {
		waited <- sentAt
	}()

	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}

	makeReply := func() (packet.Buf, error) {
		return inst.makeReply(count)
	}

	select {
	case send <- makeReply:
		sentAt = time.Now()
	case <-ctx.Done():
	}
}

func (inst *instance) makeReply(count int) (packet.Buf, error) {
	p := packet.MakeCall(inst.code, count)
	if _, err := rand.Read(p.Content()); err != nil {
		return nil, err
	}
	return p, nil
}
