package peer

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/tsavola/gate/service"
)

const (
	Name    = "peer"
	Version = 0

	headerSize            = 8
	messageHeaderSize     = headerSize + 4
	peerMessageHeaderSize = messageHeaderSize + 4
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

var (
	DefaultGroup = new(Group)

	prevId uint64 // atomic
)

func Register(r *service.Registry) {
	service.Register(r, Name, Version, DefaultGroup)
}

type Logger interface {
	Printf(string, ...interface{})
}

type Group struct {
	Log Logger

	lock  sync.Mutex
	peers map[uint64]*peer
}

func (g *Group) New() service.Instance {
	return &peer{
		group: g,
		id:    atomic.AddUint64(&prevId, 1),
	}
}

type peer struct {
	group *Group
	atom  uint32
	id    uint64
	queue queue
}

func (p *peer) Handle(op []byte, evs chan<- []byte) {
	if p.atom == 0 {
		p.atom = binary.LittleEndian.Uint32(op[headerSize:])
	}

	if len(op) < peerMessageHeaderSize {
		// TODO: send error message ev
		p.group.Log.Printf("peer %d: packet is too short", p.id)
		return
	}

	action := op[messageHeaderSize]

	switch action {
	case opInit:
		p.handleInitOp(evs)

	case opMessage:
		p.handleMessageOp(op)

	default:
		// TODO: send error message ev
		p.group.Log.Printf("peer %d: invalid action: %d", p.id, action)
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

	if len(buf) < peerMessageHeaderSize+8 {
		// TODO: send error message ev
		self.group.Log.Printf("peer %d: message: packet is too short", self.id)
		return
	}

	otherId := binary.LittleEndian.Uint64(buf[peerMessageHeaderSize:])

	binary.LittleEndian.PutUint32(buf[messageHeaderSize:], 0)
	buf[messageHeaderSize] = evMessage

	binary.LittleEndian.PutUint64(buf[peerMessageHeaderSize:], self.id)

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

func (self *peer) notify(other *peer, code byte) {
	ev := make([]byte, peerMessageHeaderSize+8)
	binary.LittleEndian.PutUint32(ev[headerSize:], self.atom)
	ev[messageHeaderSize] = code
	binary.LittleEndian.PutUint64(ev[peerMessageHeaderSize:], other.id)

	self.queue.enqueue(ev, false)
}

type queue struct {
	buffer   [][]byte
	shutdown bool
	wakeup   chan struct{}
	stopped  chan struct{}
	sink     chan<- []byte
}

func (q *queue) inited() bool {
	return q.wakeup != nil
}

func (q *queue) init(lock sync.Locker, sink chan<- []byte) {
	q.wakeup = make(chan struct{}, 1)
	q.stopped = make(chan struct{})
	q.sink = sink
	go q.loop(lock)
}

func (q *queue) enqueue(item []byte, shutdown bool) {
	if shutdown {
		q.shutdown = true
	} else {
		q.buffer = append(q.buffer, item)
	}

	select {
	case q.wakeup <- struct{}{}:
	default:
	}
}

func (q *queue) loop(lock sync.Locker) {
	defer close(q.stopped)

	var item []byte

	for {
		lock.Lock()
		if item == nil && len(q.buffer) > 0 {
			item = q.buffer[0]
			q.buffer = q.buffer[1:]
		}
		shutdown := q.shutdown
		lock.Unlock()

		if shutdown {
			break
		}

		var doSink chan<- []byte

		if item != nil {
			doSink = q.sink
		}

		select {
		case <-q.wakeup:
			// ok

		case doSink <- item:
			item = nil
		}
	}
}
