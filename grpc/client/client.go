// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"errors"
	"io"
	"sync"

	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/service"
	"gate.computer/grpc/api"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"import.name/lock"

	. "import.name/type/context"
)

type procKey struct {
	b []byte
	n int
}

var (
	procMu   sync.Mutex
	procKeys = make(map[runtime.ProcessKey]procKey)
)

func getProcKey(ctx Context) (key []byte) {
	opaque := runtime.MustContextProcessKey(ctx)

	lock.Guard(&procMu, func() {
		if x, found := procKeys[opaque]; found {
			x.n++
			procKeys[opaque] = x
			key = x.b
		}
	})
	if key != nil {
		return
	}

	array := uuid.New()
	key = array[:]
	lock.Guard(&procMu, func() {
		if x, found := procKeys[opaque]; found {
			x.n++
			procKeys[opaque] = x
			key = x.b
		} else {
			procKeys[opaque] = procKey{key, 1}
		}
	})
	return
}

func putProcKey(ctx Context) {
	opaque := runtime.MustContextProcessKey(ctx)

	procMu.Lock()
	defer procMu.Unlock()

	x := procKeys[opaque]
	x.n--
	if x.n == 0 {
		delete(procKeys, opaque)
	} else {
		procKeys[opaque] = x
	}
}

// Conn is a connection to a gRPC server.
type Conn struct {
	Services []*Service

	conn *grpc.ClientConn
}

// New takes ownership of conn.
func New(ctx Context, conn *grpc.ClientConn) (*Conn, error) {
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	r, err := api.NewRootClient(conn).Init(ctx, &api.InitRequest{})
	if err != nil {
		return nil, err
	}

	c := &Conn{
		conn: conn,
	}

	client := api.NewInstanceClient(conn)

	for _, info := range r.Services {
		c.Services = append(c.Services, newService(client, info))
	}

	conn = nil
	return c, nil
}

// NewClient connection to a gRPC server.
func NewClient(ctx Context, target string, opts ...grpc.DialOption) (*Conn, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	c, err := New(ctx, conn)
	if err != nil {
		return nil, err
	}

	conn = nil
	return c, nil
}

// Register the services which are accessible through the connection.
func (c *Conn) Register(r *service.Registry) error {
	for _, s := range c.Services {
		if err := r.Register(s); err != nil {
			return err
		}
	}
	return nil
}

// Close the gRPC client connection.
func (c *Conn) Close() error {
	err := c.conn.Close()
	c.conn = nil
	return err
}

type Service struct {
	c    api.InstanceClient
	info *api.Service
}

func newService(c api.InstanceClient, info *api.Service) *Service {
	return &Service{
		c:    c,
		info: info,
	}
}

func (s *Service) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     s.info.Name,
			Revision: s.info.Revision,
		},
		Streams: true,
	}
}

func (s *Service) Discoverable(ctx Context) bool {
	if s.info.RequirePrincipal && principal.ContextID(ctx) == nil {
		return false
	}

	return true
}

func (s *Service) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	key := getProcKey(ctx)
	defer func() {
		if key != nil {
			putProcKey(ctx)
		}
	}()

	r, err := s.c.Create(ctx, &api.CreateRequest{
		ServiceName: s.info.Name,
		Config:      newInstanceConfig(ctx, config, key),
		Snapshot:    snapshot,
	})
	if err != nil {
		return nil, err
	}

	if r.RestorationError != "" {
		return nil, errors.New(r.RestorationError) // TODO: a ModuleError
	}

	key = nil
	return newInstance(ctx, s.c, r.Id, config), nil
}

func newInstanceConfig(ctx Context, config service.InstanceConfig, key []byte) *api.InstanceConfig {
	r := &api.InstanceConfig{
		MaxSendSize: int32(config.MaxSendSize),
		ProcessKey:  key,
	}

	if pri := principal.ContextID(ctx); pri != nil {
		r.PrincipalId = pri.String()
	}

	if id, ok := principal.ContextInstanceUUID(ctx); ok {
		r.InstanceUuid = id[:]
	}

	return r
}

type instance struct {
	service.InstanceBase

	c    api.InstanceClient
	id   []byte
	code packet.Code

	stream   api.Instance_ReceiveClient
	leftout  <-chan []byte
	incoming []byte
}

func newInstance(ctx Context, c api.InstanceClient, id []byte, config service.InstanceConfig) *instance {
	return &instance{
		c:    c,
		id:   id,
		code: config.Code,
	}
}

func (inst *instance) Ready(ctx Context) error {
	stream, err := inst.c.Receive(ctx, &api.ReceiveRequest{
		Id: inst.id,
	})
	if err != nil {
		return err
	}

	inst.stream = stream
	return nil
}

func (inst *instance) Start(ctx Context, out chan<- packet.Thunk, abort func(error)) error {
	c := make(chan []byte, 1)
	go receiveForward(ctx, inst.code, out, inst.stream, c, abort)
	inst.leftout = c
	inst.stream = nil
	return nil
}

func (inst *instance) Handle(ctx Context, out chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if len(inst.incoming) == 0 {
		_, err := inst.c.Handle(ctx, &api.HandleRequest{
			Id:   inst.id,
			Data: p,
		})
		if err == nil {
			return nil, nil
		}

		select {
		case <-ctx.Done():

		default:
			return nil, err
		}

	}

	inst.incoming = append(inst.incoming, p...)
	return nil, nil
}

func (inst *instance) Shutdown(ctx Context, suspend bool) ([]byte, error) {
	putProcKey(ctx)

	if !suspend {
		_, err := inst.c.Shutdown(ctx, &api.ShutdownRequest{
			Id: inst.id,
		})
		return nil, err
	}

	_, err := inst.c.Suspend(ctx, &api.SuspendRequest{
		Id: inst.id,
	})
	if err != nil {
		return nil, err
	}

	var outgoing []byte
	if inst.leftout != nil {
		outgoing = <-inst.leftout
	}

	r, err := inst.c.Snapshot(ctx, &api.SnapshotRequest{
		Id:       inst.id,
		Outgoing: outgoing,
		Incoming: inst.incoming,
	})
	if err != nil {
		return nil, err
	}

	return r.Value, nil
}

func receiveForward(ctx Context, code packet.Code, out chan<- packet.Thunk, stream api.Instance_ReceiveClient, leftout chan<- []byte, abort func(error)) {
	defer close(leftout)

	for {
		r, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				abort(err)
			}
			return
		}

		p := mustBePacket(r.Value, abort)
		p.SetCode(code)

		select {
		case out <- p.Thunk():

		case <-ctx.Done():
			leftout <- receiveBuffer(p, stream, abort)
			return
		}
	}
}

func receiveBuffer(initial packet.Buf, stream api.Instance_ReceiveClient, abort func(error)) []byte {
	initial.SetSize()
	buf := initial

	for {
		r, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				abort(err)
			}
			return buf
		}

		p := mustBePacket(r.Value, abort)
		p.SetSize()

		buf = append(buf, p...)
	}
}

func mustBePacket(b []byte, abort func(error)) packet.Buf {
	if len(b) < packet.HeaderSize {
		abort(errors.New("invalid packet received from gRPC service"))
	}
	return b
}
