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
	RegChan   = make(chan *Service)
	sendChan  = make(chan sendEntry)
	readChan  = make(chan packet.Buf)
	writeChan = make(chan packet.Buf)
	inited    bool
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
		slog.Debug("gate: reading packet")

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

		readChan <- p[:size]
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
		services []*Service
		send     sendEntry
	)

	for {
		var (
			regC      <-chan *Service
			sendC     <-chan sendEntry
			writeC    chan<- packet.Buf
			writeCode packet.Code
		)
		if len(send.packet) == 0 {
			regC = RegChan // Only when there is space for services packet.
			sendC = sendChan
		} else {
			writeC = writeChan
			writeCode = send.packet.Code()
		}

		select {
		case s := <-regC:
			services = slices.Grow(services, int(s.Code)+1-len(services))
			for len(services) <= int(s.Code) {
				services = append(services, nil)
			}
			services[s.Code] = s

			if !slices.Contains(services, nil) {
				// Reg channel is active only when there is no queued send.
				send = sendEntry{
					packet: makeServicesPacket(services),
				}
			}

		case send = <-sendC:

		case p := <-readChan:
			slog.Debug("gate: handling packet", "size", len(p), "code", p.Code(), "domain", p.Domain(), "index", p.Index())

			switch {
			case p.Code() >= 0:
				handleServicePacket(services[p.Code()], p)
			case p.Code() == packet.CodeServices:
				handleServiceStatePacket(services, p)
			default:
				panic(p.Code())
			}

		case writeC <- send.packet:
			if writeCode >= 0 {
				s := services[writeCode]
				s.callReceptors = append(s.callReceptors, send.receptor)
			}
			send = sendEntry{}
		}
	}
}

func makeServicesPacket(services []*Service) packet.Buf {
	size := packet.HeaderSize + 2
	for _, s := range services {
		size += 1 + len(s.Name)
	}

	b := bytes.NewBuffer(packet.Make(packet.CodeServices, packet.DomainCall, size)[:packet.HeaderSize])
	binary.Write(b, binary.LittleEndian, uint16(len(services)))
	for _, s := range services {
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

func handleServicePacket(s *Service, p packet.Buf) {
	switch p.Domain() {
	case packet.DomainCall:
		i := p.Index()
		f := s.callReceptors[i]
		s.callReceptors = append(s.callReceptors[:i], s.callReceptors[i+1:]...)
		f(p.Content())

	case packet.DomainInfo:
		if p.Index() != 0 {
			panic(p.Index())
		}
		s.InfoReceptor(p.Content())

	case packet.DomainFlow:
		panic("TODO: handle flow packet")

	case packet.DomainData:
		panic("TODO: handle data packet")

	default:
		panic(p.Domain())
	}
}
