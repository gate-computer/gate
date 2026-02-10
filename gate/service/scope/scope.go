// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scope

import (
	"errors"

	"gate.computer/gate/packet"
	"gate.computer/gate/scope/program"
	"gate.computer/gate/service"

	. "import.name/type/context"
)

const (
	serviceName     = "scope"
	serviceRevision = "0"
)

var Service scope

type scope struct{}

func (scope) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (scope) Discoverable(Context) bool {
	return true
}

func (scope) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	if err := restore(ctx, snapshot); err != nil {
		return nil, err
	}
	return instance{}, nil
}

func restore(ctx Context, snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}

	scope, err := parseScope(snapshot)
	if err != nil {
		return err
	}

	return program.ContextScope(ctx).Restrict(scope)
}

const (
	callRestrict uint8 = iota
)

type instance struct{ service.InstanceBase }

func (instance) Handle(ctx Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	if buf := p.Content(); len(buf) > 0 {
		switch buf[0] {
		case callRestrict:
			return handleRestrict(ctx, p.Code(), buf[1:])
		}
	}

	return packet.MakeCall(p.Code(), 0), nil
}

func handleRestrict(ctx Context, code packet.Code, buf []byte) (packet.Buf, error) {
	scope, err := parseScope(buf)
	if err != nil {
		return nil, err
	}

	if err := program.ContextScope(ctx).Restrict(scope); err != nil {
		return nil, err
	}

	const errorSize = 2 // int16
	return packet.MakeCall(code, errorSize), nil
}

func (instance) Shutdown(ctx Context, suspend bool) ([]byte, error) {
	if !suspend {
		return nil, nil
	}

	scope, restricted := program.ContextScope(ctx).Scope()
	if !restricted {
		return nil, nil
	}

	bufsize := 1
	for _, s := range scope {
		bufsize += 1 + len(s)
	}
	b := make([]byte, 0, bufsize)

	b = append(b, uint8(len(scope)))
	for _, s := range scope {
		b = append(b, uint8(len(s)))
		b = append(b, s...)
	}

	return b, nil
}

var errParseShort = errors.New("scope encoding is too short")

func parseScope(b []byte) ([]string, error) {
	if len(b) < 1 {
		return nil, errParseShort
	}
	count := int(b[0])
	b = b[1:]

	scope := make([]string, count)

	for i := range count {
		if len(b) < 1 {
			return nil, errParseShort
		}
		size := int(b[0])
		b = b[1:]

		if len(b) < size {
			return nil, errParseShort
		}
		scope[i] = string(b[:size])
		b = b[size:]
	}

	return scope, nil
}
