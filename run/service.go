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

	serviceInfoCodeOffset    = 0
	serviceInfoVersionOffset = 4
	serviceInfoSize          = 8
)

// ServiceInfo is used to respond to a service discovery request.  Zero-value
// code means that the service is not available.  Version is service-specific.
type ServiceInfo struct {
	Code    packet.Code
	Version int32
}

// ServiceRegistry is used to look up service information when responding to a
// program's service discovery packet.  When the program sends a packet to one
// of the services, the packet is forwarded to the ServiceRegistry for
// handling.
//
// Serve is called once for each program instance.  The context is canceled and
// the receive channel is closed when the program is being shut down.  After
// that the send channel must be closed.  The maximum packet content size may
// be used when buffering data.
//
// See the service package for the default implementation.
type ServiceRegistry interface {
	Info(serviceName string) ServiceInfo
	Serve(ctx context.Context, r <-chan packet.Buf, s chan<- packet.Buf, maxContentSize int) error
}

type noServices struct{}

func (noServices) Info(string) (info ServiceInfo) {
	return
}

func (noServices) Serve(ctx context.Context, r <-chan packet.Buf, s chan<- packet.Buf, maxContentSize int,
) (err error) {
	defer close(s)
	for range r {
	}
	return
}

func handleServicesPacket(reqPacket packet.Buf, services ServiceRegistry,
) (respPacket packet.Buf, err error) {
	reqContent := reqPacket.Content()
	if len(reqContent) < serviceHeaderSize {
		err = errors.New("service discovery packet is too short")
		return
	}

	reqCountBuf := reqContent[serviceCountOffset : serviceCountOffset+4]
	count := endian.Uint32(reqCountBuf)
	if count > maxServices {
		err = errors.New("too many services requested")
		return
	}

	size := packet.BufSize(serviceHeaderSize + serviceInfoSize*int(count))
	respPacket = packet.Buf(make([]byte, size))
	endian.PutUint32(respPacket[0:], uint32(size))

	respContent := respPacket.Content()
	copy(respContent[serviceCountOffset:], reqCountBuf)

	nameBuf := reqContent[serviceHeaderSize:]
	infoBuf := respContent[serviceHeaderSize:]

	for i := uint32(0); i < count; i++ {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("name string is truncated in service discovery packet")
			return
		}

		name := string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]

		info := services.Info(name)
		copy(infoBuf[serviceInfoCodeOffset:], info.Code[:])
		endian.PutUint32(infoBuf[serviceInfoVersionOffset:], uint32(info.Version))
		infoBuf = infoBuf[serviceInfoSize:]
	}

	return
}
