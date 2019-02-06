// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/packet"
)

const maxPacketSize = 65536

const (
	packetReservedOffset = 7
)

type read struct {
	buf packet.Buf
	err error
}

func ioLoop(ctx context.Context, services ServiceRegistry, subject *Process) (err error) {
	var (
		messageInput  = make(chan packet.Buf)
		messageOutput = make(chan packet.Buf)
	)
	discoverer := services.StartServing(ctx, ServiceConfig{maxPacketSize}, messageInput, messageOutput)
	defer close(messageOutput)

	subjectInput := subjectReadLoop(subject.reader)
	defer func() {
		if subjectInput != nil {
			for range subjectInput {
			}
		}
	}()

	subjectOutput := subjectWriteLoop(subject.writer)
	subject.writer = nil
	defer func() {
		if subjectOutput != nil {
			close(subjectOutput)
		}
	}()

	var (
		pendingMsg packet.Buf
		pendingEvs []packet.Buf
	)

	var (
		suspended = subject.suspended
		done      = ctx.Done()
	)
	doSuspend := func() {
		suspended = nil
		done = nil

		subject.killSuspend()

		close(subjectOutput)
		subjectOutput = nil
	}

	for subjectInput != nil || pendingMsg != nil {
		var (
			doEv            packet.Buf
			doMessageInput  <-chan packet.Buf
			doMessageOutput chan<- packet.Buf
			doSubjectInput  <-chan read
			doSubjectOutput chan<- packet.Buf
		)

		if len(pendingEvs) > 0 {
			doEv = pendingEvs[0]
		}

		if pendingMsg != nil {
			doMessageOutput = messageOutput
		}

		if doEv == nil {
			doMessageInput = messageInput
			if pendingMsg == nil {
				doSubjectInput = subjectInput
			}
		} else {
			doSubjectOutput = subjectOutput
		}

		select {
		case p := <-doMessageInput:
			pendingEvs = append(pendingEvs, initMessagePacket(p))

		case read, ok := <-doSubjectInput:
			if !ok {
				subjectInput = nil
				break
			}
			if read.err != nil {
				err = read.err
				return
			}

			msg, ev, opErr := handlePacket(read.buf, discoverer)
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

		case doMessageOutput <- pendingMsg:
			pendingMsg = nil

		case doSubjectOutput <- doEv:
			if len(pendingEvs) > 0 {
				pendingEvs = pendingEvs[1:]
			}

		case <-suspended:
			doSuspend()

		case <-done:
			doSuspend()
		}
	}

	return
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

			size := binary.LittleEndian.Uint32(header)
			if size < packet.HeaderSize || size > maxPacketSize {
				reads <- read{err: badprogram.Errorf("runtime: invalid op packet size: %d", size)}
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

func subjectWriteLoop(w *os.File) chan<- packet.Buf {
	writes := make(chan packet.Buf)

	go func() {
		defer func() {
			for range writes {
			}
		}()

		defer w.Close()

		for buf := range writes {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}()

	return writes
}

func initMessagePacket(p packet.Buf) packet.Buf {
	if len(p) < packet.HeaderSize || len(p) > maxPacketSize {
		panic(errors.New("invalid outgoing message packet buffer length"))
	}

	if p[packetReservedOffset] != 0 {
		panic(errors.New("reserved byte is nonzero in outgoing message packet header"))
	}

	if p.Code() < 0 {
		panic(errors.New("negative service code in outgoing message packet header"))
	}

	if p.Domain() > packet.DomainData {
		panic(errors.New("invalid domain in outgoing message packet header"))
	}

	// Service implementations only need to initialize code and domain fields.
	binary.LittleEndian.PutUint32(p[packet.OffsetSize:], uint32(len(p)))

	return p
}

func handlePacket(p packet.Buf, discoverer ServiceDiscoverer) (msg, reply packet.Buf, err error) {
	if x := p[packetReservedOffset]; x != 0 {
		err = badprogram.Errorf("reserved byte has value 0x%02x in incoming packet header", x)
		return
	}

	switch code := p.Code(); {
	case code >= 0:
		if int(code) >= discoverer.NumServices() {
			err = badprogram.Errorf("invalid service code: %d", code)
			return
		}

		switch domain := p.Domain(); domain {
		case packet.DomainData:
			if n := len(p); n < packet.DataHeaderSize {
				err = badprogram.Errorf("data packet is too short: %d bytes", n)
				return
			}

			fallthrough

		case packet.DomainFlow, packet.DomainCall:
			msg = p

		default:
			err = badprogram.Errorf("invalid domain in incoming packet: %d", domain)
			return
		}

	case code == packet.CodeServices:
		reply, err = handleServicesPacket(p, discoverer)
		if err != nil {
			return
		}

	default:
		err = badprogram.Errorf("invalid code in incoming packet: %d", code)
		return
	}

	return
}
