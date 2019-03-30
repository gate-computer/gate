// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
)

const (
	ServiceName = "origin"

	DefaultMaxConns = 3

	egressBufSize = 32768
	smallReadSize = 1536
)

type Config struct {
	MaxConns int
}

var DefaultConfig = Config{DefaultMaxConns}

type Connector struct {
	Config

	closed   chan struct{}
	lock     sync.Mutex
	newConns chan *ioConn
	cond     sync.Cond
	connects int
}

func New(config *Config) (cr *Connector) {
	cr = &Connector{
		closed:   make(chan struct{}),
		newConns: make(chan *ioConn, 1),
	}
	cr.cond.L = &cr.lock

	if config != nil {
		cr.Config = *config
	}
	if cr.MaxConns <= 0 {
		cr.MaxConns = DefaultMaxConns
	}
	return
}

func (cr *Connector) Connect(ctx context.Context) func(context.Context, io.Reader, io.Writer) error {
	conn, putConns := func() (conn *ioConn, putConns chan<- *ioConn) {
		cr.lock.Lock()
		defer cr.lock.Unlock()

		putConns = cr.newConns
		if putConns == nil {
			return
		}

		cr.connects++
		conn = newIOConn(cr.closed)
		return
	}()
	if conn == nil {
		return nil
	}
	defer func() {
		cr.lock.Lock()
		defer cr.lock.Unlock()

		cr.connects--
		cr.cond.Signal()
	}()

	// log.Printf("origin %p connect", conn)

	select {
	case putConns <- conn:
		return conn.io

	case <-cr.closed:
		return nil

	case <-ctx.Done():
		return nil
	}
}

func (cr *Connector) Close() (err error) {
	close(cr.closed)

	cr.lock.Lock()
	defer cr.lock.Unlock()

	for cr.connects > 0 {
		cr.cond.Wait()
	}

	close(cr.newConns)
	for range cr.newConns {
	}
	cr.newConns = nil
	return
}

func (cr *Connector) ServiceName() string {
	return ServiceName
}

func (cr *Connector) Discoverable(ctx context.Context) bool {
	return true
}

func (cr *Connector) CreateInstance(ctx context.Context, config service.InstanceConfig) service.Instance {
	return &instanceService{
		handler: instanceHandler{
			newConns:    cr.newConns,
			maxConns:    cr.MaxConns,
			maxDataSize: config.MaxPacketSize - packet.DataHeaderSize,
		},
	}
}

func (cr *Connector) RecreateInstance(ctx context.Context, config service.InstanceConfig, _ []byte) (service.Instance, error) {
	return cr.CreateInstance(ctx, config), nil
}

type instanceService struct {
	handler  instanceHandler
	requests chan packet.Buf
}

func (si *instanceService) Resume(ctx context.Context, replies chan<- packet.Buf) {
}

func (si *instanceService) Handle(ctx context.Context, replies chan<- packet.Buf, p packet.Buf) {
	if si.requests == nil {
		c := make(chan packet.Buf)
		go si.handler.handling(ctx, p.Code(), replies, c)
		si.requests = c
	}

	select {
	case si.requests <- p:
	case <-ctx.Done():
	}
}

func (si *instanceService) ExtractState() []byte {
	return nil
}

func (si *instanceService) Close() (err error) {
	if si.requests != nil {
		close(si.requests)
	}
	return
}

type instanceHandler struct {
	newConns    <-chan *ioConn
	maxConns    int
	maxDataSize int
}

