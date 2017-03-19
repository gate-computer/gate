package run

import (
	"bytes"
	"errors"
)

const (
	serviceInfoSize    = 8
	servicesHeaderSize = headerSize + 8

	messageHeaderSize = headerSize + 4
)

type ServiceInfo struct {
	Atom    uint32
	Version uint32
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

func handleServicesOp(op []byte, services ServiceRegistry) (ev []byte, err error) {
	if len(op) < servicesHeaderSize {
		err = errors.New("services op: packet is too short")
		return
	}

	count := endian.Uint32(op[headerSize:])
	if count > maxServices {
		err = errors.New("services op: too many services requested")
		return
	}

	size := servicesHeaderSize + 8*count
	ev = make([]byte, size)
	endian.PutUint32(ev[0:], uint32(size))
	endian.PutUint16(ev[4:], evCodeServices)
	endian.PutUint32(ev[headerSize:], count)

	nameBuf := op[servicesHeaderSize:]
	infoBuf := ev[servicesHeaderSize:]

	for i := uint32(0); i < count; i++ {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("services op: name data is truncated")
			return
		}

		name := string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]

		info := services.Info(name)
		endian.PutUint32(infoBuf[0:], info.Atom)
		endian.PutUint32(infoBuf[4:], info.Version)
		infoBuf = infoBuf[serviceInfoSize:]
	}

	return
}

func handleMessageOp(op []byte) (msg []byte, err error) {
	if len(op) < messageHeaderSize {
		err = errors.New("message op: packet is too short")
		return
	}

	// hide packet flags from service implementations
	endian.PutUint16(op[6:], 0)

	msg = op
	return
}

func initMessageEv(ev []byte) []byte {
	if len(ev) < messageHeaderSize || len(ev) > maxPacketSize {
		panic(errors.New("invalid message ev packet buffer length"))
	}

	// service implementations may use packet header as scratch space
	endian.PutUint32(ev[0:], uint32(len(ev)))
	endian.PutUint16(ev[4:], evCodeMessage)
	endian.PutUint16(ev[6:], 0)

	return ev
}
