// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"slices"

	"gate.computer/gate/packet"
)

const maxPacketSize = 65536

var (
	ServiceRegChan = make(chan *Service)
	StreamRegChan  = make(chan *Stream, 1) // Sent from synchronous callbacks.
	sendChan       = make(chan sendEntry)
	writeChan      = make(chan packet.Buf)
	readChan       = make(chan packet.Buf)
	inited         bool
)

type sendEntry struct {
	packet   packet.Buf
	receptor func([]byte)
}

type Service struct {
	// Public fields are initialized by service.Register.
	Name         string
	Code         packet.Code
	InfoReceptor func([]byte)

	state         uint8
	callReceptors []func([]byte)
	streams       map[int32]*Stream
}

type Stream struct {
	// Fields are initialized by service.NewStream.
	Service *Service
	ID      int32
}

func (st *Stream) Write(b []byte) (int, error) {
	n := min(len(b), maxPacketSize-packet.HeaderSize)
	// TODO: check flow
	p := packet.MakeData(st.Service.Code, st.ID, n)
	copy(p.Data(), b)
	SendPacket(packet.Buf(p), nil)
	return n, nil
}

func (st *Stream) Read(b []byte) (int, error) {
	panic("gate: stream read not implemented")
}

func (st *Stream) Close() error {
	slog.Debug("gate: stream close not implemented", "id", st.ID)
	return nil
}

func SendPacket(p packet.Buf, receptor func([]byte)) {
	sendChan <- sendEntry{
		packet:   p,
		receptor: receptor,
	}
}

// Init must be called only by service.Register with mutex held.
func Init() {
	if inited {
		return
	}

	w, r := connect()
	go readPackets(r)
	go writePackets(w)
	go manage()
	inited = true
}

func readPackets(unbuffered io.Reader) {
	r := bufio.NewReader(unbuffered)
	for {
		var size uint32
		if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
			panic(err)
		}
		if size < packet.HeaderSize || size > maxPacketSize {
			panic(size)
		}

		p := make(packet.Buf, packet.Align(int(size)))
		if _, err := io.ReadFull(r, p[4:]); err != nil {
			panic(err)
		}
		p = p[:size]

		slog.Debug("gate: read packet", "size", len(p), "code", p.Code(), "domain", p.Domain(), "index", p.Index())

		readChan <- p
	}
}

func writePackets(w io.Writer) {
	for p := range writeChan {
		slog.Debug("gate: writing packet", "size", len(p), "code", p.Code(), "domain", p.Domain())

		p.SetSize()

		padding := packet.Align(len(p)) - len(p)
		p = append(p, make([]byte, padding)...)

		if _, err := w.Write(p); err != nil {
			panic(err)
		}
	}
}

func manage() {
	var (
		services     []*Service
		servicesSent int
		send         sendEntry
		read         packet.Buf
	)

	for {
		var (
			serviceRegC <-chan *Service
			sendC       <-chan sendEntry
			writeC      chan<- packet.Buf
			writeCode   packet.Code
			writeDomain packet.Domain
			readC       <-chan packet.Buf
		)

		if len(send.packet) == 0 {
			serviceRegC = ServiceRegChan // Only when there is space for services packet.
			sendC = sendChan
		} else {
			writeC = writeChan
			writeCode = send.packet.Code()
			writeDomain = send.packet.Domain()
		}

		if len(read) == 0 {
			readC = readChan
		}

		select {
		case s := <-serviceRegC:
			slog.Debug("gate: registering service", "name", s.Name, "code", s.Code)

			services = slices.Grow(services, int(s.Code)+1-len(services))
			for len(services) <= int(s.Code) {
				services = append(services, nil)
			}
			services[s.Code] = s

			if !slices.Contains(services, nil) {
				// Reg channel is active only when there is no queued send.
				send = sendEntry{
					packet: makeServicesPacket(services[servicesSent:]),
				}
				servicesSent = len(services)
			}

		case st := <-StreamRegChan:
			slog.Debug("gate: registering stream", "code", st.Service.Code, "id", st.ID)

			if st.Service.streams == nil {
				st.Service.streams = make(map[int32]*Stream)
			}
			st.Service.streams[st.ID] = st

		case send = <-sendC:

		case writeC <- send.packet:
			if writeDomain == packet.DomainCall && writeCode >= 0 {
				s := services[writeCode]
				s.callReceptors = append(s.callReceptors, send.receptor)
			}
			send = sendEntry{}

		case read = <-readC:
		}

		if len(read) > 0 {
			slog.Debug("gate: handling packet", "size", len(read), "code", read.Code(), "domain", read.Domain(), "index", read.Index())

			switch {
			case read.Code() >= 0:
				read = handleServicePacket(services[read.Code()], read)
				if len(read) > 0 {
					slog.Debug("gate: packet was not completely handled")
				}

			case read.Code() == packet.CodeServices:
				handleServiceStatePacket(services, read)
				read = packet.Buf{}

			default:
				panic(read.Code())
			}
		}
	}
}

func makeServicesPacket(newServices []*Service) packet.Buf {
	size := packet.HeaderSize + 2
	for _, s := range newServices {
		size += 1 + len(s.Name)
	}

	b := bytes.NewBuffer(packet.Make(packet.CodeServices, packet.DomainCall, size)[:packet.HeaderSize])
	binary.Write(b, binary.LittleEndian, uint16(len(newServices)))
	for _, s := range newServices {
		b.WriteByte(uint8(len(s.Name)))
		b.WriteString(s.Name)
	}

	return packet.Buf(b.Bytes())
}

func handleServiceStatePacket(services []*Service, p packet.Buf) {
	switch p.Domain() {
	case packet.DomainCall, packet.DomainInfo:
		// OK
	default:
		panic(p.Domain())
	}

	if p.Index() != 0 {
		panic(p.Index())
	}

	count := binary.LittleEndian.Uint16(p.Content()[0:])
	states := p.Content()[2:]

	for i := range count {
		services[i].state = states[i]
	}
}

// handleServicePacket returns non-nil if the packet handling needs to be
// retried later.
func handleServicePacket(s *Service, p packet.Buf) packet.Buf {
	switch p.Domain() {
	case packet.DomainCall:
		i := p.Index()
		f := s.callReceptors[i]
		s.callReceptors = append(s.callReceptors[:i], s.callReceptors[i+1:]...)
		f(p.Content())
		return nil

	case packet.DomainInfo:
		if p.Index() != 0 {
			panic(p.Index())
		}
		s.InfoReceptor(p.Content())
		return nil

	case packet.DomainFlow:
		p := packet.FlowBuf(p)

		for i := range p.Len() {
			if f := p.At(i); f.ID >= 0 { // Not done yet?
				st := s.streams[f.ID]

				if st == nil { // Not registered yet?
					slog.Debug("gate: flow for nonregistered stream", "code", p.Code(), "id", f.ID)
					for prev := range i { // Flag previous entries as done.
						f := p.At(prev)
						f.ID = -1
						p.Set(prev, f)
					}
					return packet.Buf(p)
				}

				switch {
				case f.IsIncrement():
					handleStreamFlow(st, f.Value)
				case f.IsEOF():
					handleStreamFlowEOF(st)
				}
			}
		}

		return nil

	case packet.DomainData:
		panic("TODO: handle data packet")

	default:
		panic(p.Domain())
	}
}

func handleStreamFlow(st *Stream, increment int32) {
	slog.Debug("gate: stream flow not implemented", "id", st.ID, "increment", increment)
}

func handleStreamFlowEOF(st *Stream) {
	slog.Debug("gate: stream flow EOF not implemented", "id", st.ID)
}