func (ih *instanceHandler) handling(ctx context.Context, code packet.Code, replies chan<- packet.Buf, requests <-chan packet.Buf) {
	streams := make(map[int32]*conn)
	defer func() {
		for _, conn := range streams {
			conn.disconnect()
		}
	}()

	var (
		nextId     int32
		nextAccept bool
		nextFlow   uint32
	)

	// defer log.Printf("origin handler: exiting")

	for {
		acceptConns := ih.newConns
		if !nextAccept {
			acceptConns = nil
		}

		if acceptConns == nil && requests == nil {
			break
		}

		// log.Printf("origin handler: selecting")

		select {
		case ioConn, ok := <-acceptConns:
			if !ok {
				ih.newConns = nil
				break
			}

			if len(streams) >= ih.maxConns {
				// BUG: If stream ids wrap around, it's not the oldest stream
				// that will be closed.  Also, this assumes that maxConns is
				// not terribly large as it uses linear search.
				var minId int32 = math.MaxInt32
				for id := range streams {
					if id < minId {
						minId = id
					}
				}

				conn := streams[minId]
				delete(streams, minId)
				conn.disconnect()
			}

			conn := &ioConn.conn
			conn.connected(code, replies, nextId, nextFlow, ih.maxDataSize)
			streams[nextId] = conn

			// log.Printf("origin handler: accepted connection for stream #%d", nextId)

			for {
				nextId = (nextId + 1) & 0x7fffffff
				if _, taken := streams[nextId]; !taken {
					break
				}
			}
			nextAccept = false
			nextFlow = 0

		case p, ok := <-requests:
			if !ok {
				requests = nil
				break
			}

			// log.Printf("origin handler: request: %v", p)

			switch p.Domain() {
			case packet.DomainFlow:
				p := packet.FlowBuf(p)

				for i := 0; i < p.Num(); i++ {
					id, increment := p.Get(i)

					switch conn, connected := streams[id]; {
					case connected:
						if !conn.increaseIngressFlow(increment) {
							delete(streams, conn.id)
						}

					case id == nextId:
						nextAccept = true
						nextFlow += increment

					case id > nextId:
						panic(fmt.Sprintf("TODO: received data packet for distant stream: %d", id))
					}
				}

			case packet.DomainData:
				p := packet.DataBuf(p)
				id := p.ID()

				switch conn, connected := streams[id]; {
				case connected:
					if !conn.receiveEgressData(p.Data()) {
						delete(streams, conn.id)
					}

				case id >= nextId:
					panic(fmt.Sprintf("TODO: received data packet for unconnected stream: %d", id))
				}

			default:
				panic(fmt.Sprintf("TODO: unexpected domain: %d", p.Domain()))
			}

		case <-ctx.Done():
			// log.Printf("origin handler: context canceled")

			return
		}
	}
}

type ioConn struct {
	connectorClosed <-chan struct{}
	conn
}

func newIOConn(connectorClosed <-chan struct{}) (conn *ioConn) {
	conn = &ioConn{connectorClosed: connectorClosed}
	conn.construct()
	return
}

func (conn *ioConn) io(ctx context.Context, r io.Reader, w io.Writer) (err error) {
	// log.Printf("origin %p io: waiting for connection", conn)

	// Wait until connected.
	select {
	case <-conn.egressSignal:

	case <-conn.connectorClosed:
		return

	case <-ctx.Done():
		return
	}

	go func() {
		if err := conn.ingress(ctx, r); err != nil {
			// TODO: report error
		}
	}()

	return conn.egress(ctx, w)
}

func (conn *ioConn) ingress(ctx context.Context, r io.Reader) (err error) {
	defer func() {
		// Send EOF.
		select {
		case conn.replies <- packet.Buf(packet.MakeData(conn.code, conn.id, 0)):
		case <-conn.connectorClosed:
		}

		conn.lock.Lock()
		defer conn.lock.Unlock()

		conn.ioClosed = true
	}()

	var (
		flowOffset uint32
		buf        packet.DataBuf
	)

	// defer log.Printf("origin %p ingress: exiting", conn)

	for {
		// log.Printf("origin %p ingress: looping", conn)

		var capacity int

		for {
			capacity = func() int {
				conn.lock.Lock()
				defer conn.lock.Unlock()

				return int(conn.ingressFlow - flowOffset)
			}()
			if capacity > 0 {
				break
			}

			select {
			case <-conn.ingressSignal:

			case <-conn.connectorClosed:
				return

			case <-ctx.Done():
				return
			}
		}

		maxSize := conn.maxDataSize
		if maxSize > capacity {
			maxSize = capacity
		}

		var b []byte

		if bufSize := buf.DataLen(); maxSize >= bufSize && bufSize < smallReadSize {
			buf = packet.MakeData(conn.code, 0, maxSize)
			b = buf.Data()
		} else {
			b = buf.Data()
			if len(b) > maxSize {
				b = b[:maxSize]
			}
		}

		var n int

		n, err = r.Read(b)
		if err != nil {
			return
		}

		var p packet.Buf
		p, buf = buf.Split(n)

		select {
		case conn.replies <- p:

		case <-conn.connectorClosed:
			return

		case <-ctx.Done():
			return
		}

		flowOffset += uint32(n)
	}
}

