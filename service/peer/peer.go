// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package peer

import (
	"context"
	"encoding/binary"

	"github.com/tsavola/gate/packet"
)

const (
	peerActionOffset = 0
	peerHeaderSize   = 8

	peerIdOffset      = peerHeaderSize + 0
	peerIdContentSize = peerHeaderSize + 8
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
	code  packet.Code
	id    uint64
	queue queue
}

func (self *peer) Handle(ctx context.Context, op packet.Buf, evs chan<- packet.Buf) {
	content := op.Content()
	if len(content) < peerHeaderSize {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: packet is too short", self.id)
		return
	}

	action := content[peerActionOffset]

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

func (self *peer) handleInitOp(evs chan<- packet.Buf) {
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

func (self *peer) handleMessageOp(buf packet.Buf) {
	if !self.queue.inited() {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: not initialized", self.id)
		return
	}

	content := buf.Content()
	if len(content) < peerIdContentSize {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: packet is too short", self.id)
		return
	}

	otherId := binary.LittleEndian.Uint64(content[peerIdOffset:])

	copy(content, make([]byte, peerHeaderSize))
	content[peerActionOffset] = evMessage

	binary.LittleEndian.PutUint64(content[peerIdOffset:], self.id)

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
	buf := packet.Make(self.code, peerIdContentSize)
	content := buf.Content()
	content[peerActionOffset] = evCode
	binary.LittleEndian.PutUint64(content[peerIdOffset:], other.id)
	self.queue.enqueue(buf, false)
}
