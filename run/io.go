package run

import (
	"errors"
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

const (
	packetFlagPollout = uint16(0x1)
	packetFlagsMask   = packetFlagPollout
)

func ioLoop(services ServiceRegistry, subject readWriteKiller) (err error) {
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
		pendingPolls int
	)

	for {
		var (
			doEv               []byte
			doMessageInput     <-chan []byte
			doMessageOutput    chan<- []byte
			doSubjectInput     <-chan read
			doSubjectOutputEnd <-chan struct{}
			doSubjectOutput    chan<- []byte
		)

		if len(pendingEvs) > 0 {
			doEv = pendingEvs[0]
		} else if pendingPolls > 0 {
			doEv = makePolloutPacket()
		}

		if pendingMsg != nil {
			doMessageOutput = messageOutput
		}

		if doEv == nil {
			doMessageInput = messageInput
			if pendingMsg == nil {
				doSubjectInput = subjectInput
			}
			doSubjectOutputEnd = subjectOutputEnd
		} else {
			doSubjectOutput = subjectOutput
		}

		select {
		case buf := <-doMessageInput:
			pendingEvs = append(pendingEvs, initMessagePacket(buf))

		case read, ok := <-doSubjectInput:
			if !ok {
				return
			}
			if read.err != nil {
				err = read.err
				return
			}

			msg, ev, poll, opErr := handlePacket(read.buf, services)
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
				pendingPolls--
			}
		}
	}
}

func subjectReadLoop(r io.Reader) <-chan read {
	reads := make(chan read)

	go func() {
		defer close(reads)

		header := make([]byte, packetHeaderSize)

		for {
			if _, err := io.ReadFull(r, header); err != nil {
				if err != io.EOF {
					reads <- read{err: fmt.Errorf("subject read: %v", err)}
				}
				return
			}

			size := endian.Uint32(header)
			if size < packetHeaderSize || size > maxPacketSize {
				reads <- read{err: fmt.Errorf("invalid op packet size: %d", size)}
				return
			}

			buf := make([]byte, size)
			copy(buf, header)

			if _, err := io.ReadFull(r, buf[packetHeaderSize:]); err != nil {
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

func initMessagePacket(buf []byte) []byte {
	if len(buf) < packetHeaderSize || len(buf) > maxPacketSize {
		panic(errors.New("invalid outgoing message packet buffer length"))
	}

	if code := endian.Uint16(buf[6:]); code == 0 {
		panic(errors.New("service code is zero in outgoing message packet header"))
	}

	// service implementations only need to initialize the code field
	endian.PutUint32(buf[0:], uint32(len(buf)))
	endian.PutUint16(buf[4:], 0)

	return buf
}

func handlePacket(buf []byte, services ServiceRegistry) (msg, reply []byte, poll bool, err error) {
	var (
		flags = endian.Uint16(buf[4:])
		code  = endian.Uint16(buf[6:])
	)

	if (flags &^ packetFlagsMask) != 0 {
		err = fmt.Errorf("invalid incoming packet flags: 0x%x", flags)
		return
	}

	poll = (flags & packetFlagPollout) != 0

	if code == 0 {
		if len(buf) > packetHeaderSize {
			reply, err = handleServicesPacket(buf, services)
		}
	} else {
		// hide packet flags from service implementations
		endian.PutUint16(buf[4:], 0)
		msg = buf
	}
	return
}

func makePolloutPacket() (buf []byte) {
	buf = make([]byte, packetHeaderSize)
	endian.PutUint32(buf[0:], packetHeaderSize)
	endian.PutUint16(buf[4:], packetFlagPollout)
	return
}
