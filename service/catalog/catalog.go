// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
)

const (
	serviceName     = "catalog"
	serviceRevision = "0"
)

type catalog struct {
	r *service.Registry
}

// New catalog of registered services.  The catalog will reflect the changes
// made to the registry, but not its clones.
func New(r *service.Registry) service.Factory { return catalog{r} }

func (c catalog) Service() service.Service {
	return service.Service{
		Name:     serviceName,
		Revision: serviceRevision,
	}
}

func (c catalog) Discoverable(context.Context) bool {
	return true
}

func (c catalog) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte,
) (service.Instance, error) {
	inst := newInstance(c.r, config.Service)
	if err := inst.restore(snapshot); err != nil {
		return nil, err
	}

	return inst, nil
}

const (
	pendingNone byte = iota
	pendingJSON
	pendingError
)

type instance struct {
	service.InstanceBase

	r *service.Registry
	packet.Service

	pending byte
}

func newInstance(r *service.Registry, config packet.Service) *instance {
	return &instance{
		r:       r,
		Service: config,
	}
}

func (inst *instance) restore(snapshot []byte) (err error) {
	if len(snapshot) > 0 {
		inst.pending = snapshot[0]
	}

	return
}

func (inst *instance) Start(ctx context.Context, send chan<- packet.Buf, abort func(error)) error {
	if inst.pending != pendingNone {
		inst.handleCall(ctx, send)
	}

	return nil
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Buf, p packet.Buf) error {
	switch dom := p.Domain(); {
	case dom == packet.DomainCall:
		if string(p.Content()) == "json" {
			inst.pending = pendingJSON
		} else {
			inst.pending = pendingError
		}

		inst.handleCall(ctx, send)

	case dom.IsStream():
		return errors.New("TODO: unexpected stream packet")
	}

	return nil
}

func (inst *instance) handleCall(ctx context.Context, send chan<- packet.Buf) {
	// TODO: correct buf size in advance
	b := bytes.NewBuffer(packet.MakeCall(inst.Code, 128)[:packet.HeaderSize])

	if inst.pending == pendingJSON {
		res := response{inst.r.Catalog(ctx)}
		sort.Sort(res)

		e := json.NewEncoder(b)
		e.SetIndent("", "\t")
		if err := e.Encode(res); err != nil {
			panic(err)
		}
	}

	select {
	case send <- b.Bytes():
		inst.pending = pendingNone

	case <-ctx.Done():
		return
	}
}

func (inst *instance) Suspend(ctx context.Context) ([]byte, error) {
	if inst.pending != pendingNone {
		return []byte{inst.pending}, nil
	}

	return nil, nil
}

type response struct {
	Services []service.Service `json:"services"`
}

func (r response) Len() int           { return len(r.Services) }
func (r response) Swap(i, j int)      { r.Services[i], r.Services[j] = r.Services[j], r.Services[i] }
func (r response) Less(i, j int) bool { return r.Services[i].Name < r.Services[j].Name }
