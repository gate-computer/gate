package run

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type InterfaceInfo uint64

func MakeInterfaceInfo(atom uint32, version uint32) InterfaceInfo {
	return InterfaceInfo(version)<<32 | InterfaceInfo(atom)
}

type Interfaces interface {
	Info(string) InterfaceInfo
	Message(packetPayload []byte, atom uint32) (interfaceFound bool)
}

type noInterfaces struct{}

func (noInterfaces) Info(string) (info InterfaceInfo) {
	return
}

func (noInterfaces) Message([]byte, uint32) (found bool) {
	return
}

type readWriteKiller struct {
	io.Reader
	io.Writer
	kill func() error
}

type originRead struct {
	err error
	ev  []byte
}

type opCode uint16

const (
	opCodeNone = opCode(iota)
	opCodeOrigin
	opCodeInterfaces
	opCodeMessage
)

type opFlags uint16

const (
	opFlagPollout = opFlags(0x1)

	opFlagsMask = opFlagPollout
)

type subjectRead struct {
	err     error
	code    opCode
	flags   opFlags
	payload []byte
}

const (
	evCodePollout = uint16(iota)
	evCodeOrigin
	evCodeInterfaces
)

func ioLoop(origin io.ReadWriter, subject readWriteKiller, ifaces Interfaces) (err error) {
	if ifaces == nil {
		ifaces = noInterfaces{}
	}

	originInput := originReadLoop(origin)
	defer func() {
		go func() {
			for range originInput {
			}
		}()
	}()

	subjectInput := subjectReadLoop(subject)
	defer func() {
		for range subjectInput {
		}
	}()

	subjectOutput, subjectOutputEnd := subjectWriteLoop(subject)
	defer func() {
		<-subjectOutputEnd
	}()
	defer close(subjectOutput)

	defer subject.kill()

	var (
		pendingEvs   [][]byte
		pendingPolls uint64
	)

	for {
		var (
			doOriginInput   <-chan originRead
			doSubjectInput  <-chan subjectRead
			doSubjectOutput chan<- []byte
		)

		var ev []byte

		if len(pendingEvs) > 0 {
			ev = pendingEvs[0]
		} else if pendingPolls > 0 {
			ev = make([]byte, 16)
			nativeEndian.PutUint32(ev[0:], 16)
			nativeEndian.PutUint16(ev[4:], evCodePollout)
			nativeEndian.PutUint64(ev[8:], pendingPolls)
		}

		if ev == nil {
			doOriginInput = originInput
			doSubjectInput = subjectInput
		} else {
			doSubjectOutput = subjectOutput
		}

		select {
		case read, ok := <-doOriginInput:
			if !ok {
				originInput = nil
				break
			}

			err = read.err
			if err != nil {
				return
			}

			ev = read.ev

		case read, ok := <-doSubjectInput:
			if !ok {
				return
			}
			if read.err != nil {
				err = read.err
				return
			}

			ev, poll, e := handleOp(read, origin, ifaces)
			if e != nil {
				err = e
				return
			}
			if ev != nil {
				pendingEvs = append(pendingEvs, ev)
			}
			if poll {
				pendingPolls++
			}

		case doSubjectOutput <- ev:
			if len(pendingEvs) > 0 {
				pendingEvs = pendingEvs[1:]
			} else {
				pendingPolls = 0
			}

		case <-subjectOutputEnd:
			return
		}
	}
}

func originReadLoop(r io.Reader) <-chan originRead {
	reads := make(chan originRead)

	go func() {
		defer close(reads)

		for {
			buf := make([]byte, maxPacketSize)
			n, err := r.Read(buf[8:])
			buf = buf[:8+n]
			nativeEndian.PutUint32(buf[0:], uint32(len(buf)))
			nativeEndian.PutUint16(buf[4:], evCodeOrigin)

			reads <- originRead{
				ev: buf,
			}

			if err != nil {
				if err != io.EOF {
					reads <- originRead{
						err: fmt.Errorf("origin read: %v", err),
					}
				} else if n != 0 {
					buf = buf[:8]
					nativeEndian.PutUint32(buf[0:], 8)
					nativeEndian.PutUint16(buf[4:], evCodeOrigin)

					reads <- originRead{
						ev: buf,
					}
				}
				return
			}
		}
	}()

	return reads
}

