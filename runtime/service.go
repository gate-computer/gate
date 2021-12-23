// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"encoding/binary"

	"gate.computer/gate/internal/error/badprogram"
	"gate.computer/gate/packet"
	"gate.computer/gate/snapshot"
)

const maxServices = 256

const serviceStateAvail uint8 = 0x1

var ErrDuplicateService error = badprogram.Errorf("duplicate service")

// ServiceState is used to respond to a service discovery request.
type ServiceState struct {
	flags uint8
}

func (s *ServiceState) SetAvail() {
	s.flags |= serviceStateAvail
}

// ServiceConfig for program instance specific ServiceRegistry invocation.
type ServiceConfig struct {
	MaxSendSize int // Maximum size which the program is prepared to receive.
}

// ServiceRegistry is a collection of configured services.
//
// StartServing is called once for each program instance.  The receive channel
// is closed when the program is being shut down.
//
// config.MaxSendSize may be used when buffering data.
//
// The snapshot buffers must not be mutated, and references to them shouldn't
// be retained for long as they may be parts of a large memory allocation.
//
// The returned channel will deliver up to one error if one occurs after
// initialization.
//
// The service package contains an implementation of this interface.
type ServiceRegistry interface {
	CreateServer(context.Context, ServiceConfig, []snapshot.Service, chan<- packet.Thunk) (InstanceServer, []ServiceState, <-chan error, error)
}

type InstanceServer interface {
	Start(context.Context, chan<- packet.Thunk) error
	Discover(ctx context.Context, newNames []string) (all []ServiceState, err error)
	Handle(context.Context, chan<- packet.Thunk, packet.Buf) error
	Shutdown(context.Context) error
	Suspend(context.Context) ([]snapshot.Service, error)
}

type serviceDiscoverer struct {
	server      InstanceServer
	numServices int
}

func (discoverer *serviceDiscoverer) handlePacket(ctx context.Context, req packet.Buf) (resp packet.Buf, err error) {
	if d := req.Domain(); d != packet.DomainCall {
		err = badprogram.Errorf("service discovery packet has wrong domain: %d", d)
		return
	}

	if n := len(req); n < packet.ServicesHeaderSize {
		err = badprogram.Errorf("service discovery packet is too short: %d bytes", n)
		return
	}

	reqCount := int(binary.LittleEndian.Uint16(req[packet.OffsetServicesCount:]))
	respCount := discoverer.numServices + reqCount
	if respCount > maxServices {
		respCount = maxServices
		reqCount = maxServices - discoverer.numServices
	}

	nameBuf := req[packet.ServicesHeaderSize:]
	names := make([]string, reqCount)

	for i := range names {
		if len(nameBuf) < 1 {
			err = badprogram.Errorf("name data is truncated in service discovery packet")
			return
		}
		nameLen := nameBuf[0]
		nameBuf = nameBuf[1:]
		if nameLen == 0 || nameLen > 127 {
			err = badprogram.Errorf("service name length in discovery packet is out of bounds")
			return
		}
		if len(nameBuf) < int(nameLen) {
			err = badprogram.Errorf("name data is truncated in service discovery packet")
			return
		}
		names[i] = string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen:]
	}

	services, err := discoverer.server.Discover(ctx, names)
	if err != nil {
		return
	}
	discoverer.numServices = len(services)

	resp = makeServicesPacket(packet.DomainCall, services)
	return
}

func (discoverer *serviceDiscoverer) checkPacket(p packet.Buf) (packet.Buf, error) {
	if int(p.Code()) >= discoverer.numServices {
		return nil, badprogram.Errorf("invalid service code in packet: %d", p.Code())
	}

	switch p.Domain() {
	case packet.DomainCall, packet.DomainInfo, packet.DomainFlow:

	case packet.DomainData:
		if n := len(p); n < packet.DataHeaderSize {
			return nil, badprogram.Errorf("data packet is too short: %d bytes", n)
		}

	default:
		return nil, badprogram.Errorf("invalid domain in packet: %d", p.Domain())
	}

	return p, nil
}

func makeServicesPacket(domain packet.Domain, services []ServiceState) (resp packet.Buf) {
	resp = packet.Make(packet.CodeServices, domain, packet.ServicesHeaderSize+len(services))
	resp.SetSize()
	binary.LittleEndian.PutUint16(resp[packet.OffsetServicesCount:], uint16(len(services)))

	for i, s := range services {
		resp[packet.ServicesHeaderSize+i] = s.flags
	}

	return
}
