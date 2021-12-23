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

func (identity) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (identity) Discoverable(context.Context) bool {
	return true
}

func (identity) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	return newInstance(config), nil
}

const (
	callNothing byte = iota
	callPrincipalID
	callInstanceID
)

type instance struct {
	service.InstanceBase

	code packet.Code
}

func newInstance(config service.InstanceConfig) *instance {
	return &instance{
		code: config.Code,
	}
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	call := callNothing
	if buf := p.Content(); len(buf) > 0 {
		call = buf[0]
	}

	var id string

	switch call {
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
	p = packet.MakeCall(inst.code, len(b))
	copy(p.Content(), b)
	return p, nil
}
