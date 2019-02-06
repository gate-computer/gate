// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/packet"
)

const maxServices = 256

const serviceStateAvail uint8 = 0x1

// ServiceState is used to respond to a service discovery request.
type ServiceState struct {
	flags uint8
}

func (s *ServiceState) SetAvail() {
	s.flags |= serviceStateAvail
}

// ServiceConfig for program instance specific ServiceRegistry invocation.
type ServiceConfig struct {
	MaxPacketSize int
}

// ServiceRegistry is a collection of configured services.
//
// StartServing is called once for each program instance.  The receive channel
// is closed when the program is being shut down.
//
// The maximum packet content size may be used when buffering data.
//
// The service package contains an implementation of this interface.
type ServiceRegistry interface {
	StartServing(ctx context.Context, config ServiceConfig, send chan<- packet.Buf, recv <-chan packet.Buf) ServiceDiscoverer
}

// ServiceDiscoverer is used to look up service availability when responding to
// a program's service discovery packet.  It modifies the internal state of the
// ServiceRegistry server.
type ServiceDiscoverer interface {
	Discover(newNames []string) (allServices []ServiceState)
	NumServices() int
}

func handleServicesPacket(req packet.Buf, discoverer ServiceDiscoverer) (resp packet.Buf, err error) {
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
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = badprogram.Errorf("name data is truncated in service discovery packet")
			return
		}

		names[i] = string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]
	}

	services := discoverer.Discover(names)

	resp = packet.Make(req.Code(), packet.ServicesHeaderSize+respCount)
	binary.LittleEndian.PutUint32(resp[packet.OffsetSize:], uint32(len(resp)))
	binary.LittleEndian.PutUint16(resp[packet.OffsetServicesCount:], uint16(respCount))

	stateBuf := resp[packet.ServicesHeaderSize:]

	for i, service := range services {
		stateBuf[i] = service.flags
	}

	return
}
