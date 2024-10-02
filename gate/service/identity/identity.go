// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/service"
	"github.com/google/uuid"

	. "import.name/type/context"
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

func (identity) Discoverable(Context) bool {
	return true
}

func (identity) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	return instance{}, nil
}

const (
	callPrincipalID uint8 = iota
	callInstanceID
)

type instance struct{ service.InstanceBase }

func (instance) Handle(ctx Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	var id string

	if buf := p.Content(); len(buf) > 0 {
		switch buf[0] {
		case callPrincipalID:
			if pri := principal.ContextID(ctx); pri != nil {
				id = pri.String()
			}

		case callInstanceID:
			if b, ok := principal.ContextInstanceUUID(ctx); ok {
				id = uuid.Must(uuid.FromBytes(b[:])).String()
			}
		}
	}

	b := []byte(id)
	p = packet.MakeCall(p.Code(), len(b))
	copy(p.Content(), b)
	return p, nil
}
