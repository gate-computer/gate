package run

import (
	"fmt"
	"io"
)

type readWriteKiller struct {
	io.Reader
	io.Writer
	kill func() error
}

type read struct {
	buf []byte
	err error
}

type opCode uint16

const (
	opCodeNone = opCode(iota)
	opCodeOrigin
	opCodeServices
	opCodeMessage
)

type opFlags uint16

const (
	opFlagPollout = opFlags(0x1)

	opFlagsMask = opFlagPollout
)

const (
	evCodePollout = uint16(iota)
	evCodeOrigin
	evCodeServices
	evCodeMessage
)

func ioLoop(origin io.ReadWriter, services ServiceRegistry, subject readWriteKiller) (err error) {
	originInput := originReadLoop(origin)
	defer func() {
		go func() {
			for range originInput {
			}
		}()
	}()

	var (
		messageInput  = make(chan []byte)
		messageOutput = make(chan []byte)
		serviceErr    = make(chan error, 1)
	)
	go func() {
		defer close(serviceErr)
		serviceErr <- services.Serve(messageOutput, messageInput)
	}()
	defer func() {
		for range messageInput {
		}
		if err == nil {
			err = <-serviceErr
			if err != nil {
				err = fmt.Errorf("serve: %v", err)
			}
		}
	}()
	defer close(messageOutput)

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
		pendingMsg   []byte
		pendingEvs   [][]byte
		pendingPolls uint64
	)

	for {
		var (
			doEv               []byte
			doOriginInput      <-chan read
			doMessageInput     <-chan []byte
			doMessageOutput    chan<- []byte
			doSubjectInput     <-chan read
			doSubjectOutputEnd <-chan struct{}
			doSubjectOutput    chan<- []byte
		)

		if len(pendingEvs) > 0 {
			doEv = pendingEvs[0]
		} else if pendingPolls > 0 {
			doEv = makePolloutEv(pendingPolls)
		}

		if pendingMsg != nil {
			doMessageOutput = messageOutput
		}

		if doEv == nil {
			doOriginInput = originInput
			doMessageInput = messageInput
			if pendingMsg == nil {
				doSubjectInput = subjectInput
			}
			doSubjectOutputEnd = subjectOutputEnd
		} else {
			doSubjectOutput = subjectOutput
		}

		select {
		case read, ok := <-doOriginInput:
			if !ok {
				originInput = nil
				break
			}
			if read.err != nil {
				err = read.err
				return
			}

			pendingEvs = append(pendingEvs, read.buf)

		case buf := <-doMessageInput:
			pendingEvs = append(pendingEvs, initMessageEv(buf))

		case read, ok := <-doSubjectInput:
			if !ok {
				return
			}
			if read.err != nil {
				err = read.err
				return
			}

			msg, ev, poll, opErr := handleOp(read.buf, origin, services)
			if opErr != nil {
				err = opErr
				return
			}

			if msg != nil {
				pendingMsg = msg
			}
			if ev != nil {
				pendingEvs = append(pendingEvs, ev)
			}
			if poll {
				pendingPolls++
			}

		case doMessageOutput <- pendingMsg:
			pendingMsg = nil

		case <-doSubjectOutputEnd:
			return

		case doSubjectOutput <- doEv:
			if len(pendingEvs) > 0 {
				pendingEvs = pendingEvs[1:]
			} else {
				pendingPolls = 0
			}
		}
	}
}

func originReadLoop(r io.Reader) <-chan read {
	reads := make(chan read)

	go func() {
		defer close(reads)

		for {
			buf := make([]byte, maxPacketSize)
			n, err := r.Read(buf[headerSize:])
			buf = buf[:headerSize+n]
			endian.PutUint32(buf[0:], uint32(len(buf)))
			endian.PutUint16(buf[4:], evCodeOrigin)
			reads <- read{buf: buf}

			if err != nil {
				if err == io.EOF {
					if n != 0 {
						buf := make([]byte, headerSize)
						endian.PutUint32(buf[0:], headerSize)
						endian.PutUint16(buf[4:], evCodeOrigin)
						reads <- read{buf: buf}
					}
				} else {
					reads <- read{err: fmt.Errorf("origin read: %v", err)}
				}
				return
			}
		}
	}()

	return reads
}

func subjectReadLoop(r io.Reader) <-chan read {
	reads := make(chan read)

	go func() {
		defer close(reads)

		header := make([]byte, headerSize)

		for {
			if _, err := io.ReadFull(r, header); err != nil {
				if err != io.EOF {
					reads <- read{err: fmt.Errorf("subject read: %v", err)}
				}
				return
			}

			size := endian.Uint32(header)
			if size < headerSize || size > maxPacketSize {
				reads <- read{err: fmt.Errorf("invalid op packet size: %d", size)}
				return
			}

			buf := make([]byte, size)
			copy(buf, header)

			if _, err := io.ReadFull(r, buf[headerSize:]); err != nil {
				reads <- read{err: fmt.Errorf("subject read: %v", err)}
				return
			}

			reads <- read{buf: buf}
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

func handleOp(op []byte, origin io.ReadWriter, services ServiceRegistry) (msg, ev []byte, poll bool, err error) {
	var (
		code  = opCode(endian.Uint16(op[4:]))
		flags = opFlags(endian.Uint16(op[6:]))
	)

	if (flags &^ opFlagsMask) != 0 {
		err = fmt.Errorf("invalid op packet flags: 0x%x", flags)
		return
	}

	poll = (flags & opFlagPollout) != 0

	switch code {
	case opCodeNone:

	case opCodeOrigin:
		_, err = origin.Write(op[headerSize:])
		if err != nil {
			err = fmt.Errorf("origin write: %v", err)
		}

	case opCodeServices:
		ev, err = handleServicesOp(op, services)

	case opCodeMessage:
		msg, err = handleMessageOp(op)

	default:
		err = fmt.Errorf("invalid op packet code: %d", code)
	}

	return
}

func makePolloutEv(count uint64) (ev []byte) {
	const size = headerSize + 8

	ev = make([]byte, size)
	endian.PutUint32(ev[0:], size)
	endian.PutUint16(ev[4:], evCodePollout)
	endian.PutUint64(ev[8:], count)

	return
}
