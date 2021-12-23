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
	"io/ioutil"
	"os"

	"gate.computer/gate/internal/error/badprogram"
	"gate.computer/gate/internal/file"
	"gate.computer/gate/packet"
	"gate.computer/gate/snapshot"
)

func popServiceBuffers(frozen *snapshot.Buffers) (services []snapshot.Service) {
	if frozen == nil {
		return
	}

	services = frozen.Services
	frozen.Services = nil
	return
}

func popInputBuffer(frozen *snapshot.Buffers) (input []byte) {
	if frozen == nil {
		return
	}

	input = frozen.Input
	frozen.Input = nil
	return
}

func popOutputBuffer(frozen *snapshot.Buffers) (output []byte) {
	if frozen == nil {
		return
	}

	output = frozen.Output
	frozen.Output = nil
	return
}

type read struct {
	buf packet.Buf
	err error
}

// ioLoop mutates Process and Buffers (if any).
func ioLoop(ctx context.Context, services ServiceRegistry, subject *Process, frozen *snapshot.Buffers) error {
	if frozen == nil {
		subject.writerOut.Unref()
	}

	var (
		dead      = subject.execution.dead
		suspended = subject.suspended
		done      = ctx.Done()
	)

	messageInput := make(chan packet.Thunk)
	server, initialServiceState, serviceDone, err := services.CreateServer(ctx, ServiceConfig{maxPacketSize}, popServiceBuffers(frozen), messageInput)
	if err != nil {
		return err
	}
	defer func() {
		if frozen != nil && err == nil {
			frozen.Services, err = server.Suspend(ctx)
		} else {
			if e := server.Shutdown(ctx); err == nil {
				err = e
			}
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := server.Start(ctx, messageInput); err != nil {
		return err
	}

	discoverer := &serviceDiscoverer{
		server:      server,
		numServices: len(initialServiceState),
	}

	pendingMsgs, initialRead, err := splitBufferedPackets(popOutputBuffer(frozen), discoverer)
	if err != nil {
		return err
	}

	var pendingEvs []packet.Buf
	if ev := popInputBuffer(frozen); len(ev) > 0 {
		pendingEvs = []packet.Buf{ev} // No need to split packets.
	}

	if len(initialServiceState) > 0 {
		pendingEvs = append(pendingEvs, makeServicesPacket(packet.DomainInfo, initialServiceState))
	}
	initialServiceState = nil

	subjectInput := subjectReadLoop(subject.reader, initialRead)
	defer func() {
		subject.reader.Close()
		subject.reader = nil

		for range subjectInput {
		}
	}()

	subjectOutput := subjectWriteLoop(subject.writer)
	subject.writer = nil
	defer func() {
		if subjectOutput != nil {
			close(subjectOutput)
		}
	}()

	for {
		for len(pendingMsgs) > 0 {
			if err := server.Handle(ctx, messageInput, pendingMsgs[0]); err != nil {
				return err
			}
			pendingMsgs = pendingMsgs[1:]
		}

		var nextEv packet.Buf
		if len(pendingEvs) > 0 {
			nextEv = pendingEvs[0]
		}

		var (
			doMessageInput  <-chan packet.Thunk
			doSubjectInput  <-chan read
			doSubjectOutput chan<- packet.Buf
		)
		if nextEv == nil {
			doMessageInput = messageInput
		}
		if nextEv == nil || dead == nil {
			doSubjectInput = subjectInput
		}
		if nextEv != nil {
			doSubjectOutput = subjectOutput
		}

		select {
		case thunk := <-doMessageInput:
			if p := thunk(); len(p) > 0 {
				pendingEvs = append(pendingEvs, initMessagePacket(p))
			}

		case read, ok := <-doSubjectInput:
			if !ok {
				panic("gate runtime process read goroutine panicked")
			}

			if read.err != nil {
				if subjectOutput != nil {
					close(subjectOutput)
					subjectOutput = nil
				}

				if frozen != nil {
					// Messages may be part of the original Buffers.Output
					// array, so don't mutate them.
					for _, msg := range pendingMsgs {
						frozen.Output = append(frozen.Output, msg...)
					}
					frozen.Output = append(frozen.Output, read.buf...)

					frozen.Input, err = ioutil.ReadAll(subject.writerOut.File())
					subject.writerOut.Unref()
					if err != nil {
						return err
					}

					var pendingLen int
					for _, ev := range pendingEvs {
						pendingLen += len(ev)
					}
					if n := len(frozen.Input) + pendingLen; cap(frozen.Input) < n {
						frozen.Input = append(make([]byte, 0, n), frozen.Input...)
					}
					for _, ev := range pendingEvs {
						frozen.Input = append(frozen.Input, ev...)
					}
				}

				if read.err != io.EOF {
					return read.err
				}
				return nil
			}

			msg, ev, opErr := handlePacket(ctx, read.buf, discoverer)
			if opErr != nil {
				return opErr
			}
			if msg != nil {
				pendingMsgs = append(pendingMsgs, msg)
			}
			if ev != nil {
				pendingEvs = append(pendingEvs, ev)
			}

		case doSubjectOutput <- nextEv:
			pendingEvs = pendingEvs[1:]

		case <-dead:
			dead = nil

		case <-suspended:
			suspended = nil
			subject.execution.suspend()

		case <-done:
			done = nil
			subject.execution.suspend()

		case err, ok := <-serviceDone:
			if ok {
				return err
			} else {
				serviceDone = nil
			}
		}
	}
}

func subjectReadLoop(r *os.File, partial []byte) <-chan read {
	reads := make(chan read)

	go func() {
		defer close(reads)

		header := make([]byte, packet.HeaderSize)

		for {
			offset := copy(header, partial)
			partial = partial[offset:]

			if n, err := io.ReadFull(r, header[offset:]); err != nil {
				if err != io.EOF {
					err = fmt.Errorf("subject read: %w", err)
				}
				reads <- read{buf: header[:offset+n], err: err}
				return
			}

			size := binary.LittleEndian.Uint32(header)
			if size < packet.HeaderSize || size > maxPacketSize {
				reads <- read{err: badprogram.Errorf("runtime: invalid op packet size: %d", size)}
				return
			}

			buf := make([]byte, packet.Align(int(size)))
			offset = copy(buf, header)
			offset += copy(buf[offset:], partial)
			partial = nil

			if n, err := io.ReadFull(r, buf[offset:]); err != nil {
				if err != io.EOF {
					err = fmt.Errorf("subject read: %w", err)
				}
				reads <- read{buf: buf[:offset+n], err: err}
				return
			}

			reads <- read{buf: buf[:size]}
		}
	}()

	return reads
}

func subjectWriteLoop(w *file.File) chan<- packet.Buf {
	writes := make(chan packet.Buf)

	go func() {
		defer w.Close()

		var iov [2][]byte
		var pad [packet.Alignment - 1]byte

		for buf := range writes {
			iov[0] = buf

			n := (packet.Alignment - (uint64(len(buf)) & (packet.Alignment - 1))) &^ packet.Alignment
			iov[1] = pad[:n]

			if err := w.WriteVec(iov); err != nil {
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

	if p.Code() < 0 {
		panic(errors.New("negative service code in outgoing message packet header"))
	}

	// Service implementations only need to initialize code and domain fields.
	// But if the size field has been initialized correctly, treat the buffer
	// as read-only.
	if binary.LittleEndian.Uint32(p[packet.OffsetSize:]) != uint32(len(p)) {
		p.SetSize()
	}

	return p
}

func handlePacket(ctx context.Context, p packet.Buf, discoverer *serviceDiscoverer) (msg, reply packet.Buf, err error) {
	switch code := p.Code(); {
	case code >= 0:
		msg, err = discoverer.checkPacket(p)
		if err != nil {
			return
		}

	case code == packet.CodeServices:
		reply, err = discoverer.handlePacket(ctx, p)
		if err != nil {
			return
		}

	default:
		err = badprogram.Errorf("invalid code in incoming packet: %d", code)
		return
	}

	return
}

func splitBufferedPackets(buf []byte, discoverer *serviceDiscoverer) ([]packet.Buf, []byte, error) {
	var msgs []packet.Buf

	for {
		if len(buf) < packet.HeaderSize {
			return msgs, buf, nil
		}

		size := binary.LittleEndian.Uint32(buf[packet.OffsetSize:])
		if size < packet.HeaderSize || size > maxPacketSize {
			return nil, nil, badprogram.Errorf("buffered packet has invalid size: %d", size)
		}

		if uint32(len(buf)) < size {
			return msgs, buf, nil
		}

		p := packet.Buf(buf[:size])

		if code := p.Code(); code < 0 {
			return nil, nil, badprogram.Errorf("invalid code in buffered packet: %d", code)
		}

		p, err := discoverer.checkPacket(p)
		if err != nil {
			return nil, nil, err
		}

		msgs = append(msgs, p)
		buf = buf[size:]
	}
}
