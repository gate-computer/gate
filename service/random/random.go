// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package random

import (
	"context"
	"crypto/rand"
	"io"
	"math"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
)

const (
	serviceName     = "random"
	serviceRevision = "0"
	wordSize        = 8 // bytes
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

	code      packet.Code
	interval  time.Duration
	sendAfter time.Time // Stale if waiting is non-nil.
	waiting   chan time.Time
}

func newInstance(s Config, i service.InstanceConfig) *instance {
	if s.BitsPerMinute > math.MaxInt32 {
		s.BitsPerMinute = math.MaxInt32
	}
	durationPerWord := wordSize * 8 * float64(time.Minute) / float64(s.BitsPerMinute)

	return &instance{
		code:      i.Code,
		interval:  time.Duration(math.Ceil(durationPerWord)),
		sendAfter: time.Now(),
	}
}

func (inst *instance) restore(snapshot []byte) {
	if len(snapshot) > 0 && snapshot[0] > 0 {
		inst.waiting = make(chan time.Time, 1)
	}
}

func (inst *instance) Start(ctx context.Context, send chan<- packet.Thunk, abort func(error)) error {
	if inst.waiting != nil {
		go inst.wait(ctx, inst.waiting, send, 0)
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

			// Refresh state.
			inst.sendAfter = sentAt.Add(inst.interval)
			inst.waiting = nil

		default:
			// Match extraneous call without providing data.
			return packet.MakeCall(inst.code, 0), nil
		}
	}

	now := time.Now()

	if delay := inst.sendAfter.Sub(now); delay > 0 {
		inst.waiting = make(chan time.Time, 1)
		go inst.wait(ctx, inst.waiting, send, delay)
		return nil, nil
	}

	reply, err := inst.makeReply()
	if err != nil {
		return nil, err
	}
	inst.sendAfter = now.Add(inst.interval)
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

func (inst *instance) wait(ctx context.Context, waited chan<- time.Time, send chan<- packet.Thunk, delay time.Duration) {
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

	select {
	case send <- inst.makeReply:
		sentAt = time.Now()
	case <-ctx.Done():
	}
}

func (inst *instance) makeReply() (packet.Buf, error) {
	p := packet.MakeCall(inst.code, wordSize)
	if _, err := io.ReadFull(rand.Reader, p.Content()); err != nil {
		return nil, err
	}
	return p, nil
}
