// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"

	"gate.computer/gate/packet"
	"gate.computer/localhost/flat"
	flatbuffers "github.com/google/flatbuffers/go"
)

// Any encoded flat.Response (just the table) must not be larger than this,
// excluding fields which are stored out of line.
const maxFlatResponseSize = 100

type handled struct {
	req packet.Buf
	res packet.Buf
}

func handle(ctx context.Context, local *Localhost, config packet.Service, req packet.Buf) handled {
	var b []byte

	tab := new(flatbuffers.Table)
	call := flat.GetRootAsCall(req, packet.HeaderSize)
	if call.Function(tab) && call.FunctionType() == flat.FunctionRequest {
		var f flat.Request
		f.Init(tab.Bytes, tab.Pos)
		b = handleRequest(ctx, local, config, f)
	}

	res := packet.Make(config.Code, packet.DomainCall, packet.HeaderSize+len(b))
	copy(res.Content(), b)

	return handled{req, res}
}

func handleRequest(ctx context.Context, local *Localhost, config packet.Service, call flat.Request) []byte {
	b := flatbuffers.NewBuilder(0)

	req := http.Request{
		Method: string(call.Method()),
	}

	callURL, err := url.Parse(string(call.Uri()))
	if err != nil || callURL.IsAbs() || callURL.Host != callURL.Hostname() {
		return buildErrorResponse(b, http.StatusBadRequest)
	}
	req.URL = &url.URL{
		Scheme:   local.scheme,
		Host:     local.host,
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

	res, err := local.client.Do(req.WithContext(ctx))
	if err != nil {
		return buildErrorResponse(b, http.StatusBadGateway)
	}
	defer res.Body.Close()

	var contentType flatbuffers.UOffsetT
	if s := res.Header.Get("Content-Type"); s != "" {
		contentType = b.CreateString(s)
	}

	contentSpace := config.MaxSendSize - int(b.Offset()) - maxFlatResponseSize
	if res.ContentLength > int64(contentSpace) {
		return buildErrorResponse(b, http.StatusBadGateway)
	}
	content, err := ioutil.ReadAll(res.Body) // TODO: limit
	if err != nil {
		return buildErrorResponse(b, http.StatusBadGateway)
	}
	if len(content) > contentSpace {
		return buildErrorResponse(b, http.StatusBadGateway)
	}

	var body flatbuffers.UOffsetT
	if len(content) > 0 {
		body = b.CreateByteVector(content)
	}

	flat.ResponseStart(b)
	flat.ResponseAddStatusCode(b, uint16(res.StatusCode))
	if contentType != 0 {
		flat.ResponseAddContentType(b, contentType)
	}
	if body != 0 {
		flat.ResponseAddBody(b, body)
	}
	b.Finish(flat.ResponseEnd(b))
	return b.FinishedBytes()
}

func buildErrorResponse(b *flatbuffers.Builder, status uint16) []byte {
	flat.ResponseStart(b)
	flat.ResponseAddStatusCode(b, status)
	b.Finish(flat.ResponseEnd(b))
	return b.FinishedBytes()
}
