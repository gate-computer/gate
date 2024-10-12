// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gate.computer/gate/packet"
	pb "gate.computer/gate/pb/service/grpc"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"import.name/lock"

	. "import.name/type/context"
)

const (
	serviceName     = "test"
	serviceRevision = "0"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)

	network := flag.String("net", "", "listener network")
	address := flag.String("addr", "", "listener address")
	flag.Parse()

	var err error
	var l net.Listener

	if *network != "" {
		l, err = net.Listen(*network, *address)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		// File descriptor number chosen by executable.
		conn, err := net.FileConn(os.NewFile(3, "fd 3"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		l = listenerFor(conn)
	}

	s := grpc.NewServer()
	pb.RegisterRootServer(s, new(rootServer))
	pb.RegisterInstanceServer(s, newInstanceServer())

	go func() {
		<-signals
		s.Stop()
	}()

	if err := s.Serve(l); err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type listener struct {
	mu     sync.Mutex
	conn   *listenConn
	addr   net.Addr
	closed <-chan struct{}
}

func listenerFor(conn net.Conn) *listener {
	closed := make(chan struct{})
	return &listener{
		conn: &listenConn{
			Conn:   conn,
			closed: closed,
		},
		addr:   conn.LocalAddr(),
		closed: closed,
	}
}

func (l *listener) Accept() (net.Conn, error) {
	var c *listenConn
	lock.Guard(&l.mu, func() {
		c = l.conn
		l.conn = nil
	})

	if c != nil {
		return c, nil
	}

	<-l.closed
	return nil, io.EOF
}

func (l *listener) Addr() net.Addr {
	return l.addr
}

func (l *listener) Close() error {
	var c *listenConn
	lock.Guard(&l.mu, func() {
		c = l.conn
		l.conn = nil
	})

	if c == nil {
		return nil
	}

	return c.Close()
}

type listenConn struct {
	net.Conn

	mu     sync.Mutex
	closed chan<- struct{}
}

func (c *listenConn) Close() error {
	var ch chan<- struct{}
	lock.Guard(&c.mu, func() {
		ch = c.closed
		c.closed = nil
	})

	if ch == nil {
		return nil
	}

	defer close(ch)
	return c.Conn.Close()
}

type rootServer struct {
	pb.UnimplementedRootServer
}

func (s *rootServer) Init(ctx Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	return &pb.InitResponse{
		Services: []*pb.Service{
			{
				Name:     serviceName,
				Revision: serviceRevision,
			},
		},
	}, nil
}

type instanceServer struct {
	pb.UnimplementedInstanceServer

	mu        sync.Mutex
	instances map[string]*instance
}

func newInstanceServer() *instanceServer {
	return &instanceServer{
		instances: make(map[string]*instance),
	}
}

func (s *instanceServer) registerInstance(inst *instance) (id []byte) {
	id = newInstanceID()
	lock.Guard(&s.mu, func() {
		s.instances[string(id)] = inst
	})
	return
}

func (s *instanceServer) getInstance(id []byte) (*instance, error) {
	return s.lookupInstance(id, false)
}

func (s *instanceServer) removeInstance(id []byte) (*instance, error) {
	return s.lookupInstance(id, true)
}

func (s *instanceServer) lookupInstance(id []byte, remove bool) (inst *instance, err error) {
	lock.Guard(&s.mu, func() {
		inst = s.instances[string(id)]
		if remove {
			delete(s.instances, string(id))
		}
	})
	if inst == nil {
		err = errors.New("instance not found")
	}
	return
}

func (s *instanceServer) Create(ctx Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	inst := newInstance()
	if err := inst.restore(req.Snapshot); err != nil {
		return &pb.CreateResponse{RestorationError: err.Error()}, nil
	}

	id := s.registerInstance(inst)
	return &pb.CreateResponse{Id: id}, nil
}

func (s *instanceServer) Receive(req *pb.ReceiveRequest, stream pb.Instance_ReceiveServer) error {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return err
	}

	return inst.sendTo(stream)
}

func (s *instanceServer) Handle(ctx Context, req *pb.HandleRequest) (*emptypb.Empty, error) {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return nil, err
	}

	inst.handle(ctx, req.Data)
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Shutdown(ctx Context, req *pb.ShutdownRequest) (*emptypb.Empty, error) {
	inst, _ := s.removeInstance(req.Id)
	if inst != nil {
		inst.shutdown()
	} else {
		fmt.Fprintln(os.Stderr, "instance not found at shutdown")
	}
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Suspend(ctx Context, req *pb.SuspendRequest) (*emptypb.Empty, error) {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return nil, err
	}

	inst.suspend()
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Snapshot(ctx Context, req *pb.SnapshotRequest) (*wrapperspb.BytesValue, error) {
	inst, err := s.removeInstance(req.Id)
	if err != nil {
		return nil, err
	}

	snapshot, err := inst.snapshot(ctx, req.Outgoing, req.Incoming)
	if err != nil {
		return nil, err
	}

	return &wrapperspb.BytesValue{Value: snapshot}, nil
}

func newInstanceID() []byte {
	id := uuid.New()
	return id[:]
}

type instance struct {
	mu        sync.Mutex
	loopback  chan packet.Buf
	receiving <-chan struct{}
	outgoing  []byte
	incoming  []byte
}

func newInstance() *instance {
	return &instance{
		loopback: make(chan packet.Buf),
	}
}

func (inst *instance) restore(snapshot []byte) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	for b := snapshot; len(b) > 0; {
		if len(b) < packet.HeaderSize {
			return errors.New("snapshot contains partial packet")
		}
		n := packet.Buf(b).EncodedSize()
		if n < packet.HeaderSize || n > len(b) {
			return errors.New("snapshot packet size out of bounds")
		}
		b = b[n:]
	}

	inst.outgoing = snapshot
	return nil
}

func (inst *instance) sendTo(stream pb.Instance_ReceiveServer) error {
	done := make(chan struct{})
	defer close(done)

	var (
		loopback <-chan packet.Buf
		outgoing []byte
	)
	lock.Guard(&inst.mu, func() {
		if inst.receiving == nil {
			loopback = inst.loopback
			if loopback != nil {
				inst.receiving = done
				outgoing = inst.outgoing
				inst.outgoing = nil
			}
		}
	})
	if loopback == nil {
		return errors.New("redundant reception")
	}

	for len(outgoing) > 0 {
		n := packet.Buf(outgoing).EncodedSize()

		if err := stream.Send(&wrapperspb.BytesValue{Value: outgoing[:n:n]}); err != nil {
			lock.Guard(&inst.mu, func() {
				inst.outgoing = outgoing
			})
			return err
		}

		outgoing = outgoing[n:]
	}

	for p := range loopback {
		if err := stream.Send(&wrapperspb.BytesValue{Value: p}); err != nil {
			lock.Guard(&inst.mu, func() {
				inst.outgoing = p
			})
			return err
		}
	}

	return nil
}

func (inst *instance) handle(ctx Context, p packet.Buf) {
	var loopback chan<- packet.Buf
	lock.Guard(&inst.mu, func() {
		if len(inst.incoming) == 0 && inst.loopback != nil {
			loopback = inst.loopback
		}
	})

	if loopback != nil {
		select {
		case loopback <- p:
			return

		case <-ctx.Done():
		}
	}

	lock.Guard(&inst.mu, func() {
		inst.incoming = append(inst.incoming, p...)
	})
}

func (inst *instance) shutdown() {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.loopback != nil {
		close(inst.loopback)
		inst.loopback = nil
	}
}

func (inst *instance) suspend() {
	inst.shutdown()
}

func (inst *instance) snapshot(ctx Context, outgoing, incoming []byte) ([]byte, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.receiving != nil {
		if inst.loopback != nil {
			return nil, errors.New("snapshotting before suspension")
		}

		select {
		case <-inst.receiving:

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	snapshot := append(append(outgoing, inst.outgoing...), append(inst.incoming, incoming...)...)
	inst.outgoing = nil
	inst.incoming = nil
	return snapshot, nil
}