func subjectReadLoop(r io.Reader) <-chan subjectRead {
	reads := make(chan subjectRead)

	go func() {
		defer close(reads)

		for {
			var header struct {
				Size  uint32
				Code  uint16
				Flags uint16
			}

			err := binary.Read(r, nativeEndian, &header)
			if err != nil {
				if err != io.EOF {
					reads <- subjectRead{
						err: fmt.Errorf("subject read: %v", err),
					}
				}
				return
			}

			if header.Size < 8 || header.Size > maxPacketSize {
				reads <- subjectRead{
					err: fmt.Errorf("invalid op packet size: %d", header.Size),
				}
				return
			}

			payload := make([]byte, header.Size-8)

			_, err = io.ReadFull(r, payload)
			if err != nil {
				reads <- subjectRead{
					err: fmt.Errorf("subject read: %v", err),
				}
				return
			}

			reads <- subjectRead{
				code:    opCode(header.Code),
				flags:   opFlags(header.Flags),
				payload: payload,
			}
		}
	}()

	return reads
}

func subjectWriteLoop(w io.Writer) (chan<- []byte, <-chan struct{}) {
	writes := make(chan []byte)
	end := make(chan struct{})

	go func() {
		defer close(end)

		for buf := range writes {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}()

	return writes, end
}

func handleOp(op subjectRead, origin io.ReadWriter, ifaces Interfaces) (ev []byte, poll bool, err error) {
	if (op.flags &^ opFlagsMask) != 0 {
		err = fmt.Errorf("invalid op packet flags: 0x%x", op.flags)
		return
	}

	poll = (op.flags & opFlagPollout) != 0

	switch op.code {
	case opCodeNone:

	case opCodeOrigin:
		_, err = origin.Write(op.payload)
		if err != nil {
			err = fmt.Errorf("origin write: %v", err)
		}

	case opCodeInterfaces:
		ev, err = handleInterfaces(op.payload, ifaces)

	case opCodeMessage:
		err = handleMessage(op.payload, ifaces)

	default:
		err = fmt.Errorf("invalid op packet code: %d", op.code)
	}
	return
}

func handleInterfaces(opPayload []byte, ifaces Interfaces) (ev []byte, err error) {
	if len(opPayload) < 4+4 {
		err = errors.New("interfaces op: packet is too short")
		return
	}

	count := nativeEndian.Uint32(opPayload)
	if count > maxInterfaces {
		err = errors.New("interfaces op: too many interfaces requested")
		return
	}

	evSize := 8 + 4 + 4 + 8*count
	ev = make([]byte, evSize)
	nativeEndian.PutUint32(ev[0:], uint32(evSize))
	nativeEndian.PutUint16(ev[4:], evCodeInterfaces)
	nativeEndian.PutUint32(ev[8:], count)

	nameBuf := opPayload[4+4:]
	infoBuf := ev[8+4+4:]

	for i := uint32(0); i < count; i++ {
		nameLen := bytes.IndexByte(nameBuf, 0)
		if nameLen < 0 {
			err = errors.New("interfaces op: name data is truncated")
			return
		}

		name := string(nameBuf[:nameLen])
		nameBuf = nameBuf[nameLen+1:]

		nativeEndian.PutUint64(infoBuf, uint64(ifaces.Info(name)))
		infoBuf = infoBuf[8:]
	}

	return
}

func handleMessage(payload []byte, ifaces Interfaces) (err error) {
	if len(payload) < 4 {
		err = errors.New("message op: packet is too short")
		return
	}

	atom := nativeEndian.Uint32(payload)

	if atom == 0 || !ifaces.Message(payload, atom) {
		err = errors.New("message op: invalid interface atom")
		return
	}

	return
}
