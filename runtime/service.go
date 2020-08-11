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
// The service package contains an implementation of this interface.
type ServiceRegistry interface {
	StartServing(
		ctx context.Context,
		config ServiceConfig,
		snapshots []snapshot.Service,
		send chan<- packet.Buf,
		recv <-chan packet.Buf,
	) (
		ServiceDiscoverer,
		[]ServiceState,
		error,
	)
}

// ServiceDiscoverer is used to look up service availability when responding to
// a program's service discovery packet.  It modifies the internal state of the
// ServiceRegistry server.
type ServiceDiscoverer interface {
	Discover(ctx context.Context, newNames []string) (all []ServiceState, err error)
	NumServices() int
	Suspend(context.Context) (snapshots []snapshot.Service)
	Shutdown(context.Context)
}

func handleServicesPacket(ctx context.Context, req packet.Buf, discoverer ServiceDiscoverer) (resp packet.Buf, err error) {
	if d := req.Domain(); d != packet.DomainCall {
		err = badprogram.Errorf("service discovery packet has wrong domain: %d", d)
		return
	}

	if n := len(req); n < packet.ServicesHeaderSize {
		err = badprogram.Errorf("service discovery packet is too short: %d bytes", n)
		return
	}

	curCount := discoverer.NumServices()
	reqCount := int(binary.LittleEndian.Uint16(req[packet.OffsetServicesCount:]))
	respCount := curCount + reqCount
	if respCount > maxServices {
		respCount = maxServices
		reqCount = maxServices - curCount
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

	services, err := discoverer.Discover(ctx, names)
	if err != nil {
		return
	}

	resp = makeServicesPacket(packet.DomainCall, services)
	return
}

func makeServicesPacket(domain packet.Domain, services []ServiceState) (resp packet.Buf) {
	resp = packet.Make(packet.CodeServices, domain, packet.ServicesHeaderSize+len(services))
	binary.LittleEndian.PutUint32(resp[packet.OffsetSize:], uint32(len(resp)))
	binary.LittleEndian.PutUint16(resp[packet.OffsetServicesCount:], uint16(len(services)))

	for i, s := range services {
		resp[packet.ServicesHeaderSize+i] = s.flags
	}

	return
}
