package run

import (
	"bytes"
	"errors"
)

type ServiceInfo uint64

func MakeServiceInfo(atom uint32, version uint32) ServiceInfo {
	// this assumes little-endian byte-order
	return ServiceInfo(version)<<32 | ServiceInfo(atom)
}

type Services interface {
	Info(string) ServiceInfo
	Message(packetPayload []byte, atom uint32) (serviceFound bool)
}

type noServices struct{}

func (noServices) Info(string) (info ServiceInfo) {
	return
}

func (noServices) Message([]byte, uint32) (found bool) {
	return
}

func handleServices(opPayload []byte, services Services) (ev []byte, err error) {
	if len(opPayload) < 4+4 {
		err = errors.New("services op: packet is too short")
		return
	}

	count := nativeEndian.Uint32(opPayload)
	if count > maxServices {
		err = errors.New("services op: too many services requested")
		return
	}

	evSize := 8 + 4 + 4 + 8*count
	ev = make([]byte, evSize)
	nativeEndian.PutUint32(ev[0:], uint32(evSize))
	nativeEndian.PutUint16(ev[4:], evCodeServices)
	nativeEndian.PutUint32(ev[8:], count)

	nameBuf := opPayload[4+4:]
	infoBuf := ev[8+4+4:]

	for i := uint32(0); i < count; i++ {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("services op: name data is truncated")
			return
		}

		name := string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]

		nativeEndian.PutUint64(infoBuf, uint64(services.Info(name)))
		infoBuf = infoBuf[8:]
	}

	return
}

func handleMessage(payload []byte, services Services) (err error) {
	if len(payload) < 4 {
		err = errors.New("message op: packet is too short")
		return
	}

	atom := nativeEndian.Uint32(payload)

	if atom == 0 || !services.Message(payload, atom) {
		err = errors.New("message op: invalid service atom")
		return
	}

	return
}