func (conn *ioConn) egress(ctx context.Context, w io.Writer) (err error) {
	defer func() {
		conn.lock.Lock()
		defer conn.lock.Unlock()

		conn.ioClosed = true
	}()

	increment := uint32(egressBufSize)
	connectorClosed := conn.connectorClosed

	// defer log.Printf("origin %p egress: exiting", conn)

	for {
		// log.Printf("origin %p egress: looping", conn)

		var (
			flowPacket packet.Buf
			flowChan   chan<- packet.Buf
		)
		if increment > 0 {
			flowPacket = packet.MakeFlow(conn.code, conn.id, increment)
			flowChan = conn.replies
		}

		select {
		case flowChan <- flowPacket:
			increment = 0

		case <-conn.egressSignal:
			n, eof := func() (int, bool) {
				conn.lock.Lock()
				defer conn.lock.Unlock()

				return len(conn.egressBuf), conn.disconnected
			}()

			if n > 0 {
				_, err = w.Write(conn.egressBuf[:n])
				if err != nil {
					return
				}
			}

			if eof {
				return
			}

			// Move new data (received during write) to start of buffer.
			func() {
				conn.lock.Lock()
				defer conn.lock.Unlock()

				size := copy(conn.egressBuf, conn.egressBuf[n:])
				conn.egressBuf = conn.egressBuf[:size]
			}()

			increment += uint32(n)

		case <-connectorClosed:
			connectorClosed = nil

		case <-ctx.Done():
			return
		}
	}
}

type conn struct {
	code        packet.Code
	replies     chan<- packet.Buf
	id          int32
	maxDataSize int

	lock          sync.Mutex
	ioClosed      bool
	disconnected  bool
	ingressSignal chan struct{}
	ingressFlow   uint32
	egressSignal  chan struct{} // Used for kickstarting ingress and egress.
	egressBuf     []byte
}

func (conn *conn) construct() {
	conn.ingressSignal = make(chan struct{}, 1)
	conn.egressSignal = make(chan struct{}, 1)
}

func (conn *conn) connected(code packet.Code, replies chan<- packet.Buf, id int32, ingressFlow uint32, maxDataSize int) {
	conn.code = code
	conn.replies = replies
	conn.id = id
	conn.maxDataSize = maxDataSize
	conn.ingressFlow = ingressFlow
	conn.egressBuf = make([]byte, 0, egressBufSize)

	// Kickstart I/O.
	conn.egressSignal <- struct{}{}
}

func (conn *conn) increaseIngressFlow(increment uint32) (ok bool) {
	ok = func() bool {
		conn.lock.Lock()
		defer conn.lock.Unlock()

		conn.ingressFlow += increment
		return !conn.ioClosed
	}()

	if ok {
		select {
		case conn.ingressSignal <- struct{}{}:
		default:
		}
	} else {
		close(conn.ingressSignal)
	}
	return
}

func (conn *conn) receiveEgressData(data []byte) (ok bool) {
	ok = func() bool {
		conn.lock.Lock()
		defer conn.lock.Unlock()

		if len(data) == 0 {
			conn.disconnected = true
			return false
		}

		if len(conn.egressBuf)+len(data) > cap(conn.egressBuf) {
			panic("TODO: egress buffer overflow")
		}
		conn.egressBuf = append(conn.egressBuf, data...)
		return !conn.ioClosed
	}()

	if ok {
		select {
		case conn.egressSignal <- struct{}{}:
		default:
		}
	} else {
		close(conn.egressSignal)
	}
	return
}

func (conn *conn) disconnect() {
	conn.lock.Lock()
	defer conn.lock.Unlock()

	conn.disconnected = true
	close(conn.ingressSignal)
	close(conn.egressSignal)
}
