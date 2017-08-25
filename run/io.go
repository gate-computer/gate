// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/wag/traps"
)

const (
	packetSizeOffset  = 0
	packetFlagsOffset = 4

	packetFlagPollout = uint16(0x1)
	packetFlagTrap    = uint16(0x8000)

	packetFlagsMask = packetFlagPollout | packetFlagTrap
)

type read struct {
	buf packet.Buf
	err error
}

func ioLoop(ctx context.Context, services ServiceRegistry, subject *Process) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	// cancel is called below

	var (
		messageInput  = make(chan packet.Buf)
		messageOutput = make(chan packet.Buf)
		serviceErr    = make(chan error, 1)
	)
	go func() {
		defer close(serviceErr)
		serviceErr <- services.Serve(ctx, messageOutput, messageInput, maxPacketSize-packet.HeaderSize)
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

	defer cancel() // cancel the Serve goroutine

	subjectInput := subjectReadLoop(subject.stdout)
	defer func() {
		for range subjectInput {
		}
	}()

	subjectOutput, subjectOutputEnd := subjectWriteLoop(subject.stdin)
	defer func() {
		<-subjectOutputEnd
	}()
	defer close(subjectOutput)

	defer subject.kill()

	var (
		pendingMsg   packet.Buf
		pendingEvs   []packet.Buf
		pendingPolls int
	)

	for {
		var (
			doEv               packet.Buf
			doMessageInput     <-chan packet.Buf
			doMessageOutput    chan<- packet.Buf
			doSubjectInput     <-chan read
			doSubjectOutputEnd <-chan struct{}
			doSubjectOutput    chan<- packet.Buf
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
		case p := <-doMessageInput:
			pendingEvs = append(pendingEvs, initMessagePacket(p))

		case read, ok := <-doSubjectInput:
			if !ok {
				return
			}
			if read.err != nil {
				err = read.err
				return
			}

			msg, ev, poll, trap, opErr := handlePacket(read.buf, services)
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
			if trap != 0 {
				// TODO
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

		case <-ctx.Done():
			err = ctx.Err()
			return
		}
	}
}

func subjectReadLoop(r *os.File) <-chan read {
	reads := make(chan read)

	go func() {
		defer close(reads)

		header := make([]byte, packet.HeaderSize)

		for {
			if _, err := io.ReadFull(r, header); err != nil {
				if err != io.EOF {
					reads <- read{err: fmt.Errorf("subject read: %v", err)}
				}
				return
			}

			size := endian.Uint32(header)
			if size < packet.HeaderSize || size > maxPacketSize {
				reads <- read{err: fmt.Errorf("invalid op packet size: %d", size)}
				return
			}

			buf := make([]byte, size)
			copy(buf, header)

			if _, err := io.ReadFull(r, buf[packet.HeaderSize:]); err != nil {
				reads <- read{err: fmt.Errorf("subject read: %v", err)}
				return
			}

			reads <- read{buf: buf}
		}
	}()

	return reads
}

func subjectWriteLoop(w *os.File) (chan<- packet.Buf, <-chan struct{}) {
	writes := make(chan packet.Buf)
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

func clearPacketFlags(p packet.Buf) {
	p[packetFlagsOffset+0] = 0
	p[packetFlagsOffset+1] = 0
}

func packetCodeIsZero(p packet.Buf) bool {
	return p[packet.CodeOffset+0] == 0 && p[packet.CodeOffset+1] == 0
}

func initMessagePacket(p packet.Buf) packet.Buf {
	if len(p) < packet.HeaderSize || len(p) > maxPacketSize {
		panic(errors.New("invalid outgoing message packet buffer length"))
	}

	if packetCodeIsZero(p) {
		panic(errors.New("service code is zero in outgoing message packet header"))
	}

	// service implementations only need to initialize the code field
	endian.PutUint32(p[packetSizeOffset:], uint32(len(p)))
	clearPacketFlags(p)

	return p
}

func handlePacket(p packet.Buf, services ServiceRegistry) (msg, reply packet.Buf, poll bool, trap traps.Id, err error) {
	flags := endian.Uint16(p[packetFlagsOffset:])
	if (flags &^ packetFlagsMask) != 0 {
		err = fmt.Errorf("invalid incoming packet flags: 0x%x", flags)
		return
	}

	if flags&packetFlagTrap != 0 {
		if flags != packetFlagTrap {
			err = fmt.Errorf("excess incoming packet flags: 0x%x", flags)
			return
		}

		code := endian.Uint16(p[packet.CodeOffset:])

		switch t := traps.Id(code); t {
		case traps.MissingFunction, traps.Suspended:
			trap = t

		default:
			err = fmt.Errorf("invalid incoming packet trap: %d", code)
		}
		return
	}

	poll = (flags & packetFlagPollout) != 0

	if packetCodeIsZero(p) {
		if len(p) > packet.HeaderSize {
			reply, err = handleServicesPacket(p, services)
		}
	} else {
		// hide packet flags from service implementations
		clearPacketFlags(p)
		msg = p
	}
	return
}

func makePolloutPacket() (p packet.Buf) {
	p = make([]byte, packet.HeaderSize)
	endian.PutUint32(p[packetSizeOffset:], packet.HeaderSize)
	endian.PutUint16(p[packetFlagsOffset:], packetFlagPollout)
	return
}
