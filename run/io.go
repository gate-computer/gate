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
	packetSizeOffset     = 0
	packetFlagsOffset    = 4
	packetReservedOffset = 5

	packetFlagSync  uint8 = 0x1
	packetFlagsMask       = packetFlagSync

	packetCodeNothing  = -1
	packetCodeServices = -2
	packetCodeLoopback = -3
	packetCodeTrap     = -20408
)

var syncPacket packet.Buf

func init() {
	syncPacket = make([]byte, packet.HeaderSize)
	endian.PutUint32(syncPacket[packetSizeOffset:], packet.HeaderSize)
	syncPacket[packetFlagsOffset] = packetFlagSync
	endian.PutUint16(syncPacket[packet.CodeOffset:], 0x10000+packetCodeNothing)
}

type read struct {
	buf packet.Buf
	err error
}

func ioLoop(ctx context.Context, services ServiceRegistry, subject *Process,
) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	// cancel is called below

	var (
		messageInput  = make(chan packet.Buf)
		messageOutput = make(chan packet.Buf)
	)
	discoverer := services.StartServing(ctx, messageOutput, messageInput, maxPacketSize-packet.HeaderSize)
	defer func() {
		for range messageInput {
		}
	}()
	defer close(messageOutput)

	defer cancel() // undo StartServing

	subjectInput := subjectReadLoop(subject.reader)
	defer func() {
		for range subjectInput {
		}
	}()

	subjectOutput, subjectOutputEnd := subjectWriteLoop(subject.writer)
	defer func() {
		<-subjectOutputEnd
	}()
	defer close(subjectOutput)

	defer subject.kill()

	var (
		pendingMsg   packet.Buf
		pendingEvs   []packet.Buf
		pendingSyncs int
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
			if pendingSyncs > 0 {
				doEv[packetFlagsOffset] |= packetFlagSync
			}
		} else if pendingSyncs > 0 {
			doEv = syncPacket
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

			msg, ev, sync, trap, opErr := handlePacket(read.buf, discoverer)
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
			if sync {
				pendingSyncs++
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
			}
			if pendingSyncs > 0 {
				pendingSyncs--
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

func initMessagePacket(p packet.Buf) packet.Buf {
	if len(p) < packet.HeaderSize || len(p) > maxPacketSize {
		panic(errors.New("invalid outgoing message packet buffer length"))
	}

	if p.Code().Int16() < 0 {
		panic(errors.New("negative service code in outgoing message packet header"))
	}

	// service implementations only need to initialize the code field
	endian.PutUint32(p[packetSizeOffset:], uint32(len(p)))
	p[packetFlagsOffset] = 0

	return p
}

func handlePacket(p packet.Buf, discoverer ServiceDiscoverer,
) (msg, reply packet.Buf, sync bool, trap traps.Id, err error) {
	flags := p[packetFlagsOffset]
	if (flags &^ packetFlagsMask) != 0 {
		err = fmt.Errorf("invalid incoming packet flags: 0x%x", flags)
		return
	}

	sync = (flags & packetFlagSync) != 0
	p[packetFlagsOffset] = 0

	code := p.Code().Int16()

	if reserved := p[packetReservedOffset]; reserved != 0 {
		switch t := traps.Id(reserved); t {
		case traps.MissingFunction, traps.Suspended:
			trap = t
		}

		if p.ContentSize() != 0 || flags != 0 || code != packetCodeTrap || trap == 0 {
			err = errors.New("incoming packet is corrupted")
			return
		}

		return
	}

	switch {
	case code >= 0:
		if int(code) >= discoverer.NumServices() {
			err = errors.New("invalid service code")
			return
		}

		msg = p

	case code == packetCodeNothing:

	case code == packetCodeServices:
		reply, err = handleServicesPacket(p, discoverer)
		if err != nil {
			return
		}

	case code == packetCodeLoopback:
		reply = p

	default:
		err = fmt.Errorf("invalid code in incoming packet: %d", code)
		return
	}

	return
}
