// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
	"savo.la/gate/localhost/flat"
)

// Snapshot may contain a call packet with its size header field overwritten
// with one of these values.
const (
	pendingIncoming uint32 = iota
	pendingOutgoing
)

type instance struct {
	local *localhost
	packet.Service

	pending packet.Buf
}

func newInstance(local *localhost, config service.InstanceConfig) *instance {
	return &instance{
		local:   local,
		Service: config.Service,
	}
}

func (inst *instance) restore(snapshot []byte) (err error) {
	if len(snapshot) == 0 {
		return
	}

	if len(snapshot) > inst.MaxPacketSize {
		err = errors.New("snapshot is too large")
		return
	}

	p, err := packet.ImportCall(snapshot, inst.Code)
	if err != nil {
		return
	}

	switch binary.LittleEndian.Uint32(p) {
	case pendingIncoming, pendingOutgoing:

	default:
		err = errors.New("snapshot is invalid")
		return
	}

	inst.pending = append(packet.Buf{}, p...)
	return
}

func (inst *instance) Resume(ctx context.Context, send chan<- packet.Buf) {
	p := inst.pending
	inst.pending = nil
	if len(p) == 0 {
		return
	}

	switch binary.LittleEndian.Uint32(p) {
	case pendingIncoming:
		inst.Handle(ctx, send, p)

	case pendingOutgoing:
		select {
		case send <- p:

		case <-ctx.Done():
			inst.pending = p
		}
	}
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Buf, p packet.Buf) {
	if p.Domain() != packet.DomainCall {
		return
	}

	build := flatbuffers.NewBuilder(0)
	restart := false
	tab := new(flatbuffers.Table)
	call := flat.GetRootAsCall(p, packet.HeaderSize)

	if call.Function(tab) {
		switch call.FunctionType() {
		case flat.FunctionHTTPRequest:
			var req flat.HTTPRequest
			req.Init(tab.Bytes, tab.Pos)
			restart = inst.handleHTTPRequest(ctx, build, req)
			if !restart {
				build.Finish(flat.HTTPResponseEnd(build))
			}
		}
	}

	if restart {
		binary.LittleEndian.PutUint32(p, pendingIncoming)
		inst.pending = p
		return
	}

	p = packet.Make(inst.Code, packet.DomainCall, packet.HeaderSize+len(build.FinishedBytes()))
	copy(p.Content(), build.FinishedBytes())

	select {
	case send <- p:

	case <-ctx.Done():
		binary.LittleEndian.PutUint32(p, pendingOutgoing)
		inst.pending = p
		return
	}
}

// handleHTTPRequest builds an unfinished HTTPResponse unless restart is set.
func (inst *instance) handleHTTPRequest(ctx context.Context, build *flatbuffers.Builder, call flat.HTTPRequest) (restart bool) {
	var req http.Request
	var err error

	req.Method = string(call.Method())

	callURL, err := url.Parse(string(call.Uri()))
	if err != nil {
		flat.HTTPResponseStart(build)
		flat.HTTPResponseAddStatusCode(build, http.StatusBadRequest)
		return
	}
	if callURL.IsAbs() || callURL.Host != callURL.Hostname() {
		flat.HTTPResponseStart(build)
		flat.HTTPResponseAddStatusCode(build, http.StatusBadRequest)
		return
	}
	req.URL = &url.URL{
		Scheme:   inst.local.scheme,
		Host:     inst.local.host,
		Path:     callURL.Path,
		RawQuery: callURL.RawQuery,
	}
	req.Host = callURL.Hostname()

	if b := call.ContentType(); len(b) > 0 {
		req.Header = http.Header{
			"Content-Type": []string{string(b)},
		}
	}

	if n := call.BodyLength(); n > 0 {
		req.ContentLength = int64(n)
		req.Body = ioutil.NopCloser(bytes.NewReader(call.BodyBytes()))
	}

	res, err := inst.local.client.Do(req.WithContext(ctx))
	if err != nil {
		if req.Method == http.MethodGet || req.Method == http.MethodHead {
			select {
			case <-ctx.Done():
				restart = true
				return

			default:
			}
		}

		flat.HTTPResponseStart(build)
		flat.HTTPResponseAddStatusCode(build, http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	contentType := build.CreateString(res.Header.Get("Content-Type"))

	var inlineBody flatbuffers.UOffsetT
	if res.ContentLength > 0 && res.ContentLength <= int64(inst.MaxPacketSize-int(build.Offset()+100)) {
		data := make([]byte, res.ContentLength)
		if _, err := io.ReadFull(res.Body, data); err != nil {
			flat.HTTPResponseStart(build)
			flat.HTTPResponseAddStatusCode(build, http.StatusInternalServerError)
			return
		}
		inlineBody = build.CreateByteVector(data)
	}

	flat.HTTPResponseStart(build)
	flat.HTTPResponseAddStatusCode(build, int32(res.StatusCode))
	flat.HTTPResponseAddContentLength(build, res.ContentLength)
	flat.HTTPResponseAddContentType(build, contentType)
	if inlineBody != 0 {
		flat.HTTPResponseAddBody(build, inlineBody)
		flat.HTTPResponseAddBodyStreamId(build, -1)
	} else if res.ContentLength != 0 {
		flat.HTTPResponseAddBodyStreamId(build, 0) // TODO: stream body
	} else {
		flat.HTTPResponseAddBodyStreamId(build, -1)
	}
	return
}

func (inst *instance) Suspend() []byte { return inst.pending }
func (inst *instance) Shutdown()       {}
