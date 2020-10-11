// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
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
	"gate.computer/gate/service/grpc/api"
	"github.com/google/uuid"
	"github.com/tsavola/mu"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	serviceName     = "test"
	serviceRevision = "0"
)

func main() {
	signals := make(chan os.Signal)
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
		// File descriptor number chosen by service/grpc/executable.
		conn, err := net.FileConn(os.NewFile(3, "fd 3"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		l = listenerFor(conn)
	}

	s := grpc.NewServer()
	api.RegisterRootServer(s, &rootServer{})
	state := newState()
	api.RegisterServiceServer(s, &serviceServer{state: state})
	api.RegisterInstanceServer(s, &instanceServer{state: state})

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
	closed <-chan struct{}
}

func listenerFor(conn net.Conn) *listener {
	closed := make(chan struct{})
	return &listener{
		conn: &listenConn{
			Conn:   conn,
			closed: closed,
		},
		closed: closed,
	}
}

func (l *listener) Accept() (net.Conn, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.conn
	l.conn = nil
	if c == nil {
		<-l.closed
		return nil, io.EOF
	}
	return c, nil
}

func (l *listener) Addr() net.Addr {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.conn.LocalAddr()
}

func (l *listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.conn
	l.conn = nil
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed == nil {
		return nil
	}
	defer close(c.closed)
	c.closed = nil
	return c.Conn.Close()
}

type rootServer struct {
	api.UnimplementedRootServer
}

func (s *rootServer) Init(ctx context.Context, req *api.InitRequest) (*api.InitResponse, error) {
	return &api.InitResponse{
		Services: []*api.ServiceInfo{
			{
				Name:     serviceName,
				Revision: serviceRevision,
			},
		},
	}, nil
}

type state struct {
	mu        mu.Mutex
	instances map[string]*instance
}

func newState() *state {
	return &state{
		instances: make(map[string]*instance),
	}
}

func (s *state) registerInstance(inst *instance) (id []byte) {
	id = newInstanceID()
	s.mu.Guard(func() {
		s.instances[string(id)] = inst
	})
	return
}

func (s *state) getInstance(id []byte) (*instance, error) {
	return s.lookupInstance(id, false)
}

func (s *state) removeInstance(id []byte) (*instance, error) {
	return s.lookupInstance(id, true)
}

func (s *state) lookupInstance(id []byte, remove bool) (inst *instance, err error) {
	s.mu.Guard(func() {
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

type serviceServer struct {
	api.UnimplementedServiceServer
	*state
}

func (s *serviceServer) CreateInstance(ctx context.Context, req *api.CreateInstanceRequest) (*api.CreateInstanceResponse, error) {
	inst := newInstance()
	if err := inst.restore(req.Snapshot); err != nil {
		return &api.CreateInstanceResponse{Error: err.Error()}, nil
	}
	id := s.registerInstance(inst)
	return &api.CreateInstanceResponse{Id: id}, nil
}

type instanceServer struct {
	api.UnimplementedInstanceServer
	*state
}

func (s *instanceServer) Receive(req *api.ReceiveRequest, stream api.Instance_ReceiveServer) error {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return err
	}

	return inst.sendTo(stream)
}

func (s *instanceServer) Handle(ctx context.Context, req *api.HandleRequest) (*emptypb.Empty, error) {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return nil, err
	}

	inst.handle(ctx, req.Data)
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Shutdown(ctx context.Context, req *api.ShutdownRequest) (*emptypb.Empty, error) {
	inst, _ := s.removeInstance(req.Id)
	if inst != nil {
		inst.shutdown()
	} else {
		fmt.Fprintln(os.Stderr, "instance not found at shutdown")
	}
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Suspend(ctx context.Context, req *api.SuspendRequest) (*emptypb.Empty, error) {
	inst, err := s.getInstance(req.Id)
	if err != nil {
		return nil, err
	}

	inst.suspend()
	return new(emptypb.Empty), nil
}

func (s *instanceServer) Snapshot(ctx context.Context, req *api.SnapshotRequest) (*wrapperspb.BytesValue, error) {
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
	mu        mu.Mutex
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

func (inst *instance) sendTo(stream api.Instance_ReceiveServer) error {
	done := make(chan struct{})
	defer close(done)

	var (
		loopback <-chan packet.Buf
		outgoing []byte
	)
	inst.mu.Guard(func() {
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
			inst.mu.Guard(func() {
				inst.outgoing = outgoing
			})
			return err
		}

		outgoing = outgoing[n:]
	}

	for p := range loopback {
		if err := stream.Send(&wrapperspb.BytesValue{Value: p}); err != nil {
			inst.mu.Guard(func() {
				inst.outgoing = p
			})
			return err
		}
	}

	return nil
}

func (inst *instance) handle(ctx context.Context, p packet.Buf) {
	var loopback chan<- packet.Buf
	inst.mu.Guard(func() {
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

	inst.mu.Guard(func() {
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

func (inst *instance) snapshot(ctx context.Context, outgoing, incoming []byte) ([]byte, error) {
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
