package run

import (
	"encoding/binary"
	"fmt"
	"io"
)

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
	opCodeNone   = opCode(0)
	opCodeOrigin = opCode(1)
)

type opFlags uint16

const (
	opFlagPollout = opFlags(0x1)

	opFlagsMask = opFlagPollout
)

type opPacket struct {
	err     error
	payload []byte
	code    opCode
	flags   opFlags
}

const (
	evCodePollout = uint16(0)
	evCodeOrigin  = uint16(1)
)

func ioLoop(origin io.ReadWriter, subject readWriteKiller) (err error) {
	reads := originLoop(origin)
	defer func() {
		go func() {
			for range reads {
			}
		}()
	}()

	ops := opLoop(subject)
	defer func() {
		for range ops {
		}
	}()

	evs, evDone := evLoop(subject)
	defer func() {
		<-evDone
	}()
	defer close(evs)

	defer subject.kill()

	var ev []byte

	for {
		var (
			doReads <-chan originRead
			doOps   <-chan opPacket
			doEvs   chan<- []byte
		)

		if ev == nil {
			doReads = reads
			doOps = ops
		} else {
			doEvs = evs
		}

		select {
		case read, ok := <-doReads:
			if !ok {
				reads = nil
				break
			}

			err = read.err
			if err != nil {
				return
			}

			ev = read.ev

		case op, ok := <-doOps:
			if !ok {
				return
			}

			ev, err = handleOp(op, origin)
			if err != nil {
				return
			}

		case doEvs <- ev:
			ev = nil

		case <-evDone:
			return
		}
	}
}

func originLoop(r io.Reader) <-chan originRead {
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
						err: err,
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

func opLoop(r io.Reader) <-chan opPacket {
	ops := make(chan opPacket)

	go func() {
		defer close(ops)

		for {
			var header struct {
				Size  uint32
				Code  uint16
				Flags uint16
			}

			err := binary.Read(r, nativeEndian, &header)
			if err != nil {
				if err != io.EOF {
					ops <- opPacket{
						err: fmt.Errorf("op read: %v", err),
					}
				}
				return
			}

			if header.Size < 8 || header.Size > maxPacketSize {
				ops <- opPacket{
					err: fmt.Errorf("invalid op packet size: %d", header.Size),
				}
				return
			}

			payload := make([]byte, header.Size-8)

			_, err = io.ReadFull(r, payload)
			if err != nil {
				ops <- opPacket{
					err: fmt.Errorf("op read: %v", err),
				}
				return
			}

			ops <- opPacket{
				payload: payload,
				code:    opCode(header.Code),
				flags:   opFlags(header.Flags),
			}
		}
	}()

	return ops
}

func evLoop(w io.Writer) (chan<- []byte, <-chan struct{}) {
	evs := make(chan []byte)
	done := make(chan struct{})

	go func() {
		defer close(done)

		for buf := range evs {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}()

	return evs, done
}

func handleOp(op opPacket, origin io.ReadWriter) (ev []byte, err error) {
	err = op.err
	if err != nil {
		err = fmt.Errorf("handleOp: %v", err)
		return
	}

	if (op.flags &^ opFlagsMask) != 0 {
		err = fmt.Errorf("invalid op packet flags: 0x%x", op.flags)
		return
	}

	switch op.code {
	case opCodeNone:

	case opCodeOrigin:
		_, err = origin.Write(op.payload)
		if err != nil {
			err = fmt.Errorf("handleOp: write to origin: %v", err)
			return
		}

	default:
		err = fmt.Errorf("invalid op packet code: %d", op.code)
		return
	}

	if (op.flags & opFlagPollout) != 0 {
		buf := make([]byte, 8)
		nativeEndian.PutUint32(buf[0:], 8)
		nativeEndian.PutUint16(buf[4:], evCodePollout)
		ev = buf
	}

	return
}
