// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"errors"

	"github.com/tsavola/gate/packet"
)

const (
	serviceCountOffset = 4
	serviceHeaderSize  = 8

	serviceFlagAvailable uint8 = 0x1

	serviceInfoFlagsOffset   = 0
	serviceInfoVersionOffset = 4
	serviceInfoSize          = 8
)

// Service is used to respond to a service discovery request.
type Service struct {
	info [serviceInfoSize]byte
}

// SetAvailable indicates a successful service discovery result.  Semantics of
// the version are service-specific.
func (s *Service) SetAvailable(version int32) {
	s.info[serviceInfoFlagsOffset] = serviceFlagAvailable
	endian.PutUint32(s.info[serviceInfoVersionOffset:], uint32(version))
}

// ServiceRegistry is a collection of fully configured services.
//
// StartServing is called once for each program instance.  The context is
// canceled and the receive channel is closed when the program is being shut
// down.  After that the send channel must be closed.  The maximum packet
// content size may be used when buffering data.
//
// See the service package for the default implementation.
type ServiceRegistry interface {
	StartServing(
		ctx context.Context,
		recv <-chan packet.Buf,
		send chan<- packet.Buf,
		maxContentSize int,
	) ServiceDiscoverer
}

// ServiceDiscoverer is used to look up service information when responding to
// a program's service discovery packet.  It modifies the internal state of the
// ServiceRegistry server.
type ServiceDiscoverer interface {
	Discover(newNames []string) (allServices []Service)
	NumServices() int
}

type noServices struct{}

func (noServices) StartServing(ctx context.Context, recv <-chan packet.Buf, send chan<- packet.Buf, maxContentSize int,
) ServiceDiscoverer {
	go func() {
		defer close(send)

		for range recv {
		}
	}()

	return new(noDiscoveries)
}

type noDiscoveries struct {
	num int
}

func (no *noDiscoveries) Discover(names []string) []Service {
	no.num += len(names)
	return make([]Service, no.num)
}

func (no *noDiscoveries) NumServices() int {
	return no.num
}

func handleServicesPacket(reqPacket packet.Buf, discoverer ServiceDiscoverer,
) (respPacket packet.Buf, err error) {
	reqContent := reqPacket.Content()
	if len(reqContent) < serviceHeaderSize {
		err = errors.New("service discovery packet is too short")
		return
	}

	reqCount := endian.Uint16(reqContent[serviceCountOffset:])
	totalCount := discoverer.NumServices() + int(reqCount)
	if totalCount > maxServices {
		err = errors.New("too many services")
		return
	}

	nameBuf := reqContent[serviceHeaderSize:]
	names := make([]string, reqCount)

	for i := range names {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("name data is truncated in service discovery packet")
			return
		}

		names[i] = string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]
	}

	services := discoverer.Discover(names)

	respContentSize := serviceHeaderSize + serviceInfoSize*totalCount
	respPacket = packet.Make(reqPacket.Code(), respContentSize)
	endian.PutUint32(respPacket[0:], uint32(len(respPacket)))

	respContent := respPacket.Content()
	endian.PutUint16(respContent[serviceCountOffset:], uint16(totalCount))

	infoBuf := respContent[serviceHeaderSize:]

	for _, service := range services {
		copy(infoBuf, service.info[:])
		infoBuf = infoBuf[serviceInfoSize:]
	}

	return
}
