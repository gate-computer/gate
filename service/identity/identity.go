// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"context"

	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/service"
	"github.com/google/uuid"
)

const (
	serviceName     = "identity"
	serviceRevision = "0"
)

var Service identity

type identity struct{}

func (identity) ServiceName() string               { return serviceName }
func (identity) ServiceRevision() string           { return serviceRevision }
func (identity) Discoverable(context.Context) bool { return true }

func (identity) CreateInstance(ctx context.Context, config service.InstanceConfig,
) service.Instance {
	return newInstance(config)
}

func (identity) RestoreInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte,
) (service.Instance, error) {
	inst := newInstance(config)
	if err := inst.restore(snapshot); err != nil {
		return nil, err
	}

	return inst, nil
}

const (
	flagPending byte = 1 << iota
)

const (
	callNothing byte = iota
	callPrincipalID
	callInstanceID
)

type instance struct {
	code packet.Code

	pending bool
	call    byte
}

func newInstance(config service.InstanceConfig) *instance {
	return &instance{
		code: config.Code,
	}
}

func (inst *instance) restore(snapshot []byte) (err error) {
	if len(snapshot) > 0 {
		inst.pending = (snapshot[0] & flagPending) != 0
		if inst.pending && len(snapshot) > 1 {
			inst.call = snapshot[1]
		}
	}
	return
}

func (inst *instance) Resume(ctx context.Context, send chan<- packet.Buf) {
	if inst.pending {
		inst.handleCall(ctx, send)
	}
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Buf, p packet.Buf) {
	switch dom := p.Domain(); {
	case dom == packet.DomainCall:
		inst.pending = true
		inst.call = callNothing
		if buf := p.Content(); len(buf) > 0 {
			inst.call = buf[0]
		}

		inst.handleCall(ctx, send)

	case dom.IsStream():
		panic("TODO")
	}
}

func (inst *instance) handleCall(ctx context.Context, send chan<- packet.Buf) {
	var id string

	switch inst.call {
	case callPrincipalID:
		if pri := principal.ContextID(ctx); pri != nil {
			id = pri.String()
		}

	case callInstanceID:
		if b, ok := principal.ContextInstanceUUID(ctx); ok {
			id = uuid.Must(uuid.FromBytes(b[:])).String()
		}
	}

	b := []byte(id)
	p := packet.MakeCall(inst.code, len(b))
	copy(p.Content(), b)

	select {
	case send <- p:
		inst.pending = false
		inst.call = callNothing

	case <-ctx.Done():
		return
	}
}

func (inst *instance) Suspend(ctx context.Context) ([]byte, error) {
	if inst.pending {
		return []byte{flagPending, inst.call}, nil
	}

	return nil, nil
}

func (inst *instance) Shutdown(ctx context.Context) error {
	return nil
}
