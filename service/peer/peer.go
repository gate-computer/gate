// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package peer

import (
	"encoding/binary"
)

const (
	packetHeaderSize     = 8
	peerPacketHeaderSize = packetHeaderSize + 8
)

const (
	opInit = iota
	opMessage
)

const (
	evError = iota
	evMessage
	evAdded
	evRemoved
)

type peer struct {
	group *Group
	code  uint16
	id    uint64
	queue queue
}

func (self *peer) Handle(op []byte, evs chan<- []byte) {
	if self.code == 0 {
		self.code = binary.LittleEndian.Uint16(op[6:])
	}

	if len(op) < peerPacketHeaderSize {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: packet is too short", self.id)
		return
	}

	action := op[packetHeaderSize]

	switch action {
	case opInit:
		self.handleInitOp(evs)

	case opMessage:
		self.handleMessageOp(op)

	default:
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: invalid action: %d", self.id, action)
		return
	}
}

func (self *peer) handleInitOp(evs chan<- []byte) {
	if self.queue.inited() {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: init: already initialized", self.id)
		return
	}
	self.queue.init(&self.group.lock, evs)

	self.group.lock.Lock()
	if self.group.peers == nil {
		self.group.peers = make(map[uint64]*peer)
	}
	for _, other := range self.group.peers {
		other.notify(self, evAdded)
		self.notify(other, evAdded)
	}
	self.group.peers[self.id] = self
	self.group.lock.Unlock()
}

func (self *peer) handleMessageOp(buf []byte) {
	if !self.queue.inited() {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: not initialized", self.id)
		return
	}

	if len(buf) < peerPacketHeaderSize+8 {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: packet is too short", self.id)
		return
	}

	otherId := binary.LittleEndian.Uint64(buf[peerPacketHeaderSize:])

	binary.LittleEndian.PutUint32(buf[packetHeaderSize:], 0)
	buf[packetHeaderSize] = evMessage

	binary.LittleEndian.PutUint64(buf[peerPacketHeaderSize:], self.id)

	self.group.lock.Lock()
	other := self.group.peers[otherId]
	if other != nil {
		other.queue.enqueue(buf, false)
	}
	self.group.lock.Unlock()

	if other == nil {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: no such peer: %d", self.id, otherId)
		return
	}
}

func (self *peer) Shutdown() {
	if self.queue.inited() {
		self.group.lock.Lock()
		self.queue.enqueue(nil, true)
		delete(self.group.peers, self.id)
		for _, other := range self.group.peers {
			other.notify(self, evRemoved)
		}
		self.group.lock.Unlock()

		<-self.queue.stopped
	}
}

func (self *peer) notify(other *peer, evCode byte) {
	ev := make([]byte, peerPacketHeaderSize+8)
	binary.LittleEndian.PutUint16(ev[6:], self.code)
	ev[packetHeaderSize] = evCode
	binary.LittleEndian.PutUint64(ev[peerPacketHeaderSize:], other.id)

	self.queue.enqueue(ev, false)
}
