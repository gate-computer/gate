// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

//go:generate flatc --go localhost.fbs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
	"savo.la/gate/localhost/flat"
)

const ServiceName = "savo.la/gate/localhost"

// Suspended state buffer may contain a packet with its size header field
// overwritten with one of these values.
const (
	suspendedPacketIncoming uint32 = iota
	suspendedPacketOutgoing
)

type Config struct {
	Addr string
}

var pluginConfig Config

func ServiceConfig() interface{} {
	return &pluginConfig
}

func InitServices(initConfig service.Config) (err error) {
	if pluginConfig.Addr == "" {
		err = errors.New("localhost service: no address")
		return
	}

	u, err := url.Parse(pluginConfig.Addr)
	if err != nil {
		return
	}
	if !u.IsAbs() {
		err = fmt.Errorf("localhost service: URL is not absolute: %s", u)
		return
	}

	var srv *localhost

	switch u.Scheme {
	case "http", "https":
		if u.Hostname() == "" {
			err = fmt.Errorf("localhost service: URL has no host: %s", u)
			return
		}
		if u.Path != "" && u.Path != "/" {
			err = fmt.Errorf("localhost service: URL has a path: %s", u)
			return
		}

		srv = &localhost{u.Scheme, u.Host, http.DefaultClient}

	case "unix":
		if u.Host != "" {
			err = fmt.Errorf("localhost service: URL has a host: %s", u)
			return
		}
		if u.Path == "" {
			err = fmt.Errorf("localhost service: URL has no path: %s", u)
			return
		}

		dialer := net.Dialer{
			Timeout:   30 * time.Second, // Same as http.DefaultTransport (Go 1.12).
			KeepAlive: 30 * time.Second, //
		}

		unixClient := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", u.Path)
				},
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       1,
				ExpectContinueTimeout: time.Second, // Same as http.DefaultTransport (Go 1.12).
			},
		}

		srv = &localhost{"http", "localhost", unixClient}

	default:
		err = fmt.Errorf("localhost service: URL scheme not supported: %s", u)
		return
	}

	initConfig.Registry.Register(srv)
	return
}

type localhost struct {
	scheme string
	host   string
	client *http.Client
}

func (*localhost) ServiceName() string {
	return ServiceName
}

func (srv *localhost) CreateInstance(instConfig service.InstanceConfig) service.Instance {
	return &instance{srv, instConfig.Code, nil}
}

func (srv *localhost) RecreateInstance(instConfig service.InstanceConfig, state []byte,
) (inst service.Instance, err error) {
	if len(state) > 0 && len(state) < packet.HeaderSize {
		err = errors.New("state buffer is too short")
		return
	}

	inst = &instance{srv, instConfig.Code, state}
	return
}

type instance struct {
	service   *localhost
	code      packet.Code
	suspended packet.Buf
}

func (inst *instance) Resume(ctx context.Context, replies chan<- packet.Buf) {
	p := inst.suspended
	inst.suspended = nil
	if len(p) == 0 {
		return
	}

	switch binary.LittleEndian.Uint32(p) {
	case suspendedPacketIncoming:
		inst.Handle(ctx, replies, p)

	case suspendedPacketOutgoing:
		select {
		case replies <- p:

		case <-ctx.Done():
			inst.suspended = p
		}
	}
}

func (inst *instance) Handle(ctx context.Context, replies chan<- packet.Buf, p packet.Buf) {
	switch p.Domain() {
	case packet.DomainCall:
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
			binary.LittleEndian.PutUint32(p, suspendedPacketIncoming)
			inst.suspended = p
			return
		}

		p = packet.Make(inst.code, packet.DomainCall, packet.HeaderSize+len(build.FinishedBytes()))
		copy(p.Content(), build.FinishedBytes())

		select {
		case replies <- p:

		case <-ctx.Done():
			binary.LittleEndian.PutUint32(p, suspendedPacketOutgoing)
			inst.suspended = p
		}
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
		Scheme:   inst.service.scheme,
		Host:     inst.service.host,
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

	res, err := inst.service.client.Do(req.WithContext(ctx))
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

	var inlineBody flatbuffers.UOffsetT
	if res.ContentLength > 0 && res.ContentLength <= 32768 {
		data := make([]byte, res.ContentLength)
		if _, err := io.ReadFull(res.Body, data); err != nil {
			flat.HTTPResponseStart(build)
			flat.HTTPResponseAddStatusCode(build, http.StatusInternalServerError)
			return
		}
		inlineBody = build.CreateByteVector(data)
	}

	contentType := build.CreateString(res.Header.Get("Content-Type"))

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

func (inst *instance) ExtractState() []byte {
	return inst.suspended
}

func (*instance) Close() (err error) {
	return
}

func main() {}
