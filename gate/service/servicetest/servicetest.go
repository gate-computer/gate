// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicetest

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"

	. "import.name/type/context"
)

const Code = packet.Code(123)

// MaxSendSize overrides the maximum size of packets sent by service.  Normally
// it is randomized and logged when creating an instance tester (but remember
// that Go caches test results).  This variable can be set when debugging a
// test failure which might be caused by a particular value.
//
// Don't override this permanently; all service implementations must work with
// arbitrarily large MaxSendSize setting.
var MaxSendSize int

type FactorySpec struct {
	NoStreams          bool
	AlwaysDiscoverable bool
}

func FactoryTest(ctx Context, t *testing.T, factory service.Factory, spec FactorySpec) {
	t.Helper()

	if factory.Properties().Streams == spec.NoStreams {
		t.Error("Streams property mismatch")
	}

	if factory.Discoverable(context.Background()) != spec.AlwaysDiscoverable {
		t.Error("AlwaysDiscoverable mismatch")
	}
	if !factory.Discoverable(ctx) {
		t.Error("not Discoverable within Context")
	}
}

type InstanceSpec struct {
	Snapshot []byte
}

type InstanceTester struct {
	Instance service.Instance
	Sent     chan packet.Thunk // From service to program.
	streams  bool
}

func NewInstanceTester(ctx Context, t *testing.T, factory service.Factory, spec InstanceSpec) *InstanceTester {
	t.Helper()

	maxSendSize := MaxSendSize
	if maxSendSize == 0 {
		// Randomize maximum packet size so that it's often equal or close to
		// the minimum (64 kB), but can also be unusually large (up to 16 MB).
		scale := math.Pow(1+rand.Float64(), 21)
		jitter := (1 + rand.Float64()) * 1000
		maxSendSize = max(65536, 8*int(scale-jitter))
	}
	t.Log("servicetest: MaxSendSize:", maxSendSize)

	instance, err := factory.CreateInstance(ctx, service.InstanceConfig{
		Service: packet.Service{
			MaxSendSize: maxSendSize,
			Code:        Code,
		},
	}, spec.Snapshot)
	if err != nil {
		t.Fatal("CreateInstance:", err)
	}

	if err := instance.Ready(ctx); err != nil {
		t.Fatal("Ready:", err)
	}

	send := make(chan packet.Thunk)
	abort := func(err error) {
		t.Error("abort function not implemented")
	}

	if err := instance.Start(ctx, send, abort); err != nil {
		t.Fatal("Start:", err)
	}

	return &InstanceTester{
		Instance: instance,
		Sent:     send,
		streams:  factory.Properties().Streams,
	}
}

func (inst *InstanceTester) Handle(ctx Context, t *testing.T, p packet.Buf) packet.Buf {
	t.Helper()

	p.SetSize()
	p, err := inst.Instance.Handle(ctx, inst.Sent, p)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Receive an item from the Sent channel.
func (inst *InstanceTester) Receive(ctx Context, t *testing.T) packet.Buf {
	t.Helper()

	select {
	case thunk, ok := <-inst.Sent:
		if !ok {
			t.Fatal("Sent channel closed")
		}
		return inst.ValidateSentPacket(t, thunk)

	case <-ctx.Done():
		t.Fatal("Context is Done:", ctx.Err())
	}

	panic("unreachable")
}

// ValidateSentPacket thunk that was received from the Sent channel.
func (inst *InstanceTester) ValidateSentPacket(t *testing.T, thunk packet.Thunk) packet.Buf {
	t.Helper()

	p, err := thunk()
	if err != nil {
		t.Fatal(err)
	}

	if p.Code() != Code {
		t.Error("received packet code mismatch:", p.Code())
	}

	switch p.Domain() {
	case packet.DomainCall, packet.DomainInfo:
		// OK
	case packet.DomainFlow, packet.DomainData:
		if !inst.streams {
			t.Fatalf("service sent %s packet but Properties.Streams is false", p.Domain())
		}
	default:
		t.Fatalf("service sent packet with invalid domain: %d", p.Domain())
	}

	return p
}

// Shutdown without suspension.
func (inst *InstanceTester) Shutdown(ctx Context, t *testing.T) {
	t.Helper()

	if snapshot, err := inst.Instance.Shutdown(ctx, false); err != nil {
		t.Error(err)
	} else if len(snapshot) > 0 {
		t.Error("Shutdown without suspension returned a snapshot")
	}
}

// Suspend calls Shutdown, expecting a snapshot.
func (inst *InstanceTester) Suspend(ctx Context, t *testing.T) []byte {
	t.Helper()

	snapshot, err := inst.Instance.Shutdown(ctx, true)
	if err != nil {
		t.Error(err)
	} else {
		t.Log("snapshot size:", len(snapshot))
	}
	return snapshot
}
