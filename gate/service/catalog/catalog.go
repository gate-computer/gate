// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"bytes"
	"encoding/json"
	"sort"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"

	. "import.name/type/context"
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
func New(r *service.Registry) service.Factory {
	return catalog{r}
}

func (c catalog) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (c catalog) Discoverable(Context) bool {
	return true
}

func (c catalog) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	return newInstance(c.r, config.Service), nil
}

type instance struct {
	service.InstanceBase

	r *service.Registry
	packet.Service
}

func newInstance(r *service.Registry, config packet.Service) *instance {
	return &instance{
		r:       r,
		Service: config,
	}
}

func (inst *instance) Handle(ctx Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() != packet.DomainCall {
		return nil, nil
	}

	// TODO: correct buf size in advance
	b := bytes.NewBuffer(packet.MakeCall(inst.Code, 128)[:packet.HeaderSize])

	if string(p.Content()) == "json" {
		res := response{inst.r.Catalog(ctx)}
		sort.Sort(res)

		e := json.NewEncoder(b)
		e.SetIndent("", "\t")
		if err := e.Encode(res); err != nil {
			panic(err)
		}
	}

	return b.Bytes(), nil
}

type response struct {
	Services []service.Service `json:"services"`
}

func (r response) Len() int           { return len(r.Services) }
func (r response) Swap(i, j int)      { r.Services[i], r.Services[j] = r.Services[j], r.Services[i] }
func (r response) Less(i, j int) bool { return r.Services[i].Name < r.Services[j].Name }
