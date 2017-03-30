package run

import (
	"bytes"
	"errors"
)

const (
	servicePacketHeaderSize = packetHeaderSize + 8
	serviceInfoSize         = 8
)

type ServiceInfo struct {
	Code    uint16
	Version int32
}

type ServiceRegistry interface {
	Info(serviceName string) ServiceInfo
	Serve(ops <-chan []byte, evs chan<- []byte) error
}

type noServices struct{}

func (noServices) Info(string) (info ServiceInfo) {
	return
}

func (noServices) Serve(ops <-chan []byte, evs chan<- []byte) (err error) {
	defer close(evs)
	for range ops {
	}
	return
}

func handleServicesPacket(request []byte, services ServiceRegistry) (response []byte, err error) {
	if len(request) < servicePacketHeaderSize {
		err = errors.New("service discovery packet is too short")
		return
	}

	count := endian.Uint32(request[packetHeaderSize+4:])
	if count > maxServices {
		err = errors.New("too many services requested")
		return
	}

	size := servicePacketHeaderSize + serviceInfoSize*count
	response = make([]byte, size)
	endian.PutUint32(response[0:], uint32(size))
	endian.PutUint32(response[packetHeaderSize+4:], count)

	nameBuf := request[servicePacketHeaderSize:]
	infoBuf := response[servicePacketHeaderSize:]

	for i := uint32(0); i < count; i++ {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("name string is truncated in service discovery packet")
			return
		}

		name := string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]

		info := services.Info(name)
		endian.PutUint16(infoBuf[0:], info.Code)
		endian.PutUint32(infoBuf[4:], uint32(info.Version))
		infoBuf = infoBuf[serviceInfoSize:]
	}

	return
}
